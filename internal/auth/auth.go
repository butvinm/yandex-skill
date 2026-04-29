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
	envCloudOrgID     = "YANDEX_CLOUD_ORG_ID" // presence implies Cloud tenancy
	envOrgID          = "YANDEX_ORG_ID"       // presence implies 360 tenancy
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
	cloudID := os.Getenv(envCloudOrgID)
	org360ID := os.Getenv(envOrgID)

	if cloudID != "" && org360ID != "" {
		return Config{}, fmt.Errorf("set exactly one of %s (Cloud) or %s (360), not both", envCloudOrgID, envOrgID)
	}

	var tenancy Tenancy
	var orgID string
	switch {
	case cloudID != "":
		tenancy = Cloud
		orgID = cloudID
	case org360ID != "":
		tenancy = Y360
		orgID = org360ID
	default:
		return Config{}, fmt.Errorf("set %s for Yandex Cloud Organization, or %s for Yandex 360 for Business", envCloudOrgID, envOrgID)
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

// orgHeaderName returns the org-id header name per tenancy. Both Tracker and
// Wiki follow the same pattern: Cloud uses X-Cloud-Org-ID, 360 uses X-Org-ID.
func (c Config) orgHeaderName() string {
	if c.Tenancy == Y360 {
		return "X-Org-ID"
	}
	return "X-Cloud-Org-ID"
}

func (c Config) TrackerHeaders() http.Header {
	h := http.Header{}
	h.Set("Authorization", c.authPrefix()+c.Token)
	h.Set(c.orgHeaderName(), c.OrgID)
	h.Set("Content-Type", "application/json")
	return h
}

func (c Config) WikiHeaders() http.Header {
	h := http.Header{}
	h.Set("Authorization", c.authPrefix()+c.Token)
	h.Set(c.orgHeaderName(), c.OrgID)
	h.Set("Content-Type", "application/json")
	return h
}
