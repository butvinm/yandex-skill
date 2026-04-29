package auth

import (
	"errors"
	"net/http"
	"os"
)

const (
	defaultTrackerBaseURL = "https://api.tracker.yandex.net"
	defaultWikiBaseURL    = "https://api.wiki.yandex.net"

	envToken          = "YANDEX_TOKEN"
	envOrgID          = "YANDEX_CLOUD_ORG_ID"
	envTrackerBaseURL = "YANDEX_TRACKER_BASE_URL"
	envWikiBaseURL    = "YANDEX_WIKI_BASE_URL"
)

type Config struct {
	Token          string
	OrgID          string
	TrackerBaseURL string
	WikiBaseURL    string
}

func Load() (Config, error) {
	c := Config{
		Token:          os.Getenv(envToken),
		OrgID:          os.Getenv(envOrgID),
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
		return Config{}, errors.New(envToken + " not set; run: export " + envToken + "=$(yc iam create-token)")
	}
	if c.OrgID == "" {
		return Config{}, errors.New(envOrgID + " not set; find via: yc organization-manager organization list")
	}
	return c, nil
}

func (c Config) TrackerHeaders() http.Header {
	h := http.Header{}
	h.Set("Authorization", "Bearer "+c.Token)
	h.Set("X-Org-ID", c.OrgID)
	h.Set("Content-Type", "application/json")
	return h
}

func (c Config) WikiHeaders() http.Header {
	h := http.Header{}
	h.Set("Authorization", "Bearer "+c.Token)
	h.Set("X-Cloud-Org-Id", c.OrgID)
	h.Set("Content-Type", "application/json")
	return h
}
