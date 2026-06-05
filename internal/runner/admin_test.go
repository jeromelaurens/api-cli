package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
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

func TestProfileCreateHeaderProfileFromGenericFlagsCreatesCatalog(t *testing.T) {
	catalogPath := filepath.Join(t.TempDir(), ".api", "profiles.json")

	result := runCLI(t, testCLIOptions{profilesPath: catalogPath},
		"profile", "create", "dolibarr",
		"--service", "dolibarr",
		"--base-url", "https://dolibarr.example.test/api/index.php",
		"--auth", "header",
		"--header", "DOLAPIKEY",
		"--credential", "api-key",
		"--json",
	)
	if result.code != 0 || result.stderr != "" {
		t.Fatalf("exit=%d stdout=%q stderr=%q", result.code, result.stdout, result.stderr)
	}

	var payload struct {
		Profile string   `json:"profile"`
		Path    string   `json:"path"`
		Created bool     `json:"created"`
		Secrets []string `json:"secretCommands"`
	}
	mustJSON(t, result.stdout, &payload)
	if payload.Profile != "dolibarr" || payload.Path != catalogPath || !payload.Created {
		t.Fatalf("create payload = %+v", payload)
	}
	if len(payload.Secrets) != 1 || payload.Secrets[0] != "api secret set dolibarr api-key" {
		t.Fatalf("secret commands = %+v", payload.Secrets)
	}

	catalog, err := LoadCatalog(catalogPath)
	if err != nil {
		t.Fatalf("load created catalog: %v", err)
	}
	profile := catalog.Profiles["dolibarr"]
	if profile.Service != "dolibarr" || profile.BaseURL != "https://dolibarr.example.test/api/index.php" || profile.Auth.Scheme != "header" || profile.Auth.Header != "DOLAPIKEY" || profile.Auth.Credential != "api-key" {
		t.Fatalf("created profile = %+v", profile)
	}
	assertOwnerOnlyCatalogPermissions(t, catalogPath)
}

func TestProfileCreateBearerProfileFromGenericFlagsAndTextOutput(t *testing.T) {
	catalogPath := filepath.Join(t.TempDir(), ".api", "profiles.json")

	result := runCLI(t, testCLIOptions{profilesPath: catalogPath},
		"profile", "create", "mailerlite",
		"--service", "mailerlite",
		"--base-url", "https://connect.mailerlite.com/api",
		"--auth", "bearer",
		"--credential", "api-token",
	)
	if result.code != 0 || result.stderr != "" {
		t.Fatalf("exit=%d stdout=%q stderr=%q", result.code, result.stdout, result.stderr)
	}
	if !strings.Contains(result.stdout, "Created profile mailerlite") || !strings.Contains(result.stdout, "api secret set mailerlite api-token") {
		t.Fatalf("text output = %q", result.stdout)
	}

	catalog, err := LoadCatalog(catalogPath)
	if err != nil {
		t.Fatalf("load created catalog: %v", err)
	}
	profile := catalog.Profiles["mailerlite"]
	if profile.BaseURL != "https://connect.mailerlite.com/api" || profile.Auth.Scheme != "bearer" || profile.Auth.Credential != "api-token" {
		t.Fatalf("created profile = %+v", profile)
	}
}

func TestProfileCreateBasicProfileFromGenericFlags(t *testing.T) {
	catalogPath := filepath.Join(t.TempDir(), ".api", "profiles.json")

	result := runCLI(t, testCLIOptions{profilesPath: catalogPath},
		"profile", "create", "crisp",
		"--service", "crisp",
		"--base-url", "https://api.crisp.chat/v1",
		"--auth", "basic",
		"--username-credential", "identifier",
		"--password-credential", "key",
		"--json",
	)
	if result.code != 0 || result.stderr != "" {
		t.Fatalf("exit=%d stdout=%q stderr=%q", result.code, result.stdout, result.stderr)
	}

	var payload struct {
		Secrets []string `json:"secretCommands"`
	}
	mustJSON(t, result.stdout, &payload)
	if got := strings.Join(payload.Secrets, "\n"); got != "api secret set crisp identifier\napi secret set crisp key" {
		t.Fatalf("secret commands = %+v", payload.Secrets)
	}

	catalog, err := LoadCatalog(catalogPath)
	if err != nil {
		t.Fatalf("load created catalog: %v", err)
	}
	profile := catalog.Profiles["crisp"]
	if profile.Auth.Scheme != "basic" || profile.Auth.UsernameCredential != "identifier" || profile.Auth.PasswordCredential != "key" {
		t.Fatalf("created profile = %+v", profile)
	}
}

