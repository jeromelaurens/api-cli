package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestProfilePathTextAndJSON(t *testing.T) {
	catalogPath := filepath.Join(t.TempDir(), "missing.json")

	text := runCLI(t, testCLIOptions{profilesPath: catalogPath}, "profile", "path")
	if text.code != 0 {
		t.Fatalf("exit = %d, want 0; stderr=%s", text.code, text.stderr)
	}
	if !strings.Contains(text.stdout, "Profiles catalog: "+catalogPath) {
		t.Fatalf("text stdout missing path:\n%s", text.stdout)
	}

	machine := runCLI(t, testCLIOptions{profilesPath: catalogPath}, "profile", "path", "--json")
	if machine.code != 0 || machine.stderr != "" {
		t.Fatalf("json exit=%d stderr=%q stdout=%s", machine.code, machine.stderr, machine.stdout)
	}
	var payload struct {
		Path   string `json:"path"`
		Exists bool   `json:"exists"`
	}
	mustJSON(t, machine.stdout, &payload)
	if payload.Path != catalogPath || payload.Exists {
		t.Fatalf("profile path payload = %+v", payload)
	}
}

func TestProfileListMissingCatalogSucceedsAsEmptyList(t *testing.T) {
	catalogPath := filepath.Join(t.TempDir(), "missing.json")

	result := runCLI(t, testCLIOptions{profilesPath: catalogPath}, "profile", "list", "--json")
	if result.code != 0 || result.stderr != "" {
		t.Fatalf("exit=%d stderr=%q stdout=%s", result.code, result.stderr, result.stdout)
	}

	var payload struct {
		Profiles      []string `json:"profiles"`
		CatalogExists bool     `json:"catalogExists"`
	}
	mustJSON(t, result.stdout, &payload)
	if len(payload.Profiles) != 0 || payload.CatalogExists {
		t.Fatalf("profile list payload = %+v", payload)
	}
}

func TestProfileListTextOutput(t *testing.T) {
	catalogPath := writeCatalog(t, `{
		"schemaVersion": 1,
		"profiles": {
			"b": {"service": "b", "baseURL": "https://b.example.test", "auth": {"scheme": "bearer", "credential": "api-token"}},
			"a": {"service": "a", "baseURL": "https://a.example.test", "auth": {"scheme": "bearer", "credential": "api-token"}}
		}
	}`)

	result := runCLI(t, testCLIOptions{profilesPath: catalogPath}, "profile", "list")
	if result.code != 0 || result.stderr != "" {
		t.Fatalf("exit=%d stdout=%q stderr=%q", result.code, result.stdout, result.stderr)
	}
	if !strings.Contains(result.stdout, "Profiles catalog: "+catalogPath) || !strings.Contains(result.stdout, "  a\n  b\n") {
		t.Fatalf("profile list text = %q", result.stdout)
	}
}

func TestProfileShowJSONAndTextErrorPolicy(t *testing.T) {
	catalogPath := writeCatalog(t, `{
		"schemaVersion": 1,
		"profiles": {
			"mailerlite": {
				"service": "mailerlite",
				"baseURL": "https://connect.mailerlite.example.test/api",
				"auth": {"scheme": "bearer", "credential": "api-token"}
			}
		}
	}`)

	show := runCLI(t, testCLIOptions{profilesPath: catalogPath}, "profile", "show", "mailerlite", "--json")
	if show.code != 0 {
		t.Fatalf("exit=%d stderr=%q stdout=%s", show.code, show.stderr, show.stdout)
	}
	var profile map[string]any
	mustJSON(t, show.stdout, &profile)
	if profile["baseURL"] != "https://connect.mailerlite.example.test/api" {
		t.Fatalf("profile JSON = %+v", profile)
	}

	missingText := runCLI(t, testCLIOptions{profilesPath: catalogPath}, "profile", "show", "missing")
	if missingText.code == 0 || missingText.stdout != "" || !strings.Contains(missingText.stderr, CodeProfileNotFound) {
		t.Fatalf("text error policy stdout=%q stderr=%q exit=%d", missingText.stdout, missingText.stderr, missingText.code)
	}

	missingJSON := runCLI(t, testCLIOptions{profilesPath: catalogPath}, "profile", "show", "missing", "--json")
	if missingJSON.code == 0 || missingJSON.stderr != "" {
		t.Fatalf("json error policy stdout=%q stderr=%q exit=%d", missingJSON.stdout, missingJSON.stderr, missingJSON.code)
	}
	assertJSONError(t, missingJSON.stdout, CodeProfileNotFound)
}

func TestGlobalAndSubcommandHelpThroughCLI(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{[]string{"--help"}, "Response envelope:"},
		{[]string{"help"}, "Response envelope:"},
		{[]string{"request", "--help"}, "Methods:"},
		{[]string{"profile", "--help"}, "api profile path"},
		{[]string{"secret", "--help"}, "There is no secret get command."},
	}
	for _, tt := range tests {
		result := runCLI(t, testCLIOptions{}, tt.args...)
		if result.code != 0 || result.stderr != "" || !strings.Contains(result.stdout, "Usage:") || !strings.Contains(result.stdout, tt.want) {
			t.Fatalf("args=%v want=%q exit=%d stdout=%q stderr=%q", tt.args, tt.want, result.code, result.stdout, result.stderr)
		}
	}
}

type testCLIOptions struct {
	profilesPath string
	stdin        string
	store        CredentialStore
	client       HTTPDoer
}

type cliResult struct {
	code   int
	stdout string
	stderr string
}

func runCLI(t *testing.T, opts testCLIOptions, args ...string) cliResult {
	t.Helper()

	var stdout, stderr bytes.Buffer
	store := opts.store
	if store == nil {
		store = NewMemoryCredentialStore()
	}
	client := opts.client
	if client == nil {
		client = defaultHTTPClient()
	}

	cli := &CLI{
		In:              strings.NewReader(opts.stdin),
		Out:             &stdout,
		Err:             &stderr,
		Env:             map[string]string{},
		HomeDir:         t.TempDir(),
		ProfilesPath:    opts.profilesPath,
		CredentialStore: store,
		HTTPClient:      client,
	}
	if opts.profilesPath != "" {
		args = append([]string{"--profiles", opts.profilesPath}, args...)
	}
	code := cli.Run(context.Background(), args)
	return cliResult{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

func mustJSON(t *testing.T, text string, dst any) {
	t.Helper()
	if err := json.Unmarshal([]byte(text), dst); err != nil {
		t.Fatalf("invalid JSON %q: %v", text, err)
	}
}

func assertJSONError(t *testing.T, text string, code string) {
	t.Helper()
	var payload struct {
		OK    bool `json:"ok"`
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	mustJSON(t, text, &payload)
	if payload.OK || payload.Error.Code != code {
		t.Fatalf("error payload = %+v, want code %s", payload, code)
	}
}
