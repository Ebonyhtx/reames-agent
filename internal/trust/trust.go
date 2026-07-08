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
	return text
}
