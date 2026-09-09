package tracker

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/butvinm/yandex-skill/internal/auth"
)

// subtaskType mirrors Tracker's built-in "subtask" link type: the same inward/outward pair arrives on both ends of a parent/child pair, only direction differs.
var subtaskType = LinkType{ID: "subtask", Inward: "Подзадача", Outward: "Родительская задача"}

func TestLink_Label(t *testing.T) {
	tests := []struct {
		direction, want string
	}{
		{"inward", "Родительская задача"},
		{"outward", "Подзадача"},
	}
	for _, tt := range tests {
		got := Link{Type: subtaskType, Direction: tt.direction}.Label()
		if got != tt.want {
			t.Errorf("direction %s: got %q want %q", tt.direction, got, tt.want)
		}
	}
}

func TestLink_Line(t *testing.T) {
	l := Link{
		Type:      LinkType{ID: "depends", Inward: "Блокирующая задача", Outward: "Зависит от"},
		Direction: "inward",
		Object:    LinkObject{Key: "BAR-2", Display: "consumer"},
		Status:    Display{"Open"},
		Assignee:  Display{"ivan"},
	}
	got := l.Line("FOO-1")
	want := "BAR-2 Зависит от FOO-1  Open  ivan  consumer"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestLink_Line_SkipsEmpty(t *testing.T) {
	l := Link{Type: subtaskType, Direction: "outward", Object: LinkObject{Key: "BAR-2", Display: "child"}}
	got := l.Line("FOO-1")
	want := "BAR-2 Подзадача FOO-1  child"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestListLinks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3/issues/FOO-1/links" {
			t.Errorf("path = %s", r.URL.Path)
		}
		_, _ = io.WriteString(w, `[{"id":1,"type":{"id":"subtask","inward":"Подзадача","outward":"Родительская задача"},"direction":"inward","object":{"key":"BAR-2","display":"parent"}},{"id":2,"direction":"outward","object":{"key":"BAR-3"}}]`)
	}))
	defer srv.Close()
	c := New(auth.Config{Token: "t", OrgID: "o", TrackerBaseURL: srv.URL})

	got, err := c.ListLinks(context.Background(), "FOO-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d links, want 2", len(got))
	}
}

func TestListLinks_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `[]`)
	}))
	defer srv.Close()
	c := New(auth.Config{Token: "t", OrgID: "o", TrackerBaseURL: srv.URL})

	got, err := c.ListLinks(context.Background(), "FOO-1")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || len(got) != 0 {
		t.Errorf("got %v, want empty non-nil slice", got)
	}
}
