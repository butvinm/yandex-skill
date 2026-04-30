package auth

import (
	"errors"
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
			name: "neither org var set",
			env: map[string]string{
				"YANDEX_TOKEN": "t1.xxx",
			},
			wantErr: "set YANDEX_CLOUD_ORG_ID for Yandex Cloud Organization, or YANDEX_ORG_ID for Yandex 360",
		},
		{
			name: "both org vars set",
			env: map[string]string{
				"YANDEX_TOKEN":        "t1.xxx",
				"YANDEX_CLOUD_ORG_ID": "cloud123",
				"YANDEX_ORG_ID":       "y360_456",
			},
			wantErr: "set exactly one of YANDEX_CLOUD_ORG_ID (Cloud) or YANDEX_ORG_ID (360), not both",
		},
		{
			name: "missing token cloud",
			env: map[string]string{
				"YANDEX_TOKEN":        "",
				"YANDEX_CLOUD_ORG_ID": "cloud123",
			},
			wantErr: "YANDEX_TOKEN not set; run: export",
		},
		{
			name: "missing token 360 hints at oauth",
			env: map[string]string{
				"YANDEX_TOKEN":  "",
				"YANDEX_ORG_ID": "y360_456",
			},
			wantErr: "for 360, get an OAuth token at oauth.yandex.com",
		},
		{
			name: "cloud tenancy detected from YANDEX_CLOUD_ORG_ID",
			env: map[string]string{
				"YANDEX_TOKEN":        "t1.xxx",
				"YANDEX_CLOUD_ORG_ID": "cloud123",
			},
			wantCheck: func(t *testing.T, c Config) {
				if c.Tenancy != Cloud {
					t.Errorf("Tenancy = %q, want cloud", c.Tenancy)
				}
				if c.OrgID != "cloud123" {
					t.Errorf("OrgID = %q", c.OrgID)
				}
			},
		},
		{
			name: "360 tenancy detected from YANDEX_ORG_ID",
			env: map[string]string{
				"YANDEX_TOKEN":  "y0_xxx",
				"YANDEX_ORG_ID": "y360_456",
			},
			wantCheck: func(t *testing.T, c Config) {
				if c.Tenancy != Y360 {
					t.Errorf("Tenancy = %q, want 360", c.Tenancy)
				}
				if c.OrgID != "y360_456" {
					t.Errorf("OrgID = %q", c.OrgID)
				}
			},
		},
		{
			name: "defaults applied when base URLs unset",
			env: map[string]string{
				"YANDEX_TOKEN":        "t1.xxx",
				"YANDEX_CLOUD_ORG_ID": "cloud123",
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
				"YANDEX_CLOUD_ORG_ID":     "cloud123",
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
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, k := range []string{
				"YANDEX_TOKEN", "YANDEX_ORG_ID", "YANDEX_CLOUD_ORG_ID",
				"YANDEX_TRACKER_BASE_URL", "YANDEX_WIKI_BASE_URL",
				"YANDEX_USE_YC",
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
	if got := h.Get("X-Cloud-Org-ID"); got != "org123" {
		t.Errorf("X-Cloud-Org-ID = %q (Cloud Tracker must use X-Cloud-Org-ID)", got)
	}
	if h.Get("X-Org-ID") != "" {
		t.Error("Cloud tracker must not set X-Org-ID")
	}
}

func TestTrackerHeaders_360(t *testing.T) {
	c := Config{Token: "y0_xxx", OrgID: "org123", Tenancy: Y360}
	h := c.TrackerHeaders()
	if got := h.Get("Authorization"); got != "OAuth y0_xxx" {
		t.Errorf("Authorization = %q, want OAuth prefix", got)
	}
	if got := h.Get("X-Org-ID"); got != "org123" {
		t.Errorf("X-Org-ID = %q (360 Tracker must use X-Org-ID)", got)
	}
	if h.Get("X-Cloud-Org-ID") != "" {
		t.Error("360 tracker must not set X-Cloud-Org-ID")
	}
}

func TestWikiHeaders_Cloud(t *testing.T) {
	c := Config{Token: "t1.xxx", OrgID: "org123", Tenancy: Cloud}
	h := c.WikiHeaders()
	if got := h.Get("Authorization"); got != "Bearer t1.xxx" {
		t.Errorf("Authorization = %q, want Bearer", got)
	}
	if got := h.Get("X-Cloud-Org-ID"); got != "org123" {
		t.Errorf("X-Cloud-Org-ID = %q (Cloud Wiki must use X-Cloud-Org-ID)", got)
	}
	if h.Get("X-Org-ID") != "" {
		t.Error("Cloud wiki must not set X-Org-ID")
	}
}

func TestWikiHeaders_360(t *testing.T) {
	c := Config{Token: "y0_xxx", OrgID: "org123", Tenancy: Y360}
	h := c.WikiHeaders()
	if got := h.Get("Authorization"); got != "OAuth y0_xxx" {
		t.Errorf("Authorization = %q, want OAuth prefix", got)
	}
	if got := h.Get("X-Org-ID"); got != "org123" {
		t.Errorf("X-Org-ID = %q (360 Wiki must use X-Org-ID)", got)
	}
	if h.Get("X-Cloud-Org-ID") != "" {
		t.Error("360 wiki must not set X-Cloud-Org-ID")
	}
}

func TestZeroValueTenancyDefaultsToCloud(t *testing.T) {
	c := Config{Token: "t", OrgID: "o"} // Tenancy=""
	if c.TrackerHeaders().Get("Authorization") != "Bearer t" {
		t.Error("zero-value Tenancy must behave as Cloud (Bearer)")
	}
	if c.TrackerHeaders().Get("X-Cloud-Org-ID") != "o" {
		t.Error("zero-value Tenancy must behave as Cloud (X-Cloud-Org-ID for Tracker)")
	}
	if c.WikiHeaders().Get("X-Cloud-Org-ID") != "o" {
		t.Error("zero-value Tenancy must behave as Cloud (X-Cloud-Org-ID for Wiki)")
	}
}

func clearAuthEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"YANDEX_TOKEN", "YANDEX_ORG_ID", "YANDEX_CLOUD_ORG_ID",
		"YANDEX_TRACKER_BASE_URL", "YANDEX_WIKI_BASE_URL",
		"YANDEX_USE_YC",
	} {
		t.Setenv(k, "")
	}
}