func TestProfileCreateGenericInteractiveBearerProfile(t *testing.T) {
	catalogPath := filepath.Join(t.TempDir(), ".api", "profiles.json")
	stdin := strings.Join([]string{
		"",
		"https://api.example.test/v1",
		"bearer",
		"access-token",
	}, "\n") + "\n"

	result := runCLI(t, testCLIOptions{profilesPath: catalogPath, stdin: stdin}, "profile", "create", "custom", "--json")
	if result.code != 0 || result.stderr != "" {
		t.Fatalf("exit=%d stdout=%q stderr=%q", result.code, result.stdout, result.stderr)
	}

	catalog, err := LoadCatalog(catalogPath)
	if err != nil {
		t.Fatalf("load created catalog: %v", err)
	}
	profile := catalog.Profiles["custom"]
	if profile.Service != "custom" || profile.BaseURL != "https://api.example.test/v1" || profile.Auth.Scheme != "bearer" || profile.Auth.Credential != "access-token" {
		t.Fatalf("created profile = %+v", profile)
	}
}

func TestProfileCreateInteractiveAuthPromptListsSupportedSchemes(t *testing.T) {
	label := profileCreateAuthSchemePromptLabel()
	for _, scheme := range []string{"bearer", "header", "query", "basic"} {
		if !strings.Contains(label, scheme) {
			t.Fatalf("auth prompt label %q does not mention %q", label, scheme)
		}
	}
}

func TestProfileCreateGenericInteractiveQueryProfilePromptsForAuthSpecificFields(t *testing.T) {
	catalogPath := filepath.Join(t.TempDir(), ".api", "profiles.json")
	stdin := strings.Join([]string{
		"pipedrive",
		"https://api.pipedrive.test/v1",
		"query",
		"api_token",
		"api-token",
	}, "\n") + "\n"

	result := runCLI(t, testCLIOptions{profilesPath: catalogPath, stdin: stdin}, "profile", "create", "pipedrive", "--json")
	if result.code != 0 || result.stderr != "" {
		t.Fatalf("exit=%d stdout=%q stderr=%q", result.code, result.stdout, result.stderr)
	}

	catalog, err := LoadCatalog(catalogPath)
	if err != nil {
		t.Fatalf("load created catalog: %v", err)
	}
	profile := catalog.Profiles["pipedrive"]
	if profile.Auth.Scheme != "query" || profile.Auth.Param != "api_token" || profile.Auth.Credential != "api-token" {
		t.Fatalf("created profile = %+v", profile)
	}
}

func TestProfileUpdateInteractiveEnterKeepsExistingValues(t *testing.T) {
	catalogPath := writeCatalog(t, `{
		"schemaVersion": 1,
		"profiles": {
			"support": {
				"service": "support",
				"baseURL": "https://support.example.test/v1",
				"readOnly": true,
				"auth": {"scheme": "header", "header": "X-API-Key", "credential": "api-key"}
			}
		}
	}`)
	stdin := strings.Join([]string{"", "", "", "", "", ""}, "\n") + "\n"

	result := runCLI(t, testCLIOptions{profilesPath: catalogPath, stdin: stdin}, "profile", "update", "support", "--json")
	if result.code != 0 || result.stderr != "" {
		t.Fatalf("exit=%d stdout=%q stderr=%q", result.code, result.stdout, result.stderr)
	}
	var payload struct {
		Profile string   `json:"profile"`
		Updated bool     `json:"updated"`
		Secrets []string `json:"secretCommands"`
	}
	mustJSON(t, result.stdout, &payload)
	if payload.Profile != "support" || !payload.Updated {
		t.Fatalf("update payload = %+v", payload)
	}
	if got := strings.Join(payload.Secrets, "\n"); got != "api secret set support api-key" {
		t.Fatalf("secret commands = %+v", payload.Secrets)
	}

	catalog, err := LoadCatalog(catalogPath)
	if err != nil {
		t.Fatalf("load updated catalog: %v", err)
	}
	profile := catalog.Profiles["support"]
	if profile.Service != "support" || profile.BaseURL != "https://support.example.test/v1" || !profile.ReadOnly || profile.Auth.Scheme != "header" || profile.Auth.Header != "X-API-Key" || profile.Auth.Credential != "api-key" {
		t.Fatalf("updated profile = %+v", profile)
	}
}

