package auth

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func swapNowFn(t *testing.T, now time.Time) {
	t.Helper()
	prev := nowFn
	nowFn = func() time.Time { return now }
	t.Cleanup(func() { nowFn = prev })
}

type fakeYCExecutor struct {
	out    []byte
	err    error
	called bool
}

func (f *fakeYCExecutor) runYC(_ context.Context) ([]byte, error) {
	f.called = true
	return f.out, f.err
}

func swapYCExec(t *testing.T, fake ycExecutor) {
	t.Helper()
	prev := ycExec
	ycExec = fake
	t.Cleanup(func() { ycExec = prev })
}

func TestFetchYCToken_Success(t *testing.T) {
	swapYCExec(t, &fakeYCExecutor{out: []byte("t1.abc\n")})
	got, err := fetchYCToken(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "t1.abc" {
		t.Errorf("token = %q, want %q (whitespace must be trimmed)", got, "t1.abc")
	}
}

func TestFetchYCToken_EmptyOutput(t *testing.T) {
	swapYCExec(t, &fakeYCExecutor{out: []byte("\n  \n")})
	_, err := fetchYCToken(context.Background())
	if err == nil {
		t.Fatal("expected error for empty output, got nil")
	}
	if !strings.Contains(err.Error(), "empty output") {
		t.Errorf("error = %q, want to mention empty output", err.Error())
	}
}

func TestFetchYCToken_ExitErrorWithStderr(t *testing.T) {
	ee := &exec.ExitError{Stderr: []byte("ERROR: not authenticated\n")}
	swapYCExec(t, &fakeYCExecutor{err: ee})
	_, err := fetchYCToken(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "not authenticated") {
		t.Errorf("error = %q, want to include yc stderr", err.Error())
	}
}

func TestFetchYCToken_UnknownError(t *testing.T) {
	swapYCExec(t, &fakeYCExecutor{err: errors.New("exec: \"yc\": executable file not found in $PATH")})
	_, err := fetchYCToken(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "running yc iam create-token") {
		t.Errorf("error = %q, want wrapping prefix", err.Error())
	}
	if !strings.Contains(err.Error(), "executable file not found") {
		t.Errorf("error = %q, want to wrap underlying error", err.Error())
	}
}
