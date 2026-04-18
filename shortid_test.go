package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestShortIDCache_ShortenAndResolve(t *testing.T) {
	c := &ShortIDCache{
		toFull:  make(map[string]string),
		toShort: make(map[string]string),
		minLen:  8,
	}

	fullID := "e79f0f16-1234-5678-abcd-123456789abc"
	short := c.Shorten(fullID)

	if short != "e79f0f16" {
		t.Fatalf("expected short ID 'e79f0f16', got %q", short)
	}

	resolved, err := c.Resolve(short)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved != fullID {
		t.Fatalf("expected resolved %q, got %q", fullID, resolved)
	}
}

func TestShortIDCache_ResolveUnknownReturnsError(t *testing.T) {
	c := &ShortIDCache{
		toFull:  make(map[string]string),
		toShort: make(map[string]string),
		minLen:  8,
	}

	_, err := c.Resolve("deadbeef")
	if err == nil {
		t.Fatal("expected error for unknown short ID, got nil")
	}
}

func TestShortIDCache_ResolveFullUUIDPassthrough(t *testing.T) {
	c := &ShortIDCache{
		toFull:  make(map[string]string),
		toShort: make(map[string]string),
		minLen:  8,
	}

	fullID := "e79f0f16-1234-5678-abcd-123456789abc"
	resolved, err := c.Resolve(fullID)
	if err != nil {
		t.Fatalf("unexpected error for full UUID: %v", err)
	}
	if resolved != fullID {
		t.Fatalf("expected %q, got %q", fullID, resolved)
	}
}

func TestShortIDCache_PersistenceAcrossInstances(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "shortid_cache.json")

	// First instance: shorten a UUID
	c1 := &ShortIDCache{
		toFull:    make(map[string]string),
		toShort:   make(map[string]string),
		minLen:    8,
		cachePath: cachePath,
	}
	fullID := "e79f0f16-1234-5678-abcd-123456789abc"
	short := c1.Shorten(fullID)
	if short != "e79f0f16" {
		t.Fatalf("expected short ID 'e79f0f16', got %q", short)
	}

	// Verify file was written
	if _, err := os.Stat(cachePath); os.IsNotExist(err) {
		t.Fatal("cache file was not created")
	}

	// Second instance: load from file and resolve
	c2 := &ShortIDCache{
		toFull:    make(map[string]string),
		toShort:   make(map[string]string),
		minLen:    8,
		cachePath: cachePath,
	}
	c2.loadFromFile()

	resolved, err := c2.Resolve(short)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved != fullID {
		t.Fatalf("expected resolved %q after reload, got %q", fullID, resolved)
	}
}

func TestShortIDCache_CollisionHandling(t *testing.T) {
	c := &ShortIDCache{
		toFull:  make(map[string]string),
		toShort: make(map[string]string),
		minLen:  8,
	}

	// Two UUIDs with same first 8 hex chars
	id1 := "e79f0f16-1234-5678-abcd-111111111111"
	id2 := "e79f0f16-1234-5678-abcd-222222222222"

	short1 := c.Shorten(id1)
	short2 := c.Shorten(id2)

	if short1 == short2 {
		t.Fatalf("collision: both got %q", short1)
	}

	r1, err := c.Resolve(short1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r1 != id1 {
		t.Fatalf("short1 resolved to %q, expected %q", r1, id1)
	}
	r2, err := c.Resolve(short2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r2 != id2 {
		t.Fatalf("short2 resolved to %q, expected %q", r2, id2)
	}
}

func TestShortIDCache_EmptyInput(t *testing.T) {
	c := &ShortIDCache{
		toFull:  make(map[string]string),
		toShort: make(map[string]string),
		minLen:  8,
	}

	if got := c.Shorten(""); got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}

	if got := c.ShortenOptional(nil); got != "" {
		t.Fatalf("expected empty string for nil, got %q", got)
	}
}

func TestShortIDCache_CorruptedFileIgnored(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "shortid_cache.json")

	// Write corrupted data
	if err := os.WriteFile(cachePath, []byte("not json"), 0600); err != nil {
		t.Fatal(err)
	}

	c := &ShortIDCache{
		toFull:    make(map[string]string),
		toShort:   make(map[string]string),
		minLen:    8,
		cachePath: cachePath,
	}
	c.loadFromFile()

	// Should work normally with empty cache
	if len(c.toFull) != 0 {
		t.Fatalf("expected empty cache after corrupted file, got %d entries", len(c.toFull))
	}
}

func TestShortIDCache_ExpandMentionsInContent(t *testing.T) {
	c := &ShortIDCache{
		toFull:  make(map[string]string),
		toShort: make(map[string]string),
		minLen:  8,
	}

	fullID1 := "e79f0f16-1234-5678-abcd-111111111111"
	fullID2 := "abcdef01-2345-6789-abcd-222222222222"
	short1 := c.Shorten(fullID1) // e79f0f16
	short2 := c.Shorten(fullID2) // abcdef01

	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "single short ID mention",
			content: "@" + short1 + " さん、確認お願いします",
			want:    "@" + fullID1 + " さん、確認お願いします",
		},
		{
			name:    "multiple short ID mentions",
			content: "@" + short1 + " @" + short2 + " ご確認ください",
			want:    "@" + fullID1 + " @" + fullID2 + " ご確認ください",
		},
		{
			name:    "full UUID left unchanged",
			content: "@" + fullID1 + " already expanded",
			want:    "@" + fullID1 + " already expanded",
		},
		{
			name:    "unknown short ID left unchanged",
			content: "@deadbeef not in cache",
			want:    "@deadbeef not in cache",
		},
		{
			name:    "no mentions",
			content: "plain text without mentions",
			want:    "plain text without mentions",
		},
		{
			name:    "empty content",
			content: "",
			want:    "",
		},
		{
			name:    "mixed short and full UUIDs",
			content: "@" + short1 + " and @" + fullID2,
			want:    "@" + fullID1 + " and @" + fullID2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := c.ExpandMentionsInContent(tt.content)
			if got != tt.want {
				t.Errorf("ExpandMentionsInContent(%q)\n  got  %q\n  want %q", tt.content, got, tt.want)
			}
		})
	}
}

func TestShortIDCache_ResolveOrFallback(t *testing.T) {
	c := &ShortIDCache{
		toFull:  make(map[string]string),
		toShort: make(map[string]string),
		minLen:  8,
	}

	// Unknown short ID: falls back to input
	got := c.resolveOrFallback("deadbeef")
	if got != "deadbeef" {
		t.Fatalf("expected fallback to input, got %q", got)
	}

	// Known short ID: resolves
	fullID := "e79f0f16-1234-5678-abcd-123456789abc"
	short := c.Shorten(fullID)
	got = c.resolveOrFallback(short)
	if got != fullID {
		t.Fatalf("expected %q, got %q", fullID, got)
	}
}
