package appserver

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"sync"
	"sync/atomic"
)

const (
	maxMessageBytes = 8 << 20
	maxInFlight     = 64
)

type requestHandler func(context.Context, json.RawMessage) (any, error)
type notifyHandler func(context.Context, json.RawMessage)

type inbound struct {
	JSONRPC string          `json:"jsonrpc,omitempty"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *wireError      `json:"error,omitempty"`
}

type outbound struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *wireError      `json:"error,omitempty"`
}

type wireError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResult struct {
	result json.RawMessage
	err    error
}

// afterResponseResult lets a handler start streaming only after its response
// has been serialized and written. turn/start uses it to preserve wire order.
type afterResponseResult interface {
	ResponseValue() any
	AfterResponse()
}

type Conn struct {
	r   io.Reader
	enc *json.Encoder

	reqH map[string]requestHandler
	notH map[string]notifyHandler
	sem  chan struct{}

	wmu sync.Mutex
	wg  sync.WaitGroup

	nextID  atomic.Int64
	pmu     sync.Mutex
	pending map[int64]chan rpcResult

	closed    chan struct{}
	closeOnce sync.Once
}

func NewConn(r io.Reader, w io.Writer) *Conn {
	return &Conn{
		r: r, enc: json.NewEncoder(w), reqH: make(map[string]requestHandler),
		notH: make(map[string]notifyHandler), sem: make(chan struct{}, maxInFlight),
		pending: make(map[int64]chan rpcResult), closed: make(chan struct{}),
	}
}

func (c *Conn) Handle(method string, h requestHandler)      { c.reqH[method] = h }
func (c *Conn) HandleNotify(method string, h notifyHandler) { c.notH[method] = h }

func (c *Conn) Serve(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stopReaderClose := make(chan struct{})
	if closer, ok := c.r.(io.Closer); ok {
		go func() {
			select {
			case <-ctx.Done():
				_ = closer.Close()
			case <-stopReaderClose:
			}
		}()
	}
	defer close(stopReaderClose)
	br := bufio.NewReaderSize(c.r, 64<<10)
	var loopErr error
	for {
		line, err := readLine(br)
		if len(line) > 0 {
			c.dispatch(ctx, line)
		}
		if err != nil {
			if ctx.Err() != nil {
				loopErr = ctx.Err()
			} else if !errors.Is(err, io.EOF) {
				loopErr = err
			}
			break
		}
		if ctx.Err() != nil {
			loopErr = ctx.Err()
			break
		}
	}
	cancel()
	c.wg.Wait()
	c.shutdown()
	return loopErr
}

func readLine(br *bufio.Reader) ([]byte, error) {
	var buf []byte
	for {
		chunk, err := br.ReadSlice('\n')
		buf = append(buf, chunk...)
		if errors.Is(err, bufio.ErrBufferFull) {
			if len(buf) > maxMessageBytes {
				return nil, errors.New("appserver: message exceeds size limit")
			}
			continue
		}
		for len(buf) > 0 && (buf[len(buf)-1] == '\n' || buf[len(buf)-1] == '\r') {
			buf = buf[:len(buf)-1]
		}
		if len(buf) > maxMessageBytes {
			return nil, errors.New("appserver: message exceeds size limit")
		}
		return buf, err
	}
}

func (c *Conn) dispatch(ctx context.Context, line []byte) {
	var in inbound
	if !json.Valid(line) {
		c.writeError(json.RawMessage("null"), ErrParse, "parse error")
		return
	}
	dec := json.NewDecoder(bytes.NewReader(line))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		c.writeError(json.RawMessage("null"), ErrInvalidRequest, "invalid request")
		return
	}
	hasID := len(in.ID) > 0
	hasResult := len(in.Result) > 0
	hasError := in.Error != nil
	if in.JSONRPC != "" || (in.Method != "" && (hasResult || hasError)) || (in.Method == "" && (!hasID || hasResult == hasError)) {
		c.writeError(json.RawMessage("null"), ErrInvalidRequest, "invalid request")
		return
	}
	switch {
	case in.Method != "" && hasID:
		select {
		case c.sem <- struct{}{}:
			c.wg.Add(1)
			go func() {
				defer c.wg.Done()
				defer func() { <-c.sem }()
				c.serveRequest(ctx, in.ID, in.Method, in.Params)
			}()
		default:
			c.writeError(in.ID, ErrOverloaded, "Server overloaded; retry later.")
		}
	case in.Method != "" && !hasID:
		if h := c.notH[in.Method]; h != nil {
			select {
			case c.sem <- struct{}{}:
				c.wg.Add(1)
				go func() { defer c.wg.Done(); defer func() { <-c.sem }(); h(ctx, in.Params) }()
			default:
				// Notifications have no response channel. Dropping excess client
				// notifications is the only bounded behavior available.
			}
		}
	case in.Method == "" && hasID:
		c.resolve(in)
	default:
		c.writeError(json.RawMessage("null"), ErrInvalidRequest, "invalid request")
	}
}

func (c *Conn) serveRequest(ctx context.Context, id json.RawMessage, method string, params json.RawMessage) {
	h := c.reqH[method]
	if h == nil {
		c.writeError(id, ErrMethodNotFound, "method not found: "+method)
		return
	}
	result, err := h(ctx, params)
	if err != nil {
		code := ErrInternal
		var rpcErr *RPCError
		if errors.As(err, &rpcErr) {
			code = rpcErr.Code
		}
		c.writeError(id, code, err.Error())
		return
	}
	var after afterResponseResult
	if candidate, ok := result.(afterResponseResult); ok {
		after = candidate
		result = candidate.ResponseValue()
	}
	raw, err := json.Marshal(result)
	if err != nil {
		c.writeError(id, ErrInternal, "marshal result: "+err.Error())
		return
	}
	if err := c.write(outbound{ID: id, Result: raw}); err == nil && after != nil {
		after.AfterResponse()
	}
}

func (c *Conn) resolve(in inbound) {
	id, err := strconv.ParseInt(string(in.ID), 10, 64)
	if err != nil {
		return
	}
	c.pmu.Lock()
	ch := c.pending[id]
	delete(c.pending, id)
	c.pmu.Unlock()
	if ch == nil {
		return
	}
	if in.Error != nil {
		ch <- rpcResult{err: fmt.Errorf("peer error %d: %s", in.Error.Code, in.Error.Message)}
		return
	}
	ch <- rpcResult{result: in.Result}
}

func (c *Conn) Notify(method string, params any) error {
	raw, err := json.Marshal(params)
	if err != nil {
		return err
	}
	return c.write(outbound{Method: method, Params: raw})
}

func (c *Conn) Request(ctx context.Context, method string, params any) (json.RawMessage, error) {
	raw, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	id := c.nextID.Add(1)
	ch := make(chan rpcResult, 1)
	c.pmu.Lock()
	c.pending[id] = ch
	c.pmu.Unlock()
	defer func() { c.pmu.Lock(); delete(c.pending, id); c.pmu.Unlock() }()
	idRaw, _ := json.Marshal(id)
	if err := c.write(outbound{ID: idRaw, Method: method, Params: raw}); err != nil {
		return nil, err
	}
	select {
	case res := <-ch:
		return res.result, res.err
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.closed:
		return nil, errors.New("appserver: connection closed")
	}
}

func (c *Conn) write(message outbound) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	return c.enc.Encode(message)
}

func (c *Conn) writeError(id json.RawMessage, code int, message string) {
	_ = c.write(outbound{ID: id, Error: &wireError{Code: code, Message: message}})
}

func (c *Conn) shutdown() {
	c.closeOnce.Do(func() {
		close(c.closed)
		c.pmu.Lock()
		defer c.pmu.Unlock()
		for id, ch := range c.pending {
			ch <- rpcResult{err: errors.New("appserver: connection closed")}
			delete(c.pending, id)
		}
	})
}
