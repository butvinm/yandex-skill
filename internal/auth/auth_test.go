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
			name: "missing token cloud (default tenancy)",
			env: map[string]string{
				"YANDEX_TOKEN":   "",
				"YANDEX_ORG_ID":  "org123",
			},
			wantErr: "YANDEX_TOKEN not set; run: export",
		},
		{
			name: "missing token 360 hints at oauth",
			env: map[string]string{
				"YANDEX_TENANCY": "360",
				"YANDEX_TOKEN":   "",
				"YANDEX_ORG_ID":  "org123",
			},
			wantErr: "for 360, get an OAuth token at oauth.yandex.com",
		},
		{
			name: "missing org id cloud",
			env: map[string]string{
				"YANDEX_TOKEN":  "t1.xxx",
				"YANDEX_ORG_ID": "",
			},
			wantErr: "yc organization-manager organization list",
		},
		{
			name: "missing org id 360",
			env: map[string]string{
				"YANDEX_TENANCY": "360",
				"YANDEX_TOKEN":   "y0_xxx",
				"YANDEX_ORG_ID":  "",
			},
			wantErr: "Yandex Tracker → Administration → Organizations",
		},
		{
			name: "invalid tenancy rejected",
			env: map[string]string{
				"YANDEX_TENANCY": "yandexCloud",
				"YANDEX_TOKEN":   "t1.xxx",
				"YANDEX_ORG_ID":  "org",
			},
			wantErr: "must be 'cloud' or '360'",
		},
		{
			name: "default tenancy is cloud",
			env: map[string]string{
				"YANDEX_TOKEN":  "t1.xxx",
				"YANDEX_ORG_ID": "org123",
			},
			wantCheck: func(t *testing.T, c Config) {
				if c.Tenancy != Cloud {
					t.Errorf("Tenancy = %q, want cloud", c.Tenancy)
				}
			},
		},
		{
			name: "tenancy 360 honored",
			env: map[string]string{
				"YANDEX_TENANCY": "360",
				"YANDEX_TOKEN":   "y0_xxx",
				"YANDEX_ORG_ID":  "org123",
			},
			wantCheck: func(t *testing.T, c Config) {
				if c.Tenancy != Y360 {
					t.Errorf("Tenancy = %q, want 360", c.Tenancy)
				}
			},
		},
		{
			name: "defaults applied when base URLs unset",
			env: map[string]string{
				"YANDEX_TOKEN":  "t1.xxx",
				"YANDEX_ORG_ID": "org123",
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
				"YANDEX_TOKEN":            "t1.xxx",
				"YANDEX_ORG_ID":           "org123",
				"YANDEX_TRACKER_BASE_URL": "https://t.example",
				"YANDEX_WIKI_BASE_URL":    "https://w.example",
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
			for _, k := range []string{
				"YANDEX_TOKEN", "YANDEX_ORG_ID",
				"YANDEX_TENANCY", "YANDEX_TRACKER_BASE_URL", "YANDEX_WIKI_BASE_URL",
			} {
				t.Setenv(k, "")
			}
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

func TestTrackerHeaders_Cloud(t *testing.T) {
	c := Config{Token: "t1.xxx", OrgID: "org123", Tenancy: Cloud}
	h := c.TrackerHeaders()
	if got := h.Get("Authorization"); got != "Bearer t1.xxx" {
		t.Errorf("Authorization = %q, want Bearer", got)
	}
	if got := h.Get("X-Org-ID"); got != "org123" {
		t.Errorf("X-Org-ID = %q", got)
	}
	if got := h.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q", got)
	}
}

func TestTrackerHeaders_360(t *testing.T) {
	c := Config{Token: "y0_xxx", OrgID: "org123", Tenancy: Y360}
	h := c.TrackerHeaders()
	if got := h.Get("Authorization"); got != "OAuth y0_xxx" {
		t.Errorf("Authorization = %q, want OAuth prefix", got)
	}
	if got := h.Get("X-Org-ID"); got != "org123" {
		t.Errorf("X-Org-ID = %q (Tracker uses same header for both tenancies)", got)
	}
}

func TestWikiHeaders_Cloud(t *testing.T) {
	c := Config{Token: "t1.xxx", OrgID: "org123", Tenancy: Cloud}
	h := c.WikiHeaders()
	if got := h.Get("Authorization"); got != "Bearer t1.xxx" {
		t.Errorf("Authorization = %q, want Bearer", got)
	}
	if got := h.Get("X-Cloud-Org-Id"); got != "org123" {
		t.Errorf("X-Cloud-Org-Id = %q (Cloud must use X-Cloud-Org-Id)", got)
	}
	if h.Get("X-Org-Id") != "" {
		t.Error("Cloud wiki must not set X-Org-Id")
	}
}

func TestWikiHeaders_360(t *testing.T) {
	c := Config{Token: "y0_xxx", OrgID: "org123", Tenancy: Y360}
	h := c.WikiHeaders()
	if got := h.Get("Authorization"); got != "OAuth y0_xxx" {
		t.Errorf("Authorization = %q, want OAuth prefix", got)
	}
	if got := h.Get("X-Org-Id"); got != "org123" {
		t.Errorf("X-Org-Id = %q (360 wiki must use X-Org-Id, not X-Cloud-Org-Id)", got)
	}
	if h.Get("X-Cloud-Org-Id") != "" {
		t.Error("360 wiki must not set X-Cloud-Org-Id")
	}
}

func TestZeroValueTenancyDefaultsToCloud(t *testing.T) {
	// Existing client tests construct Config{} directly without setting
	// Tenancy. Make sure that still picks the cloud header set.
	c := Config{Token: "t", OrgID: "o"} // Tenancy=""
	if c.TrackerHeaders().Get("Authorization") != "Bearer t" {
		t.Error("zero-value Tenancy must behave as Cloud (Bearer)")
	}
	if c.WikiHeaders().Get("X-Cloud-Org-Id") != "o" {
		t.Error("zero-value Tenancy must behave as Cloud (X-Cloud-Org-Id)")
	}
}
