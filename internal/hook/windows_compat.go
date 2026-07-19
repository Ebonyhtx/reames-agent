package hook

import (
	"encoding/base64"
	"encoding/binary"
	"strings"
	"unicode/utf16"
)

// windowsBatchCommandLine builds the cmd.exe command line for a shell-form .cmd
// or .bat hook whose executable is already quoted. Preserve the original
// argument tail byte-for-byte so valid batch syntax is not reinterpreted.
func windowsBatchCommandLine(command string) (string, bool) {
	command = strings.TrimSpace(command)
	if len(command) < 2 || command[0] != '"' {
		return "", false
	}
	closingQuote := strings.IndexByte(command[1:], '"')
	if closingQuote < 0 {
		return "", false
	}
	closingQuote++
	executable := normalizeWindowsBatchExecutable(command[1:closingQuote])
	if !isWindowsBatchExecutable(executable) {
		return "", false
	}
	tail := command[closingQuote+1:]
	if tail != "" && !isShellWhitespace(tail[0]) {
		return "", false
	}
	if !isSimpleWindowsBatchTail(tail) {
		return "", false
	}
	// /s strips the first and last quotes around the /c string, leaving the
	// quoted executable and its untouched argument tail for cmd.exe to parse.
	return `cmd.exe /d /s /c ""` + executable + `"` + tail + `"`, true
}

func windowsBatchPackageArgs(command string) ([]string, bool) {
	executable := normalizeWindowsBatchExecutable(command)
	if !isWindowsBatchExecutable(executable) || strings.ContainsAny(executable, "\"%!\r\n") {
		return nil, false
	}
	// Package hooks must stay inside the package sandbox, so the ordinary raw
	// cmd.exe SysProcAttr path is unavailable. PowerShell receives the batch
	// path through -EncodedCommand, avoiding both cmd's leading-quote ambiguity
	// and any re-parsing of spaces/metacharacters by the sandbox helper.
	commandText := `& '` + strings.ReplaceAll(executable, `'`, `''`) + `'`
	return []string{"powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-EncodedCommand", encodePowerShellCommand(commandText)}, true
}

func encodePowerShellCommand(command string) string {
	words := utf16.Encode([]rune(command))
	data := make([]byte, len(words)*2)
	for i, word := range words {
		binary.LittleEndian.PutUint16(data[i*2:], word)
	}
	return base64.StdEncoding.EncodeToString(data)
}

func normalizeWindowsBatchExecutable(executable string) string {
	return strings.ReplaceAll(strings.TrimSpace(executable), "/", `\`)
}

func isWindowsBatchExecutable(executable string) bool {
	lower := strings.ToLower(executable)
	return strings.HasSuffix(lower, ".cmd") || strings.HasSuffix(lower, ".bat")
}

func isSimpleWindowsBatchTail(tail string) bool {
	quoted := false
	for i := 0; i < len(tail); i++ {
		switch tail[i] {
		case '\r', '\n':
			return false
		case '"':
			quoted = !quoted
		case '&', '|', ';', '<', '>', '(', ')':
			if !quoted {
				return false
			}
		}
	}
	return !quoted
}

func isShellWhitespace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\r' || b == '\n'
}
