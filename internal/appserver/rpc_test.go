package appserver

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestConnRejectsMalformedAndNonAppServerFrames(t *testing.T) {
	input := strings.Join([]string{
		`{`,
		`{"jsonrpc":"2.0","id":1,"method":"unknown"}`,
		`{"id":2,"method":"unknown","extra":true}`,
		`{"id":3,"method":"unknown","result":{}}`,
		`{"id":4,"method":"unknown"}`,
	}, "\n") + "\n"
	var output bytes.Buffer
	if err := NewConn(strings.NewReader(input), &output).Serve(context.Background()); err != nil {
		t.Fatal(err)
	}

	wantCodes := []int{ErrParse, ErrInvalidRequest, ErrInvalidRequest, ErrInvalidRequest, ErrMethodNotFound}
	scanner := bufio.NewScanner(&output)
	gotCodes := make([]int, 0, len(wantCodes))
	for scanner.Scan() {
		var frame struct {
			Error *wireError `json:"error"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &frame); err != nil {
			t.Fatal(err)
		}
		if frame.Error == nil {
			t.Fatalf("expected error frame: %s", scanner.Bytes())
		}
		gotCodes = append(gotCodes, frame.Error.Code)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if len(gotCodes) != len(wantCodes) {
		t.Fatalf("error frames = %v, want %v", gotCodes, wantCodes)
	}
	counts := make(map[int]int)
	for _, code := range gotCodes {
		counts[code]++
	}
	for _, code := range wantCodes {
		counts[code]--
	}
	for code, count := range counts {
		if count != 0 {
			t.Fatalf("error code %d count delta = %d; got %v want %v", code, count, gotCodes, wantCodes)
		}
	}
}

func TestConnCancellationClosesBlockingReader(t *testing.T) {
	reader, writer := io.Pipe()
	t.Cleanup(func() { _ = writer.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- NewConn(reader, io.Discard).Serve(ctx) }()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Serve cancellation error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Serve stayed blocked on its reader after cancellation")
	}
}

func TestReadLineEnforcesMessageBudget(t *testing.T) {
	allowed := bytes.Repeat([]byte{'x'}, maxMessageBytes)
	line, err := readLine(bufio.NewReader(bytes.NewReader(append(allowed, '\n'))))
	if err != nil || len(line) != maxMessageBytes {
		t.Fatalf("maximum frame = %d bytes, err %v", len(line), err)
	}
	oversized := bytes.Repeat([]byte{'x'}, maxMessageBytes+1)
	if line, err := readLine(bufio.NewReader(bytes.NewReader(oversized))); err == nil || line != nil {
		t.Fatalf("oversized frame = %d bytes, err %v", len(line), err)
	}
}

func TestConnRejectsRequestBeyondConcurrencyBudget(t *testing.T) {
	var output bytes.Buffer
	conn := NewConn(strings.NewReader(""), &output)
	started := make(chan struct{}, maxInFlight)
	release := make(chan struct{})
	conn.Handle("block", func(context.Context, json.RawMessage) (any, error) {
		started <- struct{}{}
		<-release
		return struct{}{}, nil
	})
	ctx := context.Background()
	for id := 1; id <= maxInFlight; id++ {
		conn.dispatch(ctx, []byte(`{"id":`+strconv.Itoa(id)+`,"method":"block"}`))
	}
	for i := 0; i < maxInFlight; i++ {
		select {
		case <-started:
		case <-time.After(2 * time.Second):
			t.Fatal("blocking handlers did not fill the concurrency budget")
		}
	}
	overloadID := maxInFlight + 1
	conn.dispatch(ctx, []byte(`{"id":`+strconv.Itoa(overloadID)+`,"method":"block"}`))
	close(release)
	conn.wg.Wait()

	scanner := bufio.NewScanner(bytes.NewReader(output.Bytes()))
	found := false
	for scanner.Scan() {
		var frame struct {
			ID    json.RawMessage `json:"id"`
			Error *wireError      `json:"error"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &frame); err != nil {
			t.Fatal(err)
		}
		if string(frame.ID) == strconv.Itoa(overloadID) {
			found = frame.Error != nil && frame.Error.Code == ErrOverloaded
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatalf("request %d did not receive overloaded response: %s", overloadID, output.String())
	}
}
