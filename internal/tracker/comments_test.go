package tracker

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/butvinm/yandex-skill/internal/auth"
)

func TestComment_Plain_WithAttachments(t *testing.T) {
	c := Comment{
		Text:      "See logs.",
		CreatedBy: Display{Display: "Иван Иванов"},
		CreatedAt: "2026-05-04T10:11:12.000+0000",
		Attachments: []CommentAttachmentRef{
			{ID: "67890", Display: "screenshot.png"},
			{ID: "67891", Display: "log.txt"},
		},
	}
	got := c.Plain()
	want := "Иван Иванов  2026-05-04T10:11:12.000+0000\nSee logs.\nattachments: 67890:screenshot.png, 67891:log.txt"
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestComment_Plain_NoAttachments(t *testing.T) {
	c := Comment{
		Text:      "Merging.",
		CreatedBy: Display{Display: "Петр"},
		CreatedAt: "2026-05-04T11:00:00.000+0000",
	}
	got := c.Plain()
	want := "Петр  2026-05-04T11:00:00.000+0000\nMerging."
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
	if strings.Contains(got, "attachments:") {
		t.Errorf("plain unexpectedly contains attachments line: %q", got)
	}
}

func TestComment_Row_WithAttachments(t *testing.T) {
	c := Comment{
		Text:        "first line\nsecond line",
		CreatedBy:   Display{Display: "ivan"},
		CreatedAt:   "T",
		Attachments: []CommentAttachmentRef{{ID: "1", Display: "a"}},
	}
	got := c.Row()
	want := "ivan  T  first line  [1 attached]"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestComment_Row_NoAttachments(t *testing.T) {
	c := Comment{Text: "ok", CreatedBy: Display{Display: "ivan"}, CreatedAt: "T"}
	got := c.Row()
	want := "ivan  T  ok"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestListComments_RequestsExpandAttachments(t *testing.T) {
	var seenQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3/issues/FOO-1/comments" {
			t.Errorf("path = %s", r.URL.Path)
		}
		seenQuery = r.URL.RawQuery
		_, _ = io.WriteString(w, `[]`)
	}))
	defer srv.Close()
	c := New(auth.Config{Token: "t", OrgID: "o", TrackerBaseURL: srv.URL})

	if _, err := c.ListComments(context.Background(), "FOO-1"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(seenQuery, "expand=attachments") {
		t.Errorf("query = %q, want it to contain expand=attachments", seenQuery)
	}
}

func TestListComments_DecodesAttachments(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `[
			{
				"id": 1,
				"longId": "abc",
				"text": "see file",
				"createdBy": {"display": "ivan"},
				"createdAt": "2026-05-04T10:11:12.000+0000",
				"updatedAt": "2026-05-04T10:11:12.000+0000",
				"attachments": [
					{"self": "https://api/.../67890", "id": "67890", "display": "x.png"}
				]
			},
			{
				"id": 2,
				"text": "no files",
				"createdBy": {"display": "petr"},
				"createdAt": "2026-05-04T11:00:00.000+0000",
				"updatedAt": "2026-05-04T11:00:00.000+0000"
			}
		]`)
	}))
	defer srv.Close()
	c := New(auth.Config{Token: "t", OrgID: "o", TrackerBaseURL: srv.URL})

	got, err := c.ListComments(context.Background(), "FOO-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].ID != 1 || got[0].LongID != "abc" || got[0].CreatedBy.Display != "ivan" {
		t.Errorf("comment[0] = %+v", got[0])
	}
	if len(got[0].Attachments) != 1 || got[0].Attachments[0].ID != "67890" || got[0].Attachments[0].Display != "x.png" {
		t.Errorf("comment[0].Attachments = %+v", got[0].Attachments)
	}
	if len(got[1].Attachments) != 0 {
		t.Errorf("comment[1] should have no attachments, got %+v", got[1].Attachments)
	}
}

func TestListComments_FollowsLink(t *testing.T) {
	pages := 0
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()
	mux.HandleFunc("/v3/issues/FOO-1/comments", func(w http.ResponseWriter, r *http.Request) {
		pages++
		if pages == 1 {
			w.Header().Set("Link", `<`+srv.URL+`/v3/issues/FOO-1/comments?page=2>; rel="next"`)
			_, _ = io.WriteString(w, `[{"id":1,"text":"a","createdBy":{"display":"ivan"}}]`)
			return
		}
		_, _ = io.WriteString(w, `[{"id":2,"text":"b","createdBy":{"display":"petr"}}]`)
	})
	c := New(auth.Config{Token: "t", OrgID: "o", TrackerBaseURL: srv.URL})

	got, err := c.ListComments(context.Background(), "FOO-1")
	if err != nil {
		t.Fatal(err)
	}
	if pages != 2 {
		t.Errorf("pages = %d, want 2", pages)
	}
	if len(got) != 2 || got[0].ID != 1 || got[1].ID != 2 {
		t.Errorf("got = %+v", got)
	}
}

func TestListComments_404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		_, _ = io.WriteString(w, `{"errorMessages":["Issue not found"]}`)
	}))
	defer srv.Close()
	c := New(auth.Config{Token: "t", OrgID: "o", TrackerBaseURL: srv.URL})

	_, err := c.ListComments(context.Background(), "FOO-99")
	if err == nil {
		t.Fatal("want error")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 404 {
		t.Errorf("err = %v", err)
	}
}
