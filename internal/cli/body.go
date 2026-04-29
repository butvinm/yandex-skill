package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
)

type BodyInput struct {
	Body     string `name:"body" help:"page body inline" xor:"body"`
	BodyFile string `name:"body-file" help:"page body from file ('-' for stdin)" xor:"body"`
}

func (b BodyInput) Read(stdin io.Reader) (string, error) {
	if b.Body != "" {
		return b.Body, nil
	}
	if b.BodyFile == "" {
		return "", errors.New("--body or --body-file required")
	}
	if b.BodyFile == "-" {
		buf, err := io.ReadAll(stdin)
		if err != nil {
			return "", fmt.Errorf("read stdin: %w", err)
		}
		return string(buf), nil
	}
	buf, err := os.ReadFile(b.BodyFile)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", b.BodyFile, err)
	}
	return string(buf), nil
}