func TestProfileUpdateInteractiveCanChangeProfileAndAuthScheme(t *testing.T) {
	catalogPath := writeCatalog(t, `{
		"schemaVersion": 1,
		"profiles": {
			"support": {
				"service": "support",
				"baseURL": "https://support.example.test/v1",
				"readOnly": true,
				"auth": {"scheme": "header", "header": "X-API-Key", "credential": "api-key"}
			}
		}
	}`)
	stdin := strings.Join([]string{
		"support-v2",
		"https://support.example.test/v2",
		"false",
		"basic",
		"identifier",
		"key",
	}, "\n") + "\n"

	result := runCLI(t, testCLIOptions{profilesPath: catalogPath, stdin: stdin}, "profile", "update", "support", "--json")
	if result.code != 0 || result.stderr != "" {
		t.Fatalf("exit=%d stdout=%q stderr=%q", result.code, result.stdout, result.stderr)
	}
	var payload struct {
		Secrets []string `json:"secretCommands"`
	}
	mustJSON(t, result.stdout, &payload)
	if got := strings.Join(payload.Secrets, "\n"); got != "api secret set support identifier\napi secret set support key" {
		t.Fatalf("secret commands = %+v", payload.Secrets)
	}

	catalog, err := LoadCatalog(catalogPath)
	if err != nil {
		t.Fatalf("load updated catalog: %v", err)
	}
	profile := catalog.Profiles["support"]
	if profile.Service != "support-v2" || profile.BaseURL != "https://support.example.test/v2" || profile.ReadOnly {
		t.Fatalf("updated non-auth profile fields = %+v", profile)
	}
	if profile.Auth.Scheme != "basic" || profile.Auth.UsernameCredential != "identifier" || profile.Auth.PasswordCredential != "key" {
		t.Fatalf("updated auth = %+v", profile.Auth)
	}
}

func TestProfileModifyIsAliasForProfileUpdate(t *testing.T) {
	catalogPath := writeCatalog(t, `{
		"schemaVersion": 1,
		"profiles": {
			"support": {
				"service": "support",
				"baseURL": "https://support.example.test/v1",
				"auth": {"scheme": "bearer", "credential": "api-token"}
			}
		}
	}`)
	stdin := strings.Join([]string{"", "https://support.example.test/v2", "", "", ""}, "\n") + "\n"

	result := runCLI(t, testCLIOptions{profilesPath: catalogPath, stdin: stdin}, "profile", "modify", "support", "--json")
	if result.code != 0 || result.stderr != "" {
		t.Fatalf("exit=%d stdout=%q stderr=%q", result.code, result.stdout, result.stderr)
	}

	catalog, err := LoadCatalog(catalogPath)
	if err != nil {
		t.Fatalf("load updated catalog: %v", err)
	}
	profile := catalog.Profiles["support"]
	if profile.BaseURL != "https://support.example.test/v2" || profile.Auth.Scheme != "bearer" || profile.Auth.Credential != "api-token" {
		t.Fatalf("updated profile = %+v", profile)
	}
}

func TestProfileCreateRejectsDuplicateProfile(t *testing.T) {
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

	result := runCLI(t, testCLIOptions{profilesPath: catalogPath},
		"profile", "create", "mailerlite",
		"--service", "mailerlite",
		"--base-url", "https://connect.mailerlite.com/api",
		"--auth", "bearer",
		"--credential", "api-token",
		"--json",
	)
	if result.code == 0 || result.stderr != "" {
		t.Fatalf("exit=%d stdout=%q stderr=%q", result.code, result.stdout, result.stderr)
	}
	assertJSONError(t, result.stdout, CodeProfileCatalogInvalid)
}

func TestProfileCreateRejectsDuplicateProfileBeforeInteractivePrompts(t *testing.T) {
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

	result := runCLI(t, testCLIOptions{profilesPath: catalogPath}, "profile", "create", "mailerlite", "--json")
	if result.code == 0 || result.stderr != "" {
		t.Fatalf("exit=%d stdout=%q stderr=%q", result.code, result.stdout, result.stderr)
	}
	var payload struct {
		OK    bool `json:"ok"`
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	mustJSON(t, result.stdout, &payload)
	if payload.OK || payload.Error.Code != CodeProfileCatalogInvalid || !strings.Contains(payload.Error.Message, "profile mailerlite already exists") {
		t.Fatalf("error payload = %+v", payload)
	}
}

func TestProfileCreateRejectsPresetsToKeepCLIGeneric(t *testing.T) {
	result := runCLI(t, testCLIOptions{profilesPath: filepath.Join(t.TempDir(), "profiles.json")},
		"profile", "create", "dolibarr", "--preset", "dolibarr", "--json",
	)
	if result.code == 0 || result.stderr != "" {
		t.Fatalf("exit=%d stdout=%q stderr=%q", result.code, result.stdout, result.stderr)
	}
	assertJSONError(t, result.stdout, CodeInvalidArgs)
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

func assertOwnerOnlyCatalogPermissions(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat catalog: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("catalog mode = %o, want 600", got)
	}
	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("stat catalog dir: %v", err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("catalog dir mode = %o, want 700", got)
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
