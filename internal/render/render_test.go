package render

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

type fakePage struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

func (f fakePage) Plain() string { return f.Title + "\n\n" + f.Body }
func (f fakePage) Row() string   { return f.Title + "  " + f.Body }

func TestOne_Plain(t *testing.T) {
	var b bytes.Buffer
	err := One(&b, Plain, fakePage{Title: "T", Body: "B"})
	if err != nil {
		t.Fatal(err)
	}
	got := b.String()
	if got != "T\n\nB\n" {
		t.Fatalf("got %q", got)
	}
}

func TestOne_JSON(t *testing.T) {
	var b bytes.Buffer
	err := One(&b, JSON, fakePage{Title: "T", Body: "B"})
	if err != nil {
		t.Fatal(err)
	}
	got := b.String()
	if !strings.Contains(got, `"title": "T"`) || !strings.Contains(got, `"body": "B"`) {
		t.Fatalf("got %q", got)
	}
}

func TestMany_Plain(t *testing.T) {
	var b bytes.Buffer
	items := []fakePage{{Title: "A", Body: "1"}, {Title: "B", Body: "2"}}
	err := Many(&b, Plain, items)
	if err != nil {
		t.Fatal(err)
	}
	got := b.String()
	if got != "A  1\nB  2\n" {
		t.Fatalf("got %q", got)
	}
}

func TestMany_JSON(t *testing.T) {
	var b bytes.Buffer
	items := []fakePage{{Title: "A", Body: "1"}}
	err := Many(&b, JSON, items)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), `"title": "A"`) {
		t.Fatalf("got %q", b.String())
	}
}

func TestConfirm(t *testing.T) {
	var b bytes.Buffer
	if err := Confirm(&b, Plain, "created", "team/notes"); err != nil {
		t.Fatal(err)
	}
	if b.String() != "created: team/notes\n" {
		t.Fatalf("got %q", b.String())
	}
	b.Reset()
	if err := Confirm(&b, JSON, "updated", "team/notes"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), `"updated": "team/notes"`) {
		t.Fatalf("got %q", b.String())
	}
}

func TestError_Plain(t *testing.T) {
	var b bytes.Buffer
	Error(&b, Plain, errors.New("boom"), 0)
	if b.String() != "error: boom\n" {
		t.Fatalf("got %q", b.String())
	}
	b.Reset()
	Error(&b, Plain, errors.New("boom"), 404)
	if b.String() != "error (404): boom\n" {
		t.Fatalf("got %q", b.String())
	}
}

func TestError_JSON(t *testing.T) {
	var b bytes.Buffer
	Error(&b, JSON, errors.New("boom"), 404)
	got := b.String()
	if !strings.Contains(got, `"error":"boom"`) || !strings.Contains(got, `"status":404`) {
		t.Fatalf("got %q", got)
	}
}

func TestSkipEmpty(t *testing.T) {
	tests := []struct {
		in   []string
		want string
	}{
		{[]string{"a", "b", "c"}, "a  b  c"},
		{[]string{"a", "", "c"}, "a  c"},
		{[]string{"", "", ""}, ""},
		{[]string{"only"}, "only"},
	}
	for _, tt := range tests {
		if got := SkipEmpty(tt.in...); got != tt.want {
			t.Errorf("SkipEmpty(%v) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestSkipEmptyLines(t *testing.T) {
	got := SkipEmptyLines("a", "", "b")
	if got != "a\nb" {
		t.Fatalf("got %q", got)
	}
}