func TestLoad_YCFallback_OptInDisabled_ReturnsExistingError(t *testing.T) {
	clearAuthEnv(t)
	t.Setenv("YANDEX_CLOUD_ORG_ID", "cloud123")
	fake := &fakeYCExecutor{out: []byte("t1.should-not-be-used")}
	swapYCExec(t, fake)

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when YANDEX_TOKEN unset and YANDEX_USE_YC not set")
	}
	if !strings.Contains(err.Error(), "YANDEX_TOKEN not set; run: export") {
		t.Errorf("error = %q, want existing manual-export hint", err.Error())
	}
	if !strings.Contains(err.Error(), "YANDEX_USE_YC=1") {
		t.Errorf("error = %q, want hint about YANDEX_USE_YC opt-in", err.Error())
	}
	if fake.called {
		t.Error("yc executor must not be called when YANDEX_USE_YC is unset")
	}
}

func TestLoad_YCFallback_OptInSuccess_PopulatesToken(t *testing.T) {
	clearAuthEnv(t)
	t.Setenv("YANDEX_CLOUD_ORG_ID", "cloud123")
	t.Setenv("YANDEX_USE_YC", "1")
	swapYCExec(t, &fakeYCExecutor{out: []byte("t1.from-yc\n")})

	c, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Token != "t1.from-yc" {
		t.Errorf("Token = %q, want %q", c.Token, "t1.from-yc")
	}
	if c.Tenancy != Cloud {
		t.Errorf("Tenancy = %q, want cloud", c.Tenancy)
	}
}

func TestLoad_YCFallback_OptInFailure_WrapsError(t *testing.T) {
	clearAuthEnv(t)
	t.Setenv("YANDEX_CLOUD_ORG_ID", "cloud123")
	t.Setenv("YANDEX_USE_YC", "1")
	swapYCExec(t, &fakeYCExecutor{err: errors.New("exec: \"yc\": executable file not found in $PATH")})

	_, err := Load()
	if err == nil {
		t.Fatal("expected wrapped error from yc fallback failure")
	}
	if !strings.Contains(err.Error(), "yc fallback failed") {
		t.Errorf("error = %q, want 'yc fallback failed' marker", err.Error())
	}
	if !strings.Contains(err.Error(), "executable file not found") {
		t.Errorf("error = %q, want underlying yc error included", err.Error())
	}
}

func TestLoad_YCFallback_360_NotInvoked(t *testing.T) {
	clearAuthEnv(t)
	t.Setenv("YANDEX_ORG_ID", "y360_456")
	t.Setenv("YANDEX_USE_YC", "1") // must be ignored on 360
	fake := &fakeYCExecutor{out: []byte("t1.should-not-be-used")}
	swapYCExec(t, fake)

	_, err := Load()
	if err == nil {
		t.Fatal("expected 360 OAuth error, got nil")
	}
	if !strings.Contains(err.Error(), "for 360, get an OAuth token") {
		t.Errorf("error = %q, want 360 OAuth hint", err.Error())
	}
	if fake.called {
		t.Error("yc executor must not be called on 360 tenancy (yc cannot mint OAuth tokens)")
	}
}

func TestLoad_YCFallback_UserTokenWins(t *testing.T) {
	clearAuthEnv(t)
	t.Setenv("YANDEX_TOKEN", "t1.user-supplied")
	t.Setenv("YANDEX_CLOUD_ORG_ID", "cloud123")
	t.Setenv("YANDEX_USE_YC", "1")
	fake := &fakeYCExecutor{out: []byte("t1.from-yc")}
	swapYCExec(t, fake)

	c, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Token != "t1.user-supplied" {
		t.Errorf("Token = %q, want user-supplied to win", c.Token)
	}
	if fake.called {
		t.Error("yc executor must not be called when YANDEX_TOKEN is already set")
	}
}
