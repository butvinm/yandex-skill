package cli

import (
	"bytes"
	"strings"
	"testing"
)

// runCLI is a test helper: invokes Run with captured stdout/stderr.
func runCLI(t *testing.T, env map[string]string, args ...string) (stdout, stderr string, exit int) {
	t.Helper()
	for k, v := range env {
		t.Setenv(k, v)
	}
	var so, se bytes.Buffer
	exit = Run(args, "test-version", &so, &se, strings.NewReader(""))
	return so.String(), se.String(), exit
}

func TestVersion(t *testing.T) {
	stdout, _, exit := runCLI(t, nil, "version")
	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	if stdout != "test-version\n" {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestHelp(t *testing.T) {
	stdout, _, _ := runCLI(t, nil, "--help")
	if !strings.Contains(stdout, "yandex-cli") {
		t.Errorf("help missing program name: %q", stdout)
	}
	if !strings.Contains(stdout, "tracker") || !strings.Contains(stdout, "wiki") {
		t.Errorf("help missing subcommands: %q", stdout)
	}
}

func TestBodyXorEnforced(t *testing.T) {
	_, stderr, exit := runCLI(t, nil,
		"wiki", "pages", "create", "--slug", "x", "--title", "T", "--body", "b", "--body-file", "f")
	if exit != 2 {
		t.Errorf("exit = %d", exit)
	}
	if !strings.Contains(stderr, "body") {
		t.Errorf("stderr missing 'body' diagnostic: %q", stderr)
	}
}

func TestRequiredArgsValidated(t *testing.T) {
	_, _, exit := runCLI(t, nil, "wiki", "pages", "get")
	if exit != 2 {
		t.Errorf("exit = %d (want 2 for missing positional)", exit)
	}
}

func TestAuthErrorPropagates(t *testing.T) {
	// only token set, no org var → Load() should fail with the no-org-var hint
	t.Setenv("YANDEX_CLI_TOKEN", "tok")
	t.Setenv("YANDEX_CLI_CLOUD_ORG_ID", "")
	t.Setenv("YANDEX_CLI_ORG_ID", "")
	_, stderr, exit := runCLI(t, nil, "tracker", "queues", "list")
	if exit != 1 {
		t.Errorf("exit = %d", exit)
	}
	if !strings.Contains(stderr, "YANDEX_CLI_CLOUD_ORG_ID") || !strings.Contains(stderr, "YANDEX_CLI_ORG_ID") {
		t.Errorf("stderr should name both vars: %q", stderr)
	}
}

func TestParseAllCommandShapes(t *testing.T) {
	// Parse-only smoke test for every command. We check exit=2 only for
	// missing positionals; commands that parse successfully will fail
	// with auth error (exit=1) since env vars aren't set, which still
	// proves the kong tree accepts the shape.
	cases := []struct {
		name   string
		args   []string
		expect int // 1 = parsed, failed at auth; 2 = parse error
	}{
		{"tracker issues list with queue", []string{"tracker", "issues", "list", "--queue", "FOO"}, 1},
		{"tracker issues list with query", []string{"tracker", "issues", "list", "--query", "Status: Open"}, 1},
		{"tracker issues get", []string{"tracker", "issues", "get", "FOO-1"}, 1},
		{"tracker queues list", []string{"tracker", "queues", "list"}, 1},
		{"tracker queues get", []string{"tracker", "queues", "get", "FOO"}, 1},
		{"wiki pages list", []string{"wiki", "pages", "list", "--parent", "team"}, 1},
		{"wiki pages get", []string{"wiki", "pages", "get", "team/notes"}, 1},
		{"wiki pages create", []string{"wiki", "pages", "create", "--slug", "team/n", "--title", "T", "--body", "B"}, 1},
		{"wiki pages update", []string{"wiki", "pages", "update", "team/n", "--body", "B"}, 1},
		{"wiki pages list missing parent", []string{"wiki", "pages", "list"}, 2},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("YANDEX_CLI_TOKEN", "")
			t.Setenv("YANDEX_CLI_CLOUD_ORG_ID", "")
			_, _, exit := runCLI(t, nil, tt.args...)
			if exit != tt.expect {
				t.Errorf("exit = %d, want %d", exit, tt.expect)
			}
		})
	}
}
