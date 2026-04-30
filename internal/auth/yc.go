package auth

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

var nowFn = time.Now

type ycExecutor interface {
	runYC(ctx context.Context) ([]byte, error)
}

type realYCExecutor struct{}

func (realYCExecutor) runYC(ctx context.Context) ([]byte, error) {
	return exec.CommandContext(ctx, "yc", "iam", "create-token").Output()
}

var ycExec ycExecutor = realYCExecutor{}

func fetchYCToken(ctx context.Context) (string, error) {
	out, err := ycExec.runYC(ctx)
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			if stderr := strings.TrimSpace(string(ee.Stderr)); stderr != "" {
				return "", fmt.Errorf("yc iam create-token: %s", stderr)
			}
		}
		return "", fmt.Errorf("running yc iam create-token: %w", err)
	}
	tok := strings.TrimSpace(string(out))
	if tok == "" {
		return "", errors.New("yc iam create-token returned empty output")
	}
	return tok, nil
}

// loadYCToken returns an IAM token, preferring a fresh on-disk cache and
// falling back to `yc iam create-token`. Cache I/O failures are logged but
// non-fatal: we always return a usable token if `yc` succeeds.
func loadYCToken(ctx context.Context) (string, error) {
	cachePath, pathErr := cacheFilePath()
	if pathErr == nil {
		if ct, err := readCachedToken(cachePath); err == nil {
			period := parseRefreshPeriod(os.Getenv(envRefreshPeriod))
			if ct.isFresh(period, nowFn()) {
				return ct.Token, nil
			}
		}
	}

	tok, err := fetchYCToken(ctx)
	if err != nil {
		return "", err
	}

	if pathErr != nil {
		fmt.Fprintf(os.Stderr, "yandex-cli: skipping IAM token cache: %v\n", pathErr)
		return tok, nil
	}
	if writeErr := writeCachedToken(cachePath, cachedToken{Token: tok, AcquiredAt: nowFn()}); writeErr != nil {
		fmt.Fprintf(os.Stderr, "yandex-cli: failed to write IAM token cache: %v\n", writeErr)
	}
	return tok, nil
}
