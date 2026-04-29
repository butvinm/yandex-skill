package auth

import (
	"strings"
	"testing"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		name      string
		env       map[string]string
		wantErr   string
		wantCheck func(t *testing.T, c Config)
	}{
		{
			name: "missing token",
			env: map[string]string{
				"YANDEX_TOKEN":        "",
				"YANDEX_CLOUD_ORG_ID": "org123",
			},
			wantErr: "YANDEX_TOKEN not set",
		},
		{
			name: "missing org id",
			env: map[string]string{
				"YANDEX_TOKEN":        "t1.xxx",
				"YANDEX_CLOUD_ORG_ID": "",
			},
			wantErr: "YANDEX_CLOUD_ORG_ID not set",
		},
		{
			name: "defaults applied when base URLs unset",
			env: map[string]string{
				"YANDEX_TOKEN":        "t1.xxx",
				"YANDEX_CLOUD_ORG_ID": "org123",
			},
			wantCheck: func(t *testing.T, c Config) {
				if c.TrackerBaseURL != "https://api.tracker.yandex.net" {
					t.Errorf("tracker default = %q", c.TrackerBaseURL)
				}
				if c.WikiBaseURL != "https://api.wiki.yandex.net" {
					t.Errorf("wiki default = %q", c.WikiBaseURL)
				}
			},
		},
		{
			name: "explicit base URLs override defaults",
			env: map[string]string{
				"YANDEX_TOKEN":             "t1.xxx",
				"YANDEX_CLOUD_ORG_ID":      "org123",
				"YANDEX_TRACKER_BASE_URL":  "https://t.example",
				"YANDEX_WIKI_BASE_URL":     "https://w.example",
			},
			wantCheck: func(t *testing.T, c Config) {
				if c.TrackerBaseURL != "https://t.example" {
					t.Errorf("tracker = %q", c.TrackerBaseURL)
				}
				if c.WikiBaseURL != "https://w.example" {
					t.Errorf("wiki = %q", c.WikiBaseURL)
				}
				if c.Token != "t1.xxx" || c.OrgID != "org123" {
					t.Errorf("token=%q orgid=%q", c.Token, c.OrgID)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			c, err := Load()
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error %q does not contain %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			tt.wantCheck(t, c)
		})
	}
}

func TestTrackerHeaders(t *testing.T) {
	c := Config{Token: "t1.xxx", OrgID: "org123"}
	h := c.TrackerHeaders()
	if got := h.Get("Authorization"); got != "Bearer t1.xxx" {
		t.Errorf("Authorization = %q", got)
	}
	if got := h.Get("X-Org-ID"); got != "org123" {
		t.Errorf("X-Org-ID = %q", got)
	}
	if got := h.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q", got)
	}
}

func TestWikiHeaders(t *testing.T) {
	c := Config{Token: "t1.xxx", OrgID: "org123"}
	h := c.WikiHeaders()
	if got := h.Get("Authorization"); got != "Bearer t1.xxx" {
		t.Errorf("Authorization = %q", got)
	}
	if got := h.Get("X-Cloud-Org-Id"); got != "org123" {
		t.Errorf("X-Cloud-Org-Id = %q", got)
	}
	if got := h.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q", got)
	}
	if h.Get("X-Org-ID") != "" {
		t.Error("Wiki must not set X-Org-ID; got non-empty")
	}
}

func TestTrackerVsWikiHeadersDiffer(t *testing.T) {
	c := Config{Token: "t1.xxx", OrgID: "org123"}
	if c.TrackerHeaders().Get("X-Org-ID") == "" {
		t.Error("tracker must use X-Org-ID")
	}
	if c.WikiHeaders().Get("X-Cloud-Org-Id") == "" {
		t.Error("wiki must use X-Cloud-Org-Id")
	}
}
