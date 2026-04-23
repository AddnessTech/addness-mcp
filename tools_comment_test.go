package main

import (
	"testing"
)

func TestEnsureMentionsInContent(t *testing.T) {
	uuid1 := "550e8400-e29b-41d4-a716-446655440000"
	uuid2 := "660f9500-f39c-52e5-b827-557766551111"

	tests := []struct {
		name         string
		content      string
		mentionUUIDs []string
		want         string
	}{
		{
			name:         "no mentions",
			content:      "確認お願いします",
			mentionUUIDs: nil,
			want:         "確認お願いします",
		},
		{
			name:         "empty mentions slice",
			content:      "確認お願いします",
			mentionUUIDs: []string{},
			want:         "確認お願いします",
		},
		{
			name:         "mention already in content",
			content:      "確認お願いします @" + uuid1,
			mentionUUIDs: []string{uuid1},
			want:         "確認お願いします @" + uuid1,
		},
		{
			name:         "mention missing from content",
			content:      "確認お願いします",
			mentionUUIDs: []string{uuid1},
			want:         "確認お願いします @" + uuid1,
		},
		{
			name:         "multiple mentions missing",
			content:      "確認お願いします",
			mentionUUIDs: []string{uuid1, uuid2},
			want:         "確認お願いします @" + uuid1 + " @" + uuid2,
		},
		{
			name:         "one present one missing",
			content:      "確認お願いします @" + uuid1,
			mentionUUIDs: []string{uuid1, uuid2},
			want:         "確認お願いします @" + uuid1 + " @" + uuid2,
		},
		{
			name:         "case insensitive match",
			content:      "確認お願いします @550E8400-E29B-41D4-A716-446655440000",
			mentionUUIDs: []string{uuid1},
			want:         "確認お願いします @550E8400-E29B-41D4-A716-446655440000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ensureMentionsInContent(tt.content, tt.mentionUUIDs)
			if got != tt.want {
				t.Errorf("ensureMentionsInContent() = %q, want %q", got, tt.want)
			}
		})
	}
}
