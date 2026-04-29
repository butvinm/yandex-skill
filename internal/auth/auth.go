package auth

import (
	"errors"
	"fmt"
	"net/http"
	"os"
)

const (
	defaultTrackerBaseURL = "https://api.tracker.yandex.net"
	defaultWikiBaseURL    = "https://api.wiki.yandex.net"

	envToken          = "YANDEX_TOKEN"
	envOrgID          = "YANDEX_ORG_ID"
	envCloudOrgID     = "YANDEX_CLOUD_ORG_ID" // fallback for users coming from n-r-w/yandex-mcp
	envTenancy        = "YANDEX_TENANCY"
	envTrackerBaseURL = "YANDEX_TRACKER_BASE_URL"
	envWikiBaseURL    = "YANDEX_WIKI_BASE_URL"
)

type Tenancy string

const (
	Cloud Tenancy = "cloud"
	Y360  Tenancy = "360"
)

type Config struct {
	Token          string
	OrgID          string
	Tenancy        Tenancy
	TrackerBaseURL string
	WikiBaseURL    string
}

func Load() (Config, error) {
	tenancy := Tenancy(os.Getenv(envTenancy))
	if tenancy == "" {
		tenancy = Cloud
	}
	if tenancy != Cloud && tenancy != Y360 {
		return Config{}, fmt.Errorf("%s must be 'cloud' or '360', got %q", envTenancy, tenancy)
	}

	orgID := os.Getenv(envOrgID)
	if orgID == "" {
		orgID = os.Getenv(envCloudOrgID)
	}

	c := Config{
		Token:          os.Getenv(envToken),
		OrgID:          orgID,
		Tenancy:        tenancy,
		TrackerBaseURL: os.Getenv(envTrackerBaseURL),
		WikiBaseURL:    os.Getenv(envWikiBaseURL),
	}
	if c.TrackerBaseURL == "" {
		c.TrackerBaseURL = defaultTrackerBaseURL
	}
	if c.WikiBaseURL == "" {
		c.WikiBaseURL = defaultWikiBaseURL
	}
	if c.Token == "" {
		if tenancy == Y360 {
			return Config{}, errors.New(envToken + " not set; for 360, get an OAuth token at oauth.yandex.com")
		}
		return Config{}, errors.New(envToken + " not set; run: export " + envToken + "=$(yc iam create-token)")
	}
	if c.OrgID == "" {
		if tenancy == Y360 {
			return Config{}, errors.New(envOrgID + " not set; find via Yandex Tracker → Administration → Organizations")
		}
		return Config{}, errors.New(envOrgID + " not set; find via: yc organization-manager organization list")
	}
	return c, nil
}

// authPrefix returns the Authorization header prefix for the configured tenancy.
// Cloud uses Bearer (IAM token); 360 uses OAuth (OAuth token).
func (c Config) authPrefix() string {
	if c.Tenancy == Y360 {
		return "OAuth "
	}
	return "Bearer "
}

func (c Config) TrackerHeaders() http.Header {
	h := http.Header{}
	h.Set("Authorization", c.authPrefix()+c.Token)
	h.Set("X-Org-ID", c.OrgID)
	h.Set("Content-Type", "application/json")
	return h
}

func (c Config) WikiHeaders() http.Header {
	h := http.Header{}
	h.Set("Authorization", c.authPrefix()+c.Token)
	if c.Tenancy == Y360 {
		h.Set("X-Org-Id", c.OrgID)
	} else {
		h.Set("X-Cloud-Org-Id", c.OrgID)
	}
	h.Set("Content-Type", "application/json")
	return h
}
