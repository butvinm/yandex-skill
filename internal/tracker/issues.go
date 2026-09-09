package tracker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/butvinm/yandex-skill/internal/render"
)

type Display struct {
	Display string `json:"display"`
}

func (d Display) String() string { return d.Display }

// Issue is the raw JSON object Tracker returns for an issue, kept as a map rather than a fixed struct so that fields beyond the few Plain()/Row() render explicitly (org custom fields and standard fields this codebase doesn't otherwise model, like fixVersions or priority) survive decoding instead of being silently dropped by encoding/json.
type Issue map[string]json.RawMessage

// coreIssueFields are the keys Plain() surfaces explicitly; formatExtraFields skips them so they aren't printed a second time.
var coreIssueFields = map[string]bool{
	"key": true, "summary": true, "status": true,
	"assignee": true, "updatedAt": true, "description": true, "links": true,
}

func (i Issue) str(key string) string {
	var s string
	_ = json.Unmarshal(i[key], &s)
	return s
}

func (i Issue) display(key string) string {
	var d Display
	_ = json.Unmarshal(i[key], &d)
	return d.Display
}

func (i Issue) Plain() string {
	header := i.str("key") + ": " + i.str("summary")
	meta := render.SkipEmpty(i.display("status"), i.display("assignee"), i.str("updatedAt"))
	return render.SkipEmptyLines(header, meta, i.linksBlock(), i.str("description"), formatExtraFields(i))
}

// linksBlock renders the "links" array GetIssue attaches as an indented block under a "links:" heading, one Link.Line per entry; empty when the issue has no links or came from a search (which never carries them).
func (i Issue) linksBlock() string {
	var links []Link
	if json.Unmarshal(i["links"], &links) != nil || len(links) == 0 {
		return ""
	}
	lines := make([]string, 0, len(links)+1)
	lines = append(lines, "links:")
	for _, l := range links {
		lines = append(lines, "  "+l.Line(i.str("key")))
	}
	return strings.Join(lines, "\n")
}

func (i Issue) Row() string {
	return render.SkipEmpty(i.str("key"), i.display("status"), i.display("assignee"), i.str("summary"))
}

// formatExtraFields renders every field beyond coreIssueFields as a "key: value" block, sorted for deterministic output.
func formatExtraFields(i Issue) string {
	keys := make([]string, 0, len(i))
	for k := range i {
		if !coreIssueFields[k] {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	lines := make([]string, len(keys))
	for idx, k := range keys {
		lines[idx] = k + ": " + formatFieldValue(i[k])
	}
	return strings.Join(lines, "\n")
}

// formatFieldValue unquotes a plain JSON string; any other shape (number, bool, object, array, e.g. Tracker's {display,id} user/select fields) is passed through as compact JSON.
func formatFieldValue(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	return string(raw)
}

// GetIssue fetches an issue and attaches its links under the "links" key, since the plain issue endpoint never includes them and a reader that misses a dependency has no way to notice.
func (c *Client) GetIssue(ctx context.Context, key string) (*Issue, error) {
	var out Issue
	if _, err := c.Do(ctx, http.MethodGet, "/v3/issues/"+key, nil, &out); err != nil {
		return nil, err
	}
	links, err := c.ListLinks(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("links: %w", err)
	}
	raw, err := json.Marshal(links)
	if err != nil {
		return nil, err
	}
	out["links"] = raw
	return &out, nil
}

func (c *Client) ListIssues(ctx context.Context, queue, query string) ([]Issue, error) {
	if queue == "" && query == "" {
		return nil, errors.New("specify --queue or --query")
	}
	body := map[string]string{}
	if queue != "" {
		body["queue"] = queue
	} else {
		body["query"] = query
	}

	var all []Issue
	err := c.DoPaginated(ctx, "/v3/issues/_search", body, func(raw []byte) error {
		var batch []Issue
		if err := json.Unmarshal(raw, &batch); err != nil {
			return fmt.Errorf("decode issues page: %w", err)
		}
		all = append(all, batch...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return all, nil
}
