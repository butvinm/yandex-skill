package tracker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/butvinm/yandex-cli/internal/render"
)

type Display struct {
	Display string `json:"display"`
}

func (d Display) String() string { return d.Display }

type Issue struct {
	Key         string  `json:"key"`
	Summary     string  `json:"summary"`
	Status      Display `json:"status"`
	Assignee    Display `json:"assignee"`
	UpdatedAt   string  `json:"updatedAt"`
	Description string  `json:"description"`
}

func (i Issue) Plain() string {
	header := i.Key + ": " + i.Summary
	meta := render.SkipEmpty(i.Status.Display, i.Assignee.Display, i.UpdatedAt)
	return render.SkipEmptyLines(header, meta, i.Description)
}

func (i Issue) Row() string {
	return render.SkipEmpty(i.Key, i.Status.Display, i.Assignee.Display, i.Summary)
}

func (c *Client) GetIssue(ctx context.Context, key string) (*Issue, error) {
	var out Issue
	_, err := c.Do(ctx, http.MethodGet, "/v3/issues/"+key, nil, &out)
	if err != nil {
		return nil, err
	}
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
