package cli

import (
	"context"
	"io"
	"os"
	"path/filepath"

	"github.com/alecthomas/kong"

	"github.com/butvinm/yandex-skill/internal/auth"
	"github.com/butvinm/yandex-skill/internal/render"
	"github.com/butvinm/yandex-skill/internal/tracker"
	"github.com/butvinm/yandex-skill/internal/wiki"
)

// Globals carries cross-command state injected via kong.Bind.
type Globals struct {
	JSON   bool
	Stdout io.Writer
	Stderr io.Writer
	Stdin  io.Reader
	Ctx    context.Context
}

func (g *Globals) Format() render.Format {
	if g.JSON {
		return render.JSON
	}
	return render.Plain
}

// CLI is the root kong struct.
type CLI struct {
	JSON bool `name:"json" help:"emit JSON instead of plain text"`

	Tracker TrackerCmd `cmd:"" help:"Yandex Tracker (read)"`
	Wiki    WikiCmd    `cmd:"" help:"Yandex Wiki (read+write)"`

	Version VersionCmd `cmd:"" help:"print version"`
}

type TrackerCmd struct {
	Issues      TrackerIssuesCmd      `cmd:"" help:"issues"`
	Queues      TrackerQueuesCmd      `cmd:"" help:"queues"`
	Comments    TrackerCommentsCmd    `cmd:"" help:"issue comments"`
	Attachments TrackerAttachmentsCmd `cmd:"" help:"issue attachments (issue-level and comment-level, unified)"`
}

type TrackerCommentsCmd struct {
	List ListTrackerCommentsCmd `cmd:"" help:"list comments on an issue"`
}

type TrackerAttachmentsCmd struct {
	List     ListTrackerAttachmentsCmd    `cmd:"" help:"list attachments on an issue"`
	Download DownloadTrackerAttachmentCmd `cmd:"" help:"download an attachment by issue key + id"`
}

type TrackerIssuesCmd struct {
	List ListIssuesCmd `cmd:"" help:"list issues by queue or query"`
	Get  GetIssueCmd   `cmd:"" help:"get an issue by key"`
}

type TrackerQueuesCmd struct {
	List ListQueuesCmd `cmd:"" help:"list queues"`
	Get  GetQueueCmd   `cmd:"" help:"get a queue by key"`
}

type WikiCmd struct {
	Pages       WikiPagesCmd       `cmd:"" help:"pages"`
	Attachments WikiAttachmentsCmd `cmd:"" help:"page attachments"`
}

type WikiPagesCmd struct {
	List   ListPagesCmd  `cmd:"" help:"list page descendants by parent slug"`
	Get    GetPageCmd    `cmd:"" help:"get a page by slug"`
	Create CreatePageCmd `cmd:"" help:"create a page"`
	Update UpdatePageCmd `cmd:"" help:"update a page body"`
}

type WikiAttachmentsCmd struct {
	List     ListAttachmentsCmd    `cmd:"" help:"list attachments on a page"`
	Upload   UploadAttachmentCmd   `cmd:"" help:"upload a file to a page"`
	Download DownloadAttachmentCmd `cmd:"" help:"download an attachment by page slug + filename"`
	Delete   DeleteAttachmentCmd   `cmd:"" help:"delete an attachment by page slug + filename"`
}

type VersionCmd struct{}

func (VersionCmd) Run(g *Globals, version string) error {
	_, err := io.WriteString(g.Stdout, version+"\n")
	return err
}

// --- Tracker commands ---

type GetIssueCmd struct {
	Key string `arg:"" help:"issue key (e.g. FOO-1)"`
}

func (c *GetIssueCmd) Run(g *Globals) error {
	cfg, err := auth.Load()
	if err != nil {
		return err
	}
	issue, err := tracker.New(cfg).GetIssue(g.Ctx, c.Key)
	if err != nil {
		return err
	}
	return render.One(g.Stdout, g.Format(), *issue)
}

type ListIssuesCmd struct {
	Queue string `name:"queue" help:"queue key (e.g. FOO)"`
	Query string `name:"query" help:"Tracker query language string"`
}

