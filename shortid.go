package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// shortIDMentionPattern matches @shortID in content (hex-only, no dashes).
// Full UUIDs are handled by a dash-check in ExpandMentionsInContent, not by this pattern.
var shortIDMentionPattern = regexp.MustCompile(`(?i)@([a-f0-9]{8,32})\b`)

// isUUID returns true if s looks like a full UUID.
func isUUID(s string) bool {
	return uuidPattern.MatchString(strings.ToLower(s))
}

// ShortIDCache maps short IDs to full UUIDs within a session.
// Uses first 8 chars of the hex portion by default, extending on collision.
// The cache is persisted to disk so that it survives server restarts.
type ShortIDCache struct {
	mu        sync.RWMutex
	toFull    map[string]string // short -> full UUID
	toShort   map[string]string // full UUID -> short
	minLen    int
	cachePath string // empty = no persistence
}

// shortIDCachePath returns the default file path for the persistent cache.
func shortIDCachePath() string {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(cacheDir, "addness-mcp", "shortid_cache.json")
}

func NewShortIDCache() *ShortIDCache {
	c := &ShortIDCache{
		toFull:    make(map[string]string),
		toShort:   make(map[string]string),
		minLen:    8,
		cachePath: shortIDCachePath(),
	}
	c.loadFromFile()
	return c
}

// persistedCache is the JSON structure written to disk.
type persistedCache struct {
	ToFull  map[string]string `json:"toFull"`
	ToShort map[string]string `json:"toShort"`
}

func (c *ShortIDCache) loadFromFile() {
	if c.cachePath == "" {
		return
	}
	data, err := os.ReadFile(c.cachePath)
	if err != nil {
		return
	}
	var pc persistedCache
	if err := json.Unmarshal(data, &pc); err != nil {
		return
	}
	if pc.ToFull != nil {
		c.toFull = pc.ToFull
	}
	if pc.ToShort != nil {
		c.toShort = pc.ToShort
	}
}

// saveToFile writes the cache to disk atomically.
// Must be called with c.mu held.
func (c *ShortIDCache) saveToFile() {
	if c.cachePath == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(c.cachePath), 0700); err != nil {
		return
	}
	pc := persistedCache{
		ToFull:  c.toFull,
		ToShort: c.toShort,
	}
	data, err := json.Marshal(pc)
	if err != nil {
		return
	}
	tmp := c.cachePath + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		_ = os.Remove(tmp)
		return
	}
	if err := os.Rename(tmp, c.cachePath); err != nil {
		_ = os.Remove(tmp)
	}
}

// Shorten registers a full UUID and returns its short form.
func (c *ShortIDCache) Shorten(fullID string) string {
	if fullID == "" {
		return ""
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	if short, ok := c.toShort[fullID]; ok {
		return short
	}

	clean := strings.ReplaceAll(fullID, "-", "")
	for length := c.minLen; length <= len(clean); length++ {
		candidate := clean[:length]
		if existing, ok := c.toFull[candidate]; !ok || existing == fullID {
			c.toFull[candidate] = fullID
			c.toShort[fullID] = candidate
			c.saveToFile()
			return candidate
		}
	}

	c.toFull[clean] = fullID
	c.toShort[fullID] = clean
	c.saveToFile()
	return clean
}

// Resolve converts a short ID back to full UUID.
// Returns an error if the input looks like a short ID but is not in the cache.
// Full UUIDs are passed through unchanged.
func (c *ShortIDCache) Resolve(shortOrFull string) (string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if full, ok := c.toFull[shortOrFull]; ok {
		return full, nil
	}
	if isUUID(shortOrFull) {
		return shortOrFull, nil
	}
	return "", fmt.Errorf("unknown short ID %q: call list_my_goals or list_members first to populate the cache", shortOrFull)
}

// resolveOrFallback resolves a short ID, falling back to the input on error.
// Use this only for internal callers where the ID was just Shorten'd in the
// same session (e.g. fetchGoalContexts).
func (c *ShortIDCache) resolveOrFallback(shortID string) string {
	full, err := c.Resolve(shortID)
	if err != nil {
		return shortID
	}
	return full
}

// ExpandMentionsInContent replaces @shortID patterns in content with @fullUUID.
// Known short IDs are expanded; unknown patterns are left unchanged.
// Full UUIDs (with dashes) are not affected — if the matched hex is followed by
// a dash (indicating a UUID fragment), the match is skipped.
func (c *ShortIDCache) ExpandMentionsInContent(content string) string {
	matches := shortIDMentionPattern.FindAllStringSubmatchIndex(content, -1)
	if len(matches) == 0 {
		return content
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	var b strings.Builder
	b.Grow(len(content))
	prev := 0
	for _, loc := range matches {
		// loc[0]:loc[1] = full match (@shortID), loc[2]:loc[3] = capture group (shortID)
		matchEnd := loc[1]
		// Skip if followed by '-' (part of a full UUID like @xxxxxxxx-xxxx-...)
		if matchEnd < len(content) && content[matchEnd] == '-' {
			continue
		}
		shortID := strings.ToLower(content[loc[2]:loc[3]])
		full, ok := c.toFull[shortID]
		if !ok {
			continue
		}
		b.WriteString(content[prev:loc[0]])
		b.WriteByte('@')
		b.WriteString(full)
		prev = matchEnd
	}
	b.WriteString(content[prev:])
	return b.String()
}

// ShortenOptional handles nullable string pointer IDs.
func (c *ShortIDCache) ShortenOptional(fullID *string) string {
	if fullID == nil {
		return ""
	}
	return c.Shorten(*fullID)
}
