package auth

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

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