func (c *ListIssuesCmd) Run(g *Globals) error {
	cfg, err := auth.Load()
	if err != nil {
		return err
	}
	issues, err := tracker.New(cfg).ListIssues(g.Ctx, c.Queue, c.Query)
	if err != nil {
		return err
	}
	return render.Many(g.Stdout, g.Format(), issues)
}

type GetQueueCmd struct {
	Key string `arg:"" help:"queue key"`
}

func (c *GetQueueCmd) Run(g *Globals) error {
	cfg, err := auth.Load()
	if err != nil {
		return err
	}
	q, err := tracker.New(cfg).GetQueue(g.Ctx, c.Key)
	if err != nil {
		return err
	}
	return render.One(g.Stdout, g.Format(), *q)
}

type ListQueuesCmd struct{}

func (c *ListQueuesCmd) Run(g *Globals) error {
	cfg, err := auth.Load()
	if err != nil {
		return err
	}
	queues, err := tracker.New(cfg).ListQueues(g.Ctx)
	if err != nil {
		return err
	}
	return render.Many(g.Stdout, g.Format(), queues)
}

// --- Tracker comments + attachments commands ---

type ListTrackerCommentsCmd struct {
	Key string `arg:"" help:"issue key (e.g. FOO-1)"`
}

func (c *ListTrackerCommentsCmd) Run(g *Globals) error {
	cfg, err := auth.Load()
	if err != nil {
		return err
	}
	comments, err := tracker.New(cfg).ListComments(g.Ctx, c.Key)
	if err != nil {
		return err
	}
	return render.Many(g.Stdout, g.Format(), comments)
}

type ListTrackerAttachmentsCmd struct {
	Key string `arg:"" help:"issue key (e.g. FOO-1)"`
}

func (c *ListTrackerAttachmentsCmd) Run(g *Globals) error {
	cfg, err := auth.Load()
	if err != nil {
		return err
	}
	atts, err := tracker.New(cfg).ListAttachments(g.Ctx, c.Key)
	if err != nil {
		return err
	}
	return render.Many(g.Stdout, g.Format(), atts)
}

type DownloadTrackerAttachmentCmd struct {
	Key    string `arg:"" help:"issue key (e.g. FOO-1)"`
	ID     string `arg:"" help:"attachment id (from 'tracker attachments list')"`
	Output string `name:"output" default:"-" help:"output path; '-' for stdout"`
}

func (c *DownloadTrackerAttachmentCmd) Run(g *Globals) error {
	cfg, err := auth.Load()
	if err != nil {
		return err
	}
	var w io.Writer
	if c.Output == "-" {
		w = g.Stdout
	} else {
		f, err := os.Create(c.Output)
		if err != nil {
			return err
		}
		defer f.Close()
		w = f
	}
	return tracker.New(cfg).DownloadAttachment(g.Ctx, c.Key, c.ID, w)
}

// --- Wiki commands ---

type GetPageCmd struct {
	Slug           string `arg:"" help:"page slug (e.g. team/notes/2026-04-29)"`
	Output         string `name:"output" help:"write raw page content to file ('-' for stdout); default: stdout via Plain rendering"`
	AttachmentsDir string `name:"attachments-dir" help:"sync attachments to local directory and rewrite content URLs to local relative paths (YFM markdown only; refuses grid pages, warns on legacy)"`
}

func (c *GetPageCmd) Run(g *Globals) error {
	cfg, err := auth.Load()
	if err != nil {
		return err
	}
	client := wiki.New(cfg)
	p, err := client.GetPage(g.Ctx, c.Slug)
	if err != nil {
		return err
	}
	if c.AttachmentsDir != "" {
		rewritten, err := syncAttachmentsForGet(g.Ctx, client, p, c.AttachmentsDir, g.Stderr)
		if err != nil {
			return err
		}
		// With --attachments-dir, content is the round-trip artifact; the
		// title-prefixed Plain() rendering would corrupt that. Default to
		// raw stdout when --output isn't given.
		out := c.Output
		if out == "" {
			out = "-"
		}
		return writeRawContent(g.Stdout, out, rewritten)
	}
	if c.Output != "" {
		return writeRawContent(g.Stdout, c.Output, p.Content)
	}
	return render.One(g.Stdout, g.Format(), *p)
}

