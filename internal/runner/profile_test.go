package runner

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveProfilesPathUsesExplicitThenEnvThenDefault(t *testing.T) {
	home := filepath.Join("tmp", "home")

	if got := ResolveProfilesPath("/custom/profiles.json", map[string]string{"API_PROFILES": "/env/profiles.json"}, home); got != "/custom/profiles.json" {
		t.Fatalf("explicit path = %q, want /custom/profiles.json", got)
	}
	if got := ResolveProfilesPath("", map[string]string{"API_PROFILES": "/env/profiles.json"}, home); got != "/env/profiles.json" {
		t.Fatalf("env path = %q, want /env/profiles.json", got)
	}

	want := filepath.Join(home, ".api", "profiles.json")
	if got := ResolveProfilesPath("", nil, home); got != want {
		t.Fatalf("default path = %q, want %q", got, want)
	}
}

func TestLoadCatalogParsesAndValidatesSupportedAuthSchemes(t *testing.T) {
	path := writeCatalog(t, `{
		"schemaVersion": 1,
		"profiles": {
			"dolibarr": {
				"service": "dolibarr",
				"baseURL": "https://dolibarr.example.test/api/index.php",
				"readOnly": true,
				"auth": {"scheme": "header", "header": "DOLAPIKEY", "credential": "api-key"}
			},
			"mailerlite": {
				"service": "mailerlite",
				"baseURL": "https://connect.mailerlite.example.test/api",
				"auth": {"scheme": "bearer", "credential": "api-token"}
			},
			"pipedrive": {
				"service": "pipedrive",
				"baseURL": "https://pipedrive.example.test/v1",
				"auth": {"scheme": "query", "param": "api_token", "credential": "api-token"}
			},
			"crisp": {
				"service": "crisp",
				"baseURL": "https://api.crisp.example.test/v1",
				"auth": {"scheme": "basic", "usernameCredential": "identifier", "passwordCredential": "key"}
			}
		}
	}`)

	catalog, err := LoadCatalog(path)
	if err != nil {
		t.Fatalf("LoadCatalog returned error: %v", err)
	}

	if len(catalog.Profiles) != 4 {
		t.Fatalf("profiles count = %d, want 4", len(catalog.Profiles))
	}
	if !catalog.Profiles["dolibarr"].ReadOnly {
		t.Fatal("dolibarr profile should be read-only")
	}
	if catalog.Profiles["mailerlite"].ReadOnly {
		t.Fatal("readOnly should default to false")
	}
}

func TestLoadCatalogRejectsInvalidCatalogWithStructuredCode(t *testing.T) {
	path := writeCatalog(t, `{
		"schemaVersion": 1,
		"profiles": {
			"bad profile": {
				"service": "dolibarr",
				"baseURL": "ftp://example.test/api",
				"auth": {"scheme": "unknown"}
			}
		}
	}`)

	_, err := LoadCatalog(path)
	if codeOf(err) != CodeProfileCatalogInvalid {
		t.Fatalf("error code = %q, want %q; err=%v", codeOf(err), CodeProfileCatalogInvalid, err)
	}
}

func TestLoadCatalogMissingFileUsesCatalogMissingCode(t *testing.T) {
	_, err := LoadCatalog(filepath.Join(t.TempDir(), "missing.json"))
	if codeOf(err) != CodeProfileCatalogMissing {
		t.Fatalf("error code = %q, want %q; err=%v", codeOf(err), CodeProfileCatalogMissing, err)
	}
}

func writeCatalog(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "profiles.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write catalog: %v", err)
	}
	return path
}
