package themepack

import (
	"math"
	"strconv"
	"strings"
)

type ContrastWarning struct {
	Mode    string  `json:"mode"`
	Pair    string  `json:"pair"`
	Ratio   float64 `json:"ratio"`
	Minimum float64 `json:"minimum"`
	Suggest string  `json:"suggest"`
}

// ContrastWarnings reports common WCAG AA risks without weakening the existing
// production CSS contrast gate or blocking a package import.
func ContrastWarnings(manifest Manifest) []ContrastWarning {
	var warnings []ContrastWarning
	warnings = append(warnings, contrastWarningsForMode("light", manifest.Tokens.Light)...)
	warnings = append(warnings, contrastWarningsForMode("dark", manifest.Tokens.Dark)...)
	if warnings == nil {
		return []ContrastWarning{}
	}
	return warnings
}

func contrastWarningsForMode(mode string, tokens map[string]string) []ContrastWarning {
	if len(tokens) == 0 {
		return nil
	}
	defaults := map[string]string{
		"fg": "#f1f1ef", "bg": "#0c0d10", "fgFaint": "#8f929a",
		"chat": "#0c0d10", "accent": "#bd3f18", "accentFg": "#ffffff",
	}
	if mode == "light" {
		defaults = map[string]string{
			"fg": "#111827", "bg": "#f7f8fb", "fgFaint": "#667085",
			"chat": "#ffffff", "accent": "#315f8c", "accentFg": "#ffffff",
		}
	}
	value := func(key string) string {
		if v := strings.TrimSpace(tokens[key]); v != "" {
			return v
		}
		return defaults[key]
	}
	pairs := []struct {
		name, fg, bg string
		minimum      float64
	}{
		{"fg/bg", value("fg"), value("bg"), 4.5},
		{"fg/chat", value("fg"), value("chat"), 4.5},
		{"fgFaint/bg", value("fgFaint"), value("bg"), 3.0},
		{"accentFg/accent", value("accentFg"), value("accent"), 3.0},
	}
	var warnings []ContrastWarning
	for _, pair := range pairs {
		if ratio, ok := contrastRatio(pair.fg, pair.bg); ok && ratio < pair.minimum {
			warnings = append(warnings, ContrastWarning{
				Mode: mode, Pair: pair.name, Ratio: math.Round(ratio*100) / 100, Minimum: pair.minimum,
				Suggest: readableForeground(pair.bg),
			})
		}
	}
	return warnings
}

func contrastRatio(a, b string) (float64, bool) {
	la, okA := relativeLuminance(a)
	lb, okB := relativeLuminance(b)
	if !okA || !okB {
		return 0, false
	}
	return (math.Max(la, lb) + .05) / (math.Min(la, lb) + .05), true
}

func relativeLuminance(value string) (float64, bool) {
	value = strings.TrimPrefix(strings.TrimSpace(value), "#")
	if len(value) != 6 && len(value) != 8 {
		return 0, false
	}
	components := make([]float64, 3)
	for index := range 3 {
		parsed, err := strconv.ParseUint(value[index*2:index*2+2], 16, 8)
		if err != nil {
			return 0, false
		}
		component := float64(parsed) / 255
		if component <= .04045 {
			component /= 12.92
		} else {
			component = math.Pow((component+.055)/1.055, 2.4)
		}
		components[index] = component
	}
	return .2126*components[0] + .7152*components[1] + .0722*components[2], true
}

func readableForeground(background string) string {
	if luminance, ok := relativeLuminance(background); ok && luminance > .45 {
		return "#111827"
	}
	return "#f1f1ef"
}