// writeRawContent writes content verbatim either to stdout (when output is
// "-") or to the named file. Used by --output to bypass the title-prefixed
// Plain() rendering and produce a clean markdown round-trip artifact.
func writeRawContent(stdout io.Writer, output, content string) error {
	if output == "-" {
		_, err := io.WriteString(stdout, content)
		return err
	}
	return os.WriteFile(output, []byte(content), 0o644)
}

type ListPagesCmd struct {
	Parent string `name:"parent" required:"" help:"parent slug to list children of"`
}

func (c *ListPagesCmd) Run(g *Globals) error {
	cfg, err := auth.Load()
	if err != nil {
		return err
	}
	pages, err := wiki.New(cfg).ListPages(g.Ctx, c.Parent)
	if err != nil {
		return err
	}
	return render.Many(g.Stdout, g.Format(), pages)
}

type CreatePageCmd struct {
	Slug           string `name:"slug" required:"" help:"target slug"`
	Title          string `name:"title" required:"" help:"page title"`
	AttachmentsDir string `name:"attachments-dir" help:"upload referenced local files and rewrite content URLs to server form (always wysiwyg for new pages)"`
	BodyInput
}

func (c *CreatePageCmd) Run(g *Globals) error {
	cfg, err := auth.Load()
	if err != nil {
		return err
	}
	client := wiki.New(cfg)
	body, err := c.Read(g.Stdin)
	if err != nil {
		return err
	}
	if c.AttachmentsDir == "" {
		p, err := client.CreatePage(g.Ctx, c.Slug, c.Title, body)
		if err != nil {
			return err
		}
		return render.Confirm(g.Stdout, g.Format(), "created", p.Slug)
	}
	// Two-phase: create empty page first so we have a page id to bind
	// attachments to, then upload + rewrite + update. The Wiki API binds
	// attachments to a page id, not a slug, so this ordering is forced.
	p, err := client.CreatePage(g.Ctx, c.Slug, c.Title, "")
	if err != nil {
		return err
	}
	// API-created pages are always page_type=wysiwyg (verified
	// empirically); the create response doesn't always include the field,
	// so set it explicitly for the orchestrator's guard.
	p.PageType = wiki.PageTypeWysiwyg
	rewritten, err := syncAttachmentsForWrite(g.Ctx, client, p, body, c.AttachmentsDir, g.Stderr)
	if err != nil {
		return err
	}
	if _, err := client.UpdatePage(g.Ctx, c.Slug, rewritten); err != nil {
		return err
	}
	return render.Confirm(g.Stdout, g.Format(), "created", p.Slug)
}

type UpdatePageCmd struct {
	Slug           string `arg:"" help:"page slug"`
	AttachmentsDir string `name:"attachments-dir" help:"upload referenced local files and rewrite content URLs to server form (YFM markdown only; refuses grid pages, warns on legacy)"`
	BodyInput
}

func (c *UpdatePageCmd) Run(g *Globals) error {
	cfg, err := auth.Load()
	if err != nil {
		return err
	}
	client := wiki.New(cfg)
	body, err := c.Read(g.Stdin)
	if err != nil {
		return err
	}
	if c.AttachmentsDir != "" {
		page, err := client.GetPage(g.Ctx, c.Slug)
		if err != nil {
			return err
		}
		body, err = syncAttachmentsForWrite(g.Ctx, client, page, body, c.AttachmentsDir, g.Stderr)
		if err != nil {
			return err
		}
	}
	p, err := client.UpdatePage(g.Ctx, c.Slug, body)
	if err != nil {
		return err
	}
	return render.Confirm(g.Stdout, g.Format(), "updated", p.Slug)
}

// --- Wiki attachments commands ---

