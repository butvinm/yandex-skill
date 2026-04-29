package render

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

type Format string

const (
	Plain Format = "plain"
	JSON  Format = "json"
)

type Plainer interface {
	Plain() string
}

type Rower interface {
	Row() string
}

func One[T Plainer](w io.Writer, format Format, v T) error {
	if format == JSON {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(v)
	}
	_, err := fmt.Fprintln(w, v.Plain())
	return err
}

func Many[T Rower](w io.Writer, format Format, items []T) error {
	if format == JSON {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(items)
	}
	for _, it := range items {
		if _, err := fmt.Fprintln(w, it.Row()); err != nil {
			return err
		}
	}
	return nil
}

func Confirm(w io.Writer, format Format, action, slug string) error {
	if format == JSON {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(map[string]string{action: slug})
	}
	_, err := fmt.Fprintf(w, "%s: %s\n", action, slug)
	return err
}

func Error(w io.Writer, format Format, err error, status int) {
	if format == JSON {
		_ = json.NewEncoder(w).Encode(struct {
			Error  string `json:"error"`
			Status int    `json:"status,omitempty"`
		}{err.Error(), status})
		return
	}
	if status != 0 {
		fmt.Fprintf(w, "error (%d): %s\n", status, err.Error())
	} else {
		fmt.Fprintf(w, "error: %s\n", err.Error())
	}
}

// SkipEmpty joins non-empty strings with two-space separator.
func SkipEmpty(parts ...string) string {
	var keep []string
	for _, p := range parts {
		if p != "" {
			keep = append(keep, p)
		}
	}
	return strings.Join(keep, "  ")
}

// SkipEmptyLines joins non-empty strings with newlines.
func SkipEmptyLines(parts ...string) string {
	var keep []string
	for _, p := range parts {
		if p != "" {
			keep = append(keep, p)
		}
	}
	return strings.Join(keep, "\n")
}
