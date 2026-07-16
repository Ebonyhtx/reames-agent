package sandbox

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestCommandHostEnvironmentEncodesChildDataBehindReservedKey(t *testing.T) {
	host := []string{"PATH=/trusted/bin", childEnvironmentVariable + "=caller-planted"}
	child := []string{"PATH=/child/bin", "PLUGIN_TOKEN=explicit-plugin-secret"}
	got, err := CommandHostEnvironment(host, child)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(got, "\n")
	for _, raw := range []string{"caller-planted", "PLUGIN_TOKEN", "explicit-plugin-secret"} {
		if strings.Contains(joined, raw) {
			t.Fatalf("wrapper environment exposed raw child data %q: %v", raw, got)
		}
	}
	encoded := ""
	for _, item := range got {
		key, value, ok := strings.Cut(item, "=")
		if ok && reservedChildEnvironmentKey(key) {
			if encoded != "" {
				t.Fatalf("duplicate reserved environment entries: %v", got)
			}
			encoded = value
		}
	}
	if encoded == "" {
		t.Fatalf("reserved child environment missing: %v", got)
	}
	decoded, err := decodeChildEnvironment(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, child) {
		t.Fatalf("decoded child environment = %v, want %v", decoded, child)
	}
}

func TestChildExecCommandKeepsRawEnvironmentOutOfArgv(t *testing.T) {
	argv, err := childExecCommand([]string{"plugin-bin", "serve"}, []string{"PLUGIN_TOKEN=explicit-plugin-secret"})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(argv, "\n")
	for _, raw := range []string{"PLUGIN_TOKEN", "explicit-plugin-secret"} {
		if strings.Contains(joined, raw) {
			t.Fatalf("child helper argv exposed %q: %v", raw, argv)
		}
	}
	if len(argv) < 5 || argv[1] != ChildExecHelperCommand || argv[2] != "--" || argv[3] != "plugin-bin" {
		t.Fatalf("child helper argv = %v", argv)
	}
}

func TestTakeChildEnvironmentClearsReservedPayload(t *testing.T) {
	want := []string{"PATH=/bin", "PLUGIN_TOKEN=explicit"}
	encoded, err := encodeChildEnvironment(want)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(childEnvironmentVariable, encoded)
	got, err := takeChildEnvironment(true)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("decoded child environment = %v, want %v", got, want)
	}
	if _, ok := os.LookupEnv(childEnvironmentVariable); ok {
		t.Fatal("reserved child environment remained set after decode")
	}
}

func TestDecodeChildEnvironmentRejectsReservedVariable(t *testing.T) {
	encoded, err := encodeChildEnvironment([]string{childEnvironmentVariable + "=nested", "PATH=/bin"})
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeChildEnvironment(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded) != 1 || decoded[0] != "PATH=/bin" {
		t.Fatalf("reserved child variable was not stripped: %v", decoded)
	}
}