type ListAttachmentsCmd struct {
	PageSlug string `arg:"" name:"page-slug" help:"page slug"`
}

func (c *ListAttachmentsCmd) Run(g *Globals) error {
	cfg, err := auth.Load()
	if err != nil {
		return err
	}
	atts, err := wiki.New(cfg).ListAttachments(g.Ctx, c.PageSlug)
	if err != nil {
		return err
	}
	return render.Many(g.Stdout, g.Format(), atts)
}

type DownloadAttachmentCmd struct {
	PageSlug string `arg:"" name:"page-slug" help:"page slug"`
	Filename string `arg:"" name:"filename" help:"attachment filename"`
	Output   string `name:"output" default:"-" help:"output path; '-' for stdout"`
}

func (c *DownloadAttachmentCmd) Run(g *Globals) error {
	cfg, err := auth.Load()
	if err != nil {
		return err
	}
	var w io.Writer
	if c.Output == "-" {
		w = g.Stdout
	} else {
		f, err := os.Create(c.Output)
		if err != nil {
			return err
		}
		defer f.Close()
		w = f
	}
	return wiki.New(cfg).DownloadAttachment(g.Ctx, c.PageSlug, c.Filename, w)
}

type UploadAttachmentCmd struct {
	PageSlug string `arg:"" name:"page-slug" help:"page slug"`
	File     string `name:"file" required:"" help:"local file path to upload"`
	Name     string `name:"name" help:"attachment filename (defaults to basename of --file)"`
}

func (c *UploadAttachmentCmd) Run(g *Globals) error {
	cfg, err := auth.Load()
	if err != nil {
		return err
	}
	f, err := os.Open(c.File)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	name := c.Name
	if name == "" {
		name = filepath.Base(c.File)
	}
	att, err := wiki.New(cfg).UploadAttachment(g.Ctx, c.PageSlug, name, f, info.Size())
	if err != nil {
		return err
	}
	return render.Confirm(g.Stdout, g.Format(), "uploaded", att.Name)
}

type DeleteAttachmentCmd struct {
	PageSlug string `arg:"" name:"page-slug" help:"page slug"`
	Filename string `arg:"" name:"filename" help:"attachment filename"`
}

func (c *DeleteAttachmentCmd) Run(g *Globals) error {
	cfg, err := auth.Load()
	if err != nil {
		return err
	}
	if err := wiki.New(cfg).DeleteAttachment(g.Ctx, c.PageSlug, c.Filename); err != nil {
		return err
	}
	return render.Confirm(g.Stdout, g.Format(), "deleted", c.Filename)
}

// Run parses argv and dispatches to the matched command. Returns the exit code.
func Run(args []string, version string, stdout, stderr io.Writer, stdin io.Reader) (exit int) {
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(kongExitSentinel); !ok {
				panic(r)
			}
		}
	}()

	var c CLI
	parser, err := kong.New(&c,
		kong.Name("yandex-cli"),
		kong.Description("Read Yandex Tracker; read and write Yandex Wiki."),
		kong.UsageOnError(),
		kong.Writers(stdout, stderr),
		kong.Exit(func(code int) {
			exit = code
			panic(kongExitSentinel{})
		}),
	)
	if err != nil {
		render.Error(stderr, render.Plain, err, 0)
		return 2
	}
	kctx, err := parser.Parse(args)
	if err != nil {
		render.Error(stderr, render.Plain, err, 0)
		return 2
	}

	g := &Globals{
		JSON:   c.JSON,
		Stdout: stdout,
		Stderr: stderr,
		Stdin:  stdin,
		Ctx:    context.Background(),
	}
	if err := kctx.Run(g, version); err != nil {
		render.Error(stderr, g.Format(), err, statusFromErr(err))
		return 1
	}
	return 0
}

type kongExitSentinel struct{}

// Main is invoked from cmd/yandex-cli/main.go.
func Main(version string) {
	os.Exit(Run(os.Args[1:], version, os.Stdout, os.Stderr, os.Stdin))
}
