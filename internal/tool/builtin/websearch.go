package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"reames-agent/internal/netclient"
	"reames-agent/internal/tool"
)

func init() { tool.RegisterBuiltin(webSearch{}) }

type webSearch struct {
	proxySpec netclient.ProxySpec
}

const webSearchTimeout = 10 * time.Second

func (webSearch) Name() string { return "web_search" }

func (webSearch) Description() string {
	return "Search the web and return results with titles, URLs, and snippets. Use to find current information, documentation, or answers that may not be in your training data. Returns up to 10 results."
}

func (webSearch) Schema() json.RawMessage {
	return json.RawMessage(`{
"type":"object",
"properties":{
  "query":{"type":"string","description":"Search query string"},
  "limit":{"type":"integer","description":"Maximum results (default 5, max 10)"}
},
"required":["query"]
}`)
}

func (webSearch) ReadOnly() bool { return true }

func (webSearch) SnipHint() tool.SnipHint {
	return tool.SnipHint{Head: 16, Tail: 4, HeadChars: 3000, TailChars: 500}
}

func (ws webSearch) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("web_search: invalid args: %w", err)
	}
	if p.Query == "" {
		return "", fmt.Errorf("web_search: query is required")
	}
	limit := p.Limit
	if limit <= 0 {
		limit = 5
	}
	if limit > 10 {
		limit = 10
	}

	results, err := ws.search(ctx, p.Query, limit)
	if err != nil {
		return "", fmt.Errorf("web_search: %w", err)
	}
	if len(results) == 0 {
		return "No results found.", nil
	}
	return formatResults(results), nil
}

type searchResult struct {
	Title   string
	URL     string
	Snippet string
}

func (ws webSearch) search(ctx context.Context, query string, limit int) ([]searchResult, error) {
	pf, err := netclient.ProxyFunc(ws.proxySpec)
	if err != nil {
		return nil, err
	}
	client := &http.Client{
		Transport: &http.Transport{Proxy: pf},
		Timeout:   webSearchTimeout,
	}

	searchURL := "https://html.duckduckgo.com/html/?q=" + url.QueryEscape(query)
	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Reames-Agent/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<19))
	if err != nil {
		return nil, err
	}

	return parseDuckDuckGo(string(body), limit), nil
}

// parseDuckDuckGo extracts search results from DuckDuckGo's HTML response.
func parseDuckDuckGo(html string, limit int) []searchResult {
	var results []searchResult

	for len(results) < limit && html != "" {
		title := extractBetween(html, `class="result__title"`, `</a>`)
		if title == "" {
			title = extractBetween(html, `class="result__a"`, `</a>`)
		}
		link := extractBetween(html, `class="result__url"`, `</span>`)
		snippet := extractBetween(html, `class="result__snippet"`, `</span>`)

		if title == "" && link == "" && snippet == "" {
			break
		}

		title = cleanHTML(title)
		link = cleanHTML(link)
		snippet = cleanHTML(snippet)

		if title != "" || snippet != "" {
			results = append(results, searchResult{Title: title, URL: link, Snippet: snippet})
		}

		marker := `class="result__body"`
		idx := strings.Index(html, marker)
		if idx < 0 {
			marker = `class="result--`
			idx = strings.Index(html, marker)
		}
		if idx >= 0 {
			html = html[idx+len(marker):]
		} else {
			html = ""
		}
	}

	return results
}

func extractBetween(s, start, end string) string {
	i := strings.Index(s, start)
	if i < 0 {
		return ""
	}
	s = s[i+len(start):]
	j := strings.Index(s, end)
	if j < 0 {
		return ""
	}
	return s[:j]
}

func cleanHTML(s string) string {
	var b strings.Builder
	inTag := false
	for _, r := range s {
		if r == '<' {
			inTag = true
			continue
		}
		if r == '>' {
			inTag = false
			continue
		}
		if !inTag {
			b.WriteRune(r)
		}
	}
	result := strings.TrimSpace(b.String())
	result = strings.ReplaceAll(result, "&amp;", "&")
	result = strings.ReplaceAll(result, "&lt;", "<")
	result = strings.ReplaceAll(result, "&gt;", ">")
	result = strings.ReplaceAll(result, "&quot;", "\"")
	result = strings.ReplaceAll(result, "&#x27;", "'")
	return result
}

func formatResults(results []searchResult) string {
	var b strings.Builder
	for i, r := range results {
		fmt.Fprintf(&b, "%d. **%s**\n", i+1, r.Title)
		if r.URL != "" {
			fmt.Fprintf(&b, "   %s\n", r.URL)
		}
		if r.Snippet != "" {
			fmt.Fprintf(&b, "   %s\n", r.Snippet)
		}
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}
