package cli

import (
	"context"
	"io"
	"os"

	"github.com/alecthomas/kong"

	"github.com/butvinm/yandex-cli/internal/auth"
	"github.com/butvinm/yandex-cli/internal/render"
	"github.com/butvinm/yandex-cli/internal/tracker"
	"github.com/butvinm/yandex-cli/internal/wiki"
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
	Issues TrackerIssuesCmd `cmd:"" help:"issues"`
	Queues TrackerQueuesCmd `cmd:"" help:"queues"`
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
	Pages WikiPagesCmd `cmd:"" help:"pages"`
}

type WikiPagesCmd struct {
	List   ListPagesCmd   `cmd:"" help:"list page descendants by parent slug"`
	Get    GetPageCmd     `cmd:"" help:"get a page by slug"`
	Create CreatePageCmd  `cmd:"" help:"create a page"`
	Update UpdatePageCmd  `cmd:"" help:"update a page body"`
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

// --- Wiki commands ---

type GetPageCmd struct {
	Slug string `arg:"" help:"page slug (e.g. team/notes/2026-04-29)"`
}

func (c *GetPageCmd) Run(g *Globals) error {
	cfg, err := auth.Load()
	if err != nil {
		return err
	}
	p, err := wiki.New(cfg).GetPage(g.Ctx, c.Slug)
	if err != nil {
		return err
	}
	return render.One(g.Stdout, g.Format(), *p)
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
	Slug  string `name:"slug" required:"" help:"target slug"`
	Title string `name:"title" required:"" help:"page title"`
	BodyInput
}

func (c *CreatePageCmd) Run(g *Globals) error {
	cfg, err := auth.Load()
	if err != nil {
		return err
	}
	body, err := c.Read(g.Stdin)
	if err != nil {
		return err
	}
	p, err := wiki.New(cfg).CreatePage(g.Ctx, c.Slug, c.Title, body)
	if err != nil {
		return err
	}
	return render.Confirm(g.Stdout, g.Format(), "created", p.Slug)
}

type UpdatePageCmd struct {
	Slug string `arg:"" help:"page slug"`
	BodyInput
}

func (c *UpdatePageCmd) Run(g *Globals) error {
	cfg, err := auth.Load()
	if err != nil {
		return err
	}
	body, err := c.Read(g.Stdin)
	if err != nil {
		return err
	}
	p, err := wiki.New(cfg).UpdatePage(g.Ctx, c.Slug, body)
	if err != nil {
		return err
	}
	return render.Confirm(g.Stdout, g.Format(), "updated", p.Slug)
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
