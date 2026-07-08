package pluginpkg

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

// RegistryEntry describes a plugin available in a remote registry.
type RegistryEntry struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Version     string `json:"version"`
	Author      string `json:"author,omitempty"`
	Source      string `json:"source"`
	Homepage    string `json:"homepage,omitempty"`
	Category    string `json:"category,omitempty"`
}

// RegistryIndex is the top-level response from a plugin registry.
type RegistryIndex struct {
	Plugins  []RegistryEntry `json:"plugins"`
	Updated  time.Time       `json:"updated"`
	Registry string          `json:"registry"`
}

// DefaultRegistryURL is the default plugin registry endpoint.
const DefaultRegistryURL = "https://plugins.reames-agent.io/index.json"

// FetchRegistry downloads the plugin index from url (default if empty).
func FetchRegistry(url string, client *http.Client) (*RegistryIndex, error) {
	if url == "" {
		url = DefaultRegistryURL
	}
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("plugin registry unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("plugin registry returned %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	var index RegistryIndex
	if err := json.Unmarshal(body, &index); err != nil {
		return nil, fmt.Errorf("plugin registry format error: %w", err)
	}
	return &index, nil
}

// Search filters registry entries by query (name or description substring).
func Search(index *RegistryIndex, query string) []RegistryEntry {
	if query == "" {
		return index.Plugins
	}
	query = strings.ToLower(query)
	var out []RegistryEntry
	for _, p := range index.Plugins {
		if strings.Contains(strings.ToLower(p.Name), query) ||
			strings.Contains(strings.ToLower(p.Description), query) ||
			strings.Contains(strings.ToLower(p.Category), query) {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// ByCategory groups entries by category.
func ByCategory(index *RegistryIndex) map[string][]RegistryEntry {
	m := make(map[string][]RegistryEntry)
	for _, p := range index.Plugins {
		cat := p.Category
		if cat == "" {
			cat = "other"
		}
		m[cat] = append(m[cat], p)
	}
	return m
}
