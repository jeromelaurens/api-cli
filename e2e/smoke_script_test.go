package e2e

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRealSmokeScriptIsSafeAndDocumentsTargetProfiles(t *testing.T) {
	path := filepath.Join(repoRoot(t), "scripts", "smoke-real.sh")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("smoke script missing: %v", err)
	}
	if info.Mode()&0o111 == 0 {
		t.Fatalf("smoke script should be executable, mode=%v", info.Mode())
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read smoke script: %v", err)
	}
	text := string(data)
	for _, want := range []string{
		"api request dolibarr GET",
		"api request mailerlite GET /subscribers --query limit=0",
		"api request pipedrive GET /users/me",
		"api request pennylane GET /me",
		"api request crisp GET",
		"CRISP_SMOKE_PATH",
		"DOLIBARR_SMOKE_PATH",
		"bodyType",
		"leakCheck",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("smoke script missing %q", want)
		}
	}
	for _, forbidden := range []string{
		"--header Authorization=",
		"api_token=",
		"secret get",
		"YOUR_COMPANY_API_TOKEN",
		"e2e-bearer-token",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("smoke script contains forbidden text %q", forbidden)
		}
	}
}

func TestRealProfileConfigureScriptUsesGenericFlagsAndNoSecrets(t *testing.T) {
	path := filepath.Join(repoRoot(t), "scripts", "configure-real-profiles.sh")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("configure script missing: %v", err)
	}
	if info.Mode()&0o111 == 0 {
		t.Fatalf("configure script should be executable, mode=%v", info.Mode())
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read configure script: %v", err)
	}
	text := string(data)
	for _, want := range []string{
		"DOLIBARR_BASE_URL",
		"api profile create dolibarr",
		"--auth header",
		"--header DOLAPIKEY",
		"api profile create mailerlite",
		"--auth bearer",
		"api profile create pipedrive",
		"--auth query",
		"--param api_token",
		"api profile create pennylane",
		"api profile create crisp",
		"--auth basic",
		"--username-credential identifier",
		"--password-credential key",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("configure script missing %q", want)
		}
	}
	for _, forbidden := range []string{
		"--preset",
		"secret set",
		"secret get",
		"Authorization=",
		"YOUR_",
		"token-value",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("configure script contains forbidden text %q", forbidden)
		}
	}
}

func TestRealProfileConfigureScriptCreatesProfilesInTemporaryCatalog(t *testing.T) {
	root := repoRoot(t)
	catalogPath := filepath.Join(t.TempDir(), "profiles.json")
	cmd := exec.Command(filepath.Join(root, "scripts", "configure-real-profiles.sh"))
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"API_BIN="+goRunAPICommand(t, root, catalogPath),
		"DOLIBARR_BASE_URL=https://dolibarr.example.test/api/index.php",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("configure profiles failed: %v\n%s", err, output)
	}

	data, err := os.ReadFile(catalogPath)
	if err != nil {
		t.Fatalf("read catalog: %v\nscript output:\n%s", err, output)
	}
	var catalog struct {
		Profiles map[string]struct {
			Service string `json:"service"`
			Auth    struct {
				Scheme string `json:"scheme"`
			} `json:"auth"`
		} `json:"profiles"`
	}
	if err := json.Unmarshal(data, &catalog); err != nil {
		t.Fatalf("catalog JSON: %v\n%s", err, data)
	}
	want := map[string]string{
		"dolibarr":   "header",
		"mailerlite": "bearer",
		"pipedrive":  "query",
		"pennylane":  "bearer",
		"crisp":      "basic",
	}
	for profile, scheme := range want {
		got, ok := catalog.Profiles[profile]
		if !ok {
			t.Fatalf("profile %s missing in catalog: %+v", profile, catalog.Profiles)
		}
		if got.Auth.Scheme != scheme {
			t.Fatalf("profile %s scheme = %q, want %q", profile, got.Auth.Scheme, scheme)
		}
	}
}

func goRunAPICommand(t *testing.T, root string, catalogPath string) string {
	t.Helper()
	return "go run ./cmd/api --profiles " + catalogPath
}
