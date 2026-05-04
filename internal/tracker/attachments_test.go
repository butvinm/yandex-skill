package tracker

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/butvinm/yandex-skill/internal/auth"
)

func TestHumanSize(t *testing.T) {
	tests := []struct {
		in   int64
		want string
	}{
		{0, ""},
		{-1, ""},
		{1, "1 B"},
		{1023, "1023 B"},
		{1024, "1.0 KiB"},
		{14823, "14.5 KiB"},
		{1024 * 1024, "1.0 MiB"},
		{int64(1.5 * 1024 * 1024), "1.5 MiB"},
		{1024 * 1024 * 1024, "1.0 GiB"},
	}
	for _, tt := range tests {
		if got := humanSize(tt.in); got != tt.want {
			t.Errorf("humanSize(%d) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestAttachment_Plain(t *testing.T) {
	a := Attachment{
		ID:        "67890",
		Name:      "screenshot.png",
		Size:      14823,
		Mimetype:  "image/png",
		Content:   "https://api.tracker.yandex.net/v3/issues/PROJ-1/attachments/67890/screenshot.png",
		CreatedAt: "2026-05-04T10:11:12.000+0000",
	}
	got := a.Plain()
	want := "67890  screenshot.png  image/png  14.5 KiB  2026-05-04T10:11:12.000+0000  https://api.tracker.yandex.net/v3/issues/PROJ-1/attachments/67890/screenshot.png"
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
	if a.Row() != got {
		t.Errorf("Row != Plain: %q vs %q", a.Row(), got)
	}
}

func TestAttachment_Plain_SkipsEmpty(t *testing.T) {
	a := Attachment{ID: "1", Name: "x", Content: "u"}
	got := a.Plain()
	want := "1  x  u"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestListAttachments_DecodesAll(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3/issues/FOO-1/attachments" {
			t.Errorf("path = %s", r.URL.Path)
		}
		_, _ = io.WriteString(w, `[
			{"id":"1","name":"a.png","size":100,"mimetype":"image/png","content":"https://x/a","createdBy":{"display":"ivan"},"createdAt":"T1"},
			{"id":"2","name":"b.txt","size":2048,"mimetype":"text/plain","content":"https://x/b","createdBy":{"display":"petr"},"createdAt":"T2"}
		]`)
	}))
	defer srv.Close()
	c := New(auth.Config{Token: "t", OrgID: "o", TrackerBaseURL: srv.URL})

	got, err := c.ListAttachments(context.Background(), "FOO-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].ID != "1" || got[0].Name != "a.png" || got[0].Size != 100 || got[0].CreatedBy.Display != "ivan" {
		t.Errorf("attachment[0] = %+v", got[0])
	}
	if got[1].ID != "2" || got[1].Mimetype != "text/plain" || got[1].Size != 2048 {
		t.Errorf("attachment[1] = %+v", got[1])
	}
}

func TestListAttachments_FollowsLink(t *testing.T) {
	pages := 0
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()
	mux.HandleFunc("/v3/issues/FOO-1/attachments", func(w http.ResponseWriter, r *http.Request) {
		pages++
		if pages == 1 {
			w.Header().Set("Link", `<`+srv.URL+`/v3/issues/FOO-1/attachments?page=2>; rel="next"`)
			_, _ = io.WriteString(w, `[{"id":"1","name":"a"}]`)
			return
		}
		_, _ = io.WriteString(w, `[{"id":"2","name":"b"}]`)
	})
	c := New(auth.Config{Token: "t", OrgID: "o", TrackerBaseURL: srv.URL})

	got, err := c.ListAttachments(context.Background(), "FOO-1")
	if err != nil {
		t.Fatal(err)
	}
	if pages != 2 {
		t.Errorf("pages = %d, want 2", pages)
	}
	if len(got) != 2 || got[0].ID != "1" || got[1].ID != "2" {
		t.Errorf("got = %+v", got)
	}
}

func TestListAttachments_404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		_, _ = io.WriteString(w, `{"errorMessages":["Issue not found"]}`)
	}))
	defer srv.Close()
	c := New(auth.Config{Token: "t", OrgID: "o", TrackerBaseURL: srv.URL})

	_, err := c.ListAttachments(context.Background(), "FOO-99")
	if err == nil {
		t.Fatal("want error")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 404 {
		t.Errorf("err = %v", err)
	}
}
