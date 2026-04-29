package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBodyInput_Inline(t *testing.T) {
	b := BodyInput{Body: "hello"}
	got, err := b.Read(nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != "hello" {
		t.Errorf("got %q", got)
	}
}

func TestBodyInput_File(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "draft.md")
	if err := os.WriteFile(p, []byte("# Title\n\nBody"), 0o600); err != nil {
		t.Fatal(err)
	}
	b := BodyInput{BodyFile: p}
	got, err := b.Read(nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != "# Title\n\nBody" {
		t.Errorf("got %q", got)
	}
}

func TestBodyInput_Stdin(t *testing.T) {
	b := BodyInput{BodyFile: "-"}
	got, err := b.Read(strings.NewReader("from stdin"))
	if err != nil {
		t.Fatal(err)
	}
	if got != "from stdin" {
		t.Errorf("got %q", got)
	}
}

func TestBodyInput_Empty(t *testing.T) {
	b := BodyInput{}
	_, err := b.Read(nil)
	if err == nil || !strings.Contains(err.Error(), "--body or --body-file required") {
		t.Fatalf("err = %v", err)
	}
}

func TestBodyInput_FileMissing(t *testing.T) {
	b := BodyInput{BodyFile: "/nonexistent/path/to/file"}
	_, err := b.Read(nil)
	if err == nil {
		t.Fatal("want error")
	}
}
