package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type cachedToken struct {
	Token      string    `json:"token"`
	AcquiredAt time.Time `json:"acquired_at"`
}

func cacheFilePath() (string, error) {
	dir, err := os.UserCacheDir()
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
