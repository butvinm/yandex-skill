package tracker

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/butvinm/yandex-skill/internal/auth"
)

func TestQueue_Plain(t *testing.T) {
	q := Queue{Key: "FOO", Name: "Backlog", Lead: Display{Display: "ivan"}, DefaultPriority: Display{Display: "Normal"}}
	got := q.Plain()
	want := "FOO: Backlog\nivan  Normal"
	if got != want {
		t.Errorf("got %q\nwant %q", got, want)
	}
}

func TestQueue_Row(t *testing.T) {
	q := Queue{Key: "FOO", Name: "Backlog"}
	if got := q.Row(); got != "FOO  Backlog" {
		t.Errorf("got %q", got)
	}
}

func TestGetQueue(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3/queues/FOO" {
			t.Errorf("path = %s", r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"key":"FOO","name":"Backlog","lead":{"display":"ivan"},"defaultPriority":{"display":"Normal"}}`)
	}))
	defer srv.Close()
	c := New(auth.Config{Token: "t", OrgID: "o", TrackerBaseURL: srv.URL})

	got, err := c.GetQueue(context.Background(), "FOO")
	if err != nil {
		t.Fatal(err)
	}
	if got.Key != "FOO" || got.Lead.Display != "ivan" {
		t.Errorf("got = %+v", got)
	}
}

func TestListQueues(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3/queues/" {
			t.Errorf("path = %s", r.URL.Path)
		}
		_, _ = io.WriteString(w, `[{"key":"A","name":"Alpha"},{"key":"B","name":"Beta"}]`)
	}))
	defer srv.Close()
	c := New(auth.Config{Token: "t", OrgID: "o", TrackerBaseURL: srv.URL})

	got, err := c.ListQueues(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Key != "A" {
		t.Errorf("got = %+v", got)
	}
}
