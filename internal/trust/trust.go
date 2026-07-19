package trust

import (
	"regexp"
	"strings"
)

func SanitizeHTML(raw string) string {
	dangerousWithContent := []string{"script", "style", "iframe", "object", "embed", "noscript"}
	for _, tag := range dangerousWithContent {
		re := regexp.MustCompile(`(?is)<` + tag + `[^>]*>.*?</` + tag + `>`)
		raw = re.ReplaceAllString(raw, "")
	}
	re := regexp.MustCompile(`<[^>]*>`)
	raw = re.ReplaceAllString(raw, "")
	raw = regexp.MustCompile(`\s+`).ReplaceAllString(raw, " ")
	return strings.TrimSpace(raw)
}

func WrapUntrusted(content, source string) string {
	if content == "" {
		return ""
	}
	return "[UNTRUSTED_" + strings.ToUpper(source) + "_OUTPUT]\n" +
		content + "\n[/UNTRUSTED_" + strings.ToUpper(source) + "_OUTPUT]"
}

func RedactSecrets(text string) string {
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`sk-[a-zA-Z0-9]{32,}`),
		regexp.MustCompile(`(?:ghp|gho|ghu|ghs|ghr)_[a-zA-Z0-9]{36,}`),
		regexp.MustCompile(`xox[baprs]-[a-zA-Z0-9-]{10,}`),
		regexp.MustCompile(`AIza[0-9A-Za-z\-_]{35}`),
		regexp.MustCompile(`-----BEGIN (?:RSA |EC |DSA |OPENSSH )?PRIVATE KEY-----`),
		regexp.MustCompile(`eyJ[A-Za-z0-9\-_=]+\.[A-Za-z0-9\-_=]+\.?[A-Za-z0-9\-_.+/=]*`),
	}
	labels := []string{"OpenAI", "GitHub", "Slack", "Google", "PrivateKey", "JWT"}
	for i, p := range patterns {
		text = p.ReplaceAllString(text, "[REDACTED:"+labels[i]+"]")
	}
	return redactCredentialKeyValues(text)
}

const redactedCredentialValue = "[REDACTED]"

// redactCredentialKeyValues scans KEY=value, JSON-like key:value, and
// Authorization headers in linear time. Large concurrent transcripts used to
// make credential-shaped regexp matching disproportionately expensive; the
// byte scanner keeps the trust boundary predictable while preserving schemes
// such as "Bearer" and "Basic" for diagnostics.
func redactCredentialKeyValues(text string) string {
	var out strings.Builder
	last := 0
	for sep := 0; sep < len(text); sep++ {
		if text[sep] != ':' && text[sep] != '=' {
			continue
		}
		keyEnd := sep
		for keyEnd > 0 && asciiSpace(text[keyEnd-1]) {
			keyEnd--
		}
		if keyEnd > 0 && (text[keyEnd-1] == '\'' || text[keyEnd-1] == '"') {
			keyEnd--
		}
		keyStart := keyEnd
		for keyStart > 0 && credentialKeyByte(text[keyStart-1]) {
			keyStart--
		}
		key := text[keyStart:keyEnd]
		if !credentialTextKeySensitive(key) {
			continue
		}

		valueStart := sep + 1
		for valueStart < len(text) && asciiSpace(text[valueStart]) {
			valueStart++
		}
		if valueStart < len(text) && (text[valueStart] == '\'' || text[valueStart] == '"') {
			valueStart++
		}
		schemeStart := valueStart
		for valueStart < len(text) && credentialKeyByte(text[valueStart]) {
			valueStart++
		}
		if valueStart < len(text) && asciiSpace(text[valueStart]) && authorizationScheme(text[schemeStart:valueStart]) {
			for valueStart < len(text) && asciiSpace(text[valueStart]) {
				valueStart++
			}
			if valueStart < len(text) && (text[valueStart] == '\'' || text[valueStart] == '"') {
				valueStart++
			}
		} else {
			valueStart = schemeStart
		}

		valueEnd := valueStart
		for valueEnd < len(text) && !credentialValueDelimiter(text[valueEnd]) {
			valueEnd++
		}
		if valueEnd == valueStart {
			continue
		}
		if alreadyRedactedCredentialValue(text[valueStart:valueEnd]) {
			sep = valueEnd - 1
			continue
		}
		if last == 0 {
			out.Grow(len(text))
		}
		out.WriteString(text[last:valueStart])
		out.WriteString(redactedCredentialValue)
		last = valueEnd
		sep = valueEnd - 1
	}
	if last == 0 {
		return text
	}
	out.WriteString(text[last:])
	return out.String()
}

func credentialValueDelimiter(b byte) bool {
	return asciiSpace(b) || b == '\'' || b == '"' || b == ',' || b == ';' || b == '&' || b == '#'
}

func alreadyRedactedCredentialValue(value string) bool {
	return strings.HasPrefix(value, "[REDACTED]") ||
		strings.HasPrefix(value, "[REDACTED:") && strings.Contains(value, "]")
}

func credentialKeyByte(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b == '_' || b == '-' || b == '.'
}

func asciiSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r' || b == '\f'
}

func authorizationKey(key string) bool {
	upper := strings.ToUpper(key)
	return upper == "AUTHORIZATION" || strings.HasSuffix(upper, "-AUTHORIZATION") || strings.HasSuffix(upper, "_AUTHORIZATION") || strings.HasSuffix(upper, ".AUTHORIZATION")
}

func credentialTextKeySensitive(key string) bool {
	upper := strings.ToUpper(key)
	compact := strings.NewReplacer("_", "", "-", "").Replace(upper)
	return authorizationKey(key) ||
		strings.Contains(compact, "APIKEY") ||
		strings.Contains(compact, "ACCESSKEY") ||
		strings.Contains(compact, "PRIVATEKEY") ||
		strings.Contains(upper, "SECRET") ||
		strings.Contains(upper, "TOKEN") ||
		strings.Contains(upper, "PASSWORD") ||
		strings.Contains(upper, "PASSWD") ||
		strings.Contains(upper, "_PWD") ||
		strings.Contains(upper, "-PWD")
}

func authorizationScheme(value string) bool {
	switch strings.ToLower(value) {
	case "bearer", "basic", "digest", "negotiate", "ntlm", "token", "bot", "apikey":
		return true
	default:
		return false
	}
}
