package tracker

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/butvinm/yandex-skill/internal/render"
)

type LinkType struct {
	ID      string `json:"id"`
	Inward  string `json:"inward"`
	Outward string `json:"outward"`
}

type LinkObject struct {
	Key     string `json:"key"`
	Display string `json:"display"`
}

// Link is the subset of a GET /v3/issues/{key}/links entry that plain output renders; the JSON output carries the untouched API objects instead.
type Link struct {
	Type      LinkType   `json:"type"`
	Direction string     `json:"direction"`
	Object    LinkObject `json:"object"`
	Status    Display    `json:"status"`
	Assignee  Display    `json:"assignee"`
}

// Label names the linked issue's role relative to the current one. The inward/outward pair is fixed per link type and names the two ends of the relation, so which name applies to the linked issue depends on direction. Verified against the parent field and the Tracker UI: an issue's parent arrives as direction=inward with outward="Родительская задача", and its subtask as direction=outward with inward="Подзадача".
func (l Link) Label() string {
	if l.Direction == "inward" {
		return l.Type.Outward
	}
	return l.Type.Inward
}

// Line renders the link as the sentence "<linked-key> <label> <current-key>" followed by the linked issue's status, assignee and title. The explicit current key keeps asymmetric relations unambiguous: "ADCAI-89 Зависит от FOO-1" says ADCAI-89 depends on FOO-1, where a bare "Зависит от: ADCAI-89" would read the other way round.
func (l Link) Line(current string) string {
	return render.SkipEmpty(l.Object.Key+" "+l.Label()+" "+current, l.Status.Display, l.Assignee.Display, l.Object.Display)
}

// ListLinks fetches every link of an issue as raw API objects. It uses the dedicated links endpoint rather than GET /v3/issues/{key}?expand=links because the expanded form omits each linked issue's status and assignee.
func (c *Client) ListLinks(ctx context.Context, issueKey string) ([]json.RawMessage, error) {
	all := []json.RawMessage{}
	err := c.DoPaginated(ctx, "/v3/issues/"+issueKey+"/links", nil, func(raw []byte) error {
		var batch []json.RawMessage
		if err := json.Unmarshal(raw, &batch); err != nil {
			return fmt.Errorf("decode links page: %w", err)
		}
		all = append(all, batch...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return all, nil
}
