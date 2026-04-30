package auth

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWriteThenRead_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "iam-token.json")

	want := cachedToken{
		Token:      "t1.abcdef",
		AcquiredAt: time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC),
	}
	if err := writeCachedToken(path, want); err != nil {
		t.Fatalf("writeCachedToken: %v", err)
	}
	got, err := readCachedToken(path)
	if err != nil {
		t.Fatalf("readCachedToken: %v", err)
	}
	if got.Token != want.Token {
		t.Errorf("Token = %q, want %q", got.Token, want.Token)
	}
	if !got.AcquiredAt.Equal(want.AcquiredAt) {
		t.Errorf("AcquiredAt = %v, want %v", got.AcquiredAt, want.AcquiredAt)
	}
}

func TestRead_MissingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nope.json")
	_, err := readCachedToken(path)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("err = %v, want fs.ErrNotExist", err)
	}
}

func TestRead_MalformedJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := readCachedToken(path)
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
	if !strings.Contains(err.Error(), "parsing cached token") {
		t.Errorf("err = %v, want wrapped parse error", err)
	}
}

func TestRead_EmptyTokenField(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.json")
	if err := os.WriteFile(path, []byte(`{"token":"","acquired_at":"2026-04-30T12:00:00Z"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := readCachedToken(path)
	if err == nil {
		t.Fatal("expected error for empty token")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("err = %v, want error mentioning empty", err)
	}
}

func TestWrite_FilePermissions(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "yandex-cli")
	path := filepath.Join(subdir, "iam-token.json")

	ct := cachedToken{Token: "t1.x", AcquiredAt: time.Now().UTC()}
	if err := writeCachedToken(path, ct); err != nil {
		t.Fatalf("writeCachedToken: %v", err)
	}

	dirInfo, err := os.Stat(subdir)
	if err != nil {
		t.Fatal(err)
	}
	if dirInfo.Mode().Perm() != 0o700 {
		t.Errorf("dir perms = %o, want 0700", dirInfo.Mode().Perm())
	}

	fileInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fileInfo.Mode().Perm() != 0o600 {
		t.Errorf("file perms = %o, want 0600", fileInfo.Mode().Perm())
	}
}

func TestWrite_OverwritesAtomically(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "iam-token.json")

	first := cachedToken{Token: "t1.first", AcquiredAt: time.Now().UTC()}
	if err := writeCachedToken(path, first); err != nil {
		t.Fatalf("first write: %v", err)
	}
	second := cachedToken{Token: "t1.second", AcquiredAt: time.Now().UTC()}
	if err := writeCachedToken(path, second); err != nil {
		t.Fatalf("second write: %v", err)
	}

	got, err := readCachedToken(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Token != "t1.second" {
		t.Errorf("Token = %q, want second write to win", got.Token)
	}

	if _, err := os.Stat(path + ".tmp"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf(".tmp file should not linger after rename, got %v", err)
	}
}

func TestParseRefreshPeriod(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want time.Duration
	}{
		{"empty defaults to 10h", "", 10 * time.Hour},
		{"valid 5", "5", 5 * time.Hour},
		{"valid 12 (max)", "12", 12 * time.Hour},
		{"clamps over 12", "100", 12 * time.Hour},
		{"zero falls back", "0", 10 * time.Hour},
		{"negative falls back", "-3", 10 * time.Hour},
		{"non-numeric falls back", "abc", 10 * time.Hour},
		{"trailing-h falls back", "12h", 10 * time.Hour},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseRefreshPeriod(tt.in); got != tt.want {
				t.Errorf("parseRefreshPeriod(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestIsFresh(t *testing.T) {
	now := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name          string
		acquiredAt    time.Time
		refreshPeriod time.Duration
		want          bool
	}{
		{"age below period", now.Add(-1 * time.Hour), 10 * time.Hour, true},
		{"age equals period", now.Add(-10 * time.Hour), 10 * time.Hour, false},
		{"age exceeds period", now.Add(-11 * time.Hour), 10 * time.Hour, false},
		{"just acquired", now, 10 * time.Hour, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ct := cachedToken{Token: "t", AcquiredAt: tt.acquiredAt}
			if got := ct.isFresh(tt.refreshPeriod, now); got != tt.want {
				t.Errorf("isFresh = %v, want %v", got, tt.want)
			}
		})
	}
}
