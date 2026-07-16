package sandbox

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// ChildExecHelperCommand is the hidden entry point used to install a package
// process environment only after the OS sandbox boundary is active.
const ChildExecHelperCommand = "__reamesAgent_child_exec"

const childEnvironmentVariable = "REAMES_AGENT_INTERNAL_CHILD_ENV_V1"

// CommandHostEnvironment returns the environment for the trusted sandbox
// wrapper. Child variables are serialized behind one reserved variable so
// manifest keys and values never appear in the wrapper command line or affect
// the wrapper's dynamic loader/runtime before confinement is active.
func CommandHostEnvironment(hostEnv, childEnv []string) ([]string, error) {
	out := make([]string, 0, len(hostEnv)+1)
	for _, item := range hostEnv {
		key, _, ok := strings.Cut(item, "=")
		if ok && reservedChildEnvironmentKey(key) {
			continue
		}
		out = append(out, item)
	}
	if childEnv == nil {
		return out, nil
	}
	encoded, err := encodeChildEnvironment(childEnv)
	if err != nil {
		return nil, err
	}
	return append(out, childEnvironmentVariable+"="+encoded), nil
}

func childExecCommand(args, childEnv []string) ([]string, error) {
	if childEnv == nil {
		return append([]string(nil), args...), nil
	}
	exe, err := os.Executable()
	if err != nil || exe == "" {
		if err == nil {
			err = fmt.Errorf("current executable is empty")
		}
		return nil, fmt.Errorf("resolve sandbox child helper: %w", err)
	}
	return append([]string{exe, ChildExecHelperCommand, "--"}, args...), nil
}

func encodeChildEnvironment(env []string) (string, error) {
	clean := make([]string, 0, len(env))
	for _, item := range env {
		key, _, ok := strings.Cut(item, "=")
		if !ok || strings.TrimSpace(key) == "" || strings.IndexByte(key, 0) >= 0 || strings.IndexByte(item, 0) >= 0 {
			return "", fmt.Errorf("encode sandbox child environment: invalid entry")
		}
		if reservedChildEnvironmentKey(key) {
			continue
		}
		clean = append(clean, item)
	}
	raw, err := json.Marshal(clean)
	if err != nil {
		return "", fmt.Errorf("encode sandbox child environment: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func decodeChildEnvironment(encoded string) ([]string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("decode sandbox child environment: invalid encoding")
	}
	var env []string
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("decode sandbox child environment: invalid payload")
	}
	for _, item := range env {
		key, _, ok := strings.Cut(item, "=")
		if !ok || strings.TrimSpace(key) == "" || strings.IndexByte(key, 0) >= 0 || strings.IndexByte(item, 0) >= 0 || reservedChildEnvironmentKey(key) {
			return nil, fmt.Errorf("decode sandbox child environment: invalid entry")
		}
	}
	return env, nil
}

// takeChildEnvironment removes the serialized payload from this helper before
// it starts an untrusted child. The encoded value must not propagate further.
func takeChildEnvironment(required bool) ([]string, error) {
	encoded, ok := os.LookupEnv(childEnvironmentVariable)
	_ = os.Unsetenv(childEnvironmentVariable)
	if !ok {
		if required {
			return nil, fmt.Errorf("sandbox child environment is missing")
		}
		return nil, nil
	}
	env, err := decodeChildEnvironment(encoded)
	if err != nil {
		return nil, err
	}
	return env, nil
}

func reservedChildEnvironmentKey(key string) bool {
	return strings.EqualFold(strings.TrimSpace(key), childEnvironmentVariable)
}
