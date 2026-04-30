package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

const (
	envRefreshPeriod    = "YANDEX_IAM_TOKEN_REFRESH_PERIOD"
	defaultRefreshHours = 10
	maxRefreshHours     = 12
)

// parseRefreshPeriod converts an env-var string into a refresh duration.
// Empty, malformed, or non-positive values fall back to the default. Values
// above maxRefreshHours are clamped to maxRefreshHours (Yandex IAM tokens
// expire at 12h; clamping protects against typos that would otherwise let
// the cache outlive the token). Errors are intentionally swallowed: a typo
// in this knob should not break every CLI invocation.
func parseRefreshPeriod(envValue string) time.Duration {
	if envValue == "" {
		return defaultRefreshHours * time.Hour
	}
	hours, err := strconv.Atoi(envValue)
	if err != nil || hours <= 0 {
		return defaultRefreshHours * time.Hour
	}
	if hours > maxRefreshHours {
		hours = maxRefreshHours
	}
	return time.Duration(hours) * time.Hour
}

type cachedToken struct {
	Token      string    `json:"token"`
	AcquiredAt time.Time `json:"acquired_at"`
}

var cacheDirFn = os.UserCacheDir

func cacheFilePath() (string, error) {
	dir, err := cacheDirFn()
	if err != nil {
		return "", fmt.Errorf("resolving user cache dir: %w", err)
	}
	return filepath.Join(dir, "yandex-cli", "iam-token.json"), nil
}

func readCachedToken(path string) (cachedToken, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return cachedToken{}, err
	}
	var ct cachedToken
	if err := json.Unmarshal(data, &ct); err != nil {
		return cachedToken{}, fmt.Errorf("parsing cached token: %w", err)
	}
	if ct.Token == "" {
		return cachedToken{}, errors.New("cached token is empty")
	}
	return ct, nil
}

func writeCachedToken(path string, ct cachedToken) error {
	data, err := json.Marshal(ct)
	if err != nil {
		return fmt.Errorf("encoding cached token: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("creating cache dir: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("writing cache tmp file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("renaming cache tmp file: %w", err)
	}
	return nil
}

func (ct cachedToken) isFresh(refreshPeriod time.Duration, now time.Time) bool {
	return now.Sub(ct.AcquiredAt) < refreshPeriod
}
