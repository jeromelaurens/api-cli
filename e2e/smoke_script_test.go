package e2e

import (
	"os"
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

