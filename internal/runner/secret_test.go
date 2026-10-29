package runner

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"
)

func TestSecretSetListCheckAndDeleteWithMemoryStore(t *testing.T) {
	catalogPath := writeCredentialCatalog(t)
	store := NewMemoryCredentialStore()

	set := runCLI(t, testCLIOptions{profilesPath: catalogPath, stdin: "secret-value\n", store: store}, "secret", "set", "dolibarr", "api-key", "--json")
	if set.code != 0 || set.stderr != "" || strings.Contains(set.stdout, "secret-value") {
		t.Fatalf("set exit=%d stdout=%q stderr=%q", set.code, set.stdout, set.stderr)
	}

	list := runCLI(t, testCLIOptions{profilesPath: catalogPath, store: store}, "secret", "list", "dolibarr", "--check", "--json")
	if list.code != 0 || list.stderr != "" {
		t.Fatalf("list exit=%d stdout=%q stderr=%q", list.code, list.stdout, list.stderr)
	}
	var payload struct {
		Profile     string `json:"profile"`
		Credentials []struct {
			Name    string `json:"name"`
			Present bool   `json:"present"`
		} `json:"credentials"`
	}
	mustJSON(t, list.stdout, &payload)
	if payload.Profile != "dolibarr" || len(payload.Credentials) != 1 || payload.Credentials[0].Name != "api-key" || !payload.Credentials[0].Present {
		t.Fatalf("secret list payload = %+v", payload)
	}

	del := runCLI(t, testCLIOptions{profilesPath: catalogPath, store: store}, "secret", "delete", "dolibarr", "api-key", "--json")
	if del.code != 0 || del.stderr != "" {
		t.Fatalf("delete exit=%d stdout=%q stderr=%q", del.code, del.stdout, del.stderr)
	}

	after := runCLI(t, testCLIOptions{profilesPath: catalogPath, store: store}, "secret", "list", "dolibarr", "--check", "--json")
	mustJSON(t, after.stdout, &payload)
	if payload.Credentials[0].Present {
		t.Fatalf("credential should not be present after delete: %+v", payload)
	}
}

func TestProfilesOfOneServiceShareCredentials(t *testing.T) {
	var seen []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, password, _ := r.BasicAuth()
		seen = append(seen, user+":"+password)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{}`)
	}))
	t.Cleanup(server.Close)

	auth := `"auth": {"scheme": "basic", "usernameCredential": "email", "passwordCredential": "api-token"}`
	catalogPath := writeCatalog(t, fmt.Sprintf(`{
		"schemaVersion": 1,
		"profiles": {
			"site-a": {"service": "atlassian", "baseURL": %q, %s},
			"site-b": {"service": "atlassian", "baseURL": %q, %s}
		}
	}`, server.URL, auth, server.URL, auth))
	store := NewMemoryCredentialStore()

	for _, credential := range []struct{ name, value string }{{"email", "me@example.test"}, {"api-token", "shared-token"}} {
		set := runCLI(t, testCLIOptions{profilesPath: catalogPath, stdin: credential.value, store: store}, "secret", "set", "site-a", credential.name)
		if set.code != 0 {
			t.Fatalf("set exit=%d stdout=%q stderr=%q", set.code, set.stdout, set.stderr)
		}
	}
	if len(store.values) != 2 || store.values["atlassian/email"] != "me@example.test" || store.values["atlassian/api-token"] != "shared-token" {
		t.Fatalf("keyring keys = %v, want one entry per service credential", store.values)
	}

	opts := testCLIOptions{profilesPath: catalogPath, store: store}
	if list := runCLI(t, opts, "secret", "list", "site-b", "--check"); strings.Count(list.stdout, "present=true") != 2 {
		t.Fatalf("site-b list = %q", list.stdout)
	}
	if result := runCLI(t, opts, "request", "site-b", "GET", "/myself"); result.code != 0 {
		t.Fatalf("request exit=%d stdout=%q stderr=%q", result.code, result.stdout, result.stderr)
	}
	if fmt.Sprint(seen) != "[me@example.test:shared-token]" {
		t.Fatalf("basic auth seen = %v", seen)
	}
}

func TestSecretCommandsTextOutputDoesNotRevealCredentialValue(t *testing.T) {
	catalogPath := writeCredentialCatalog(t)
	store := NewMemoryCredentialStore()

	set := runCLI(t, testCLIOptions{profilesPath: catalogPath, stdin: "text-secret\n", store: store}, "secret", "set", "dolibarr", "api-key")
	if set.code != 0 || set.stderr != "" {
		t.Fatalf("set exit=%d stdout=%q stderr=%q", set.code, set.stdout, set.stderr)
	}
	if !strings.Contains(set.stdout, "Stored credential dolibarr/api-key") || strings.Contains(set.stdout, "text-secret") {
		t.Fatalf("set text output = %q", set.stdout)
	}

	list := runCLI(t, testCLIOptions{profilesPath: catalogPath, store: store}, "secret", "list", "dolibarr", "--check")
	if list.code != 0 || list.stderr != "" {
		t.Fatalf("list exit=%d stdout=%q stderr=%q", list.code, list.stdout, list.stderr)
	}
	if !strings.Contains(list.stdout, "api-key: present=true") || strings.Contains(list.stdout, "text-secret") {
		t.Fatalf("list text output = %q", list.stdout)
	}

	del := runCLI(t, testCLIOptions{profilesPath: catalogPath, store: store}, "secret", "delete", "dolibarr", "api-key")
	if del.code != 0 || del.stderr != "" {
		t.Fatalf("delete exit=%d stdout=%q stderr=%q", del.code, del.stdout, del.stderr)
	}
	if !strings.Contains(del.stdout, "Deleted credential dolibarr/api-key") || strings.Contains(del.stdout, "text-secret") {
		t.Fatalf("delete text output = %q", del.stdout)
	}
}

func TestSecretListWithoutCheckDoesNotReadCredentialStore(t *testing.T) {
	catalogPath := writeCredentialCatalog(t)
	store := &panicOnGetStore{CredentialStore: NewMemoryCredentialStore()}

	result := runCLI(t, testCLIOptions{profilesPath: catalogPath, store: store}, "secret", "list", "crisp", "--json")
	if result.code != 0 || result.stderr != "" {
		t.Fatalf("exit=%d stdout=%q stderr=%q", result.code, result.stdout, result.stderr)
	}
	var payload struct {
		Credentials []map[string]any `json:"credentials"`
	}
	mustJSON(t, result.stdout, &payload)
	for _, item := range payload.Credentials {
		if _, exists := item["present"]; exists {
			t.Fatalf("present should be omitted without --check: %+v", payload)
		}
	}
}

func TestSecretGetIsNotImplemented(t *testing.T) {
	result := runCLI(t, testCLIOptions{profilesPath: writeCredentialCatalog(t)}, "secret", "get", "dolibarr", "api-key", "--json")
	if result.code == 0 || result.stderr != "" {
		t.Fatalf("exit=%d stdout=%q stderr=%q", result.code, result.stdout, result.stderr)
	}
	assertJSONError(t, result.stdout, CodeInvalidArgs)
}

func TestSecretCommandsValidateProfileCredentialPairs(t *testing.T) {
	catalogPath := writeCredentialCatalog(t)

	result := runCLI(t, testCLIOptions{profilesPath: catalogPath, stdin: "value\n"}, "secret", "set", "dolibarr", "wrong-name", "--json")
	if result.code == 0 {
		t.Fatalf("expected invalid credential to fail: stdout=%q", result.stdout)
	}
	assertJSONError(t, result.stdout, CodeProfileCatalogInvalid)
}

func TestKeyringStoreWorksWithGoKeyringMock(t *testing.T) {
	store := NewMockedKeyringStoreForTest(t)
	if err := store.Set(credentialKey("dolibarr", "api-key"), "mocked-value"); err != nil {
		t.Fatalf("set mocked keyring: %v", err)
	}
	got, err := store.Get(credentialKey("dolibarr", "api-key"))
	if err != nil {
		t.Fatalf("get mocked keyring: %v", err)
	}
	if got != "mocked-value" {
		t.Fatalf("mocked keyring value = %q", got)
	}
	if err := store.Delete(credentialKey("dolibarr", "api-key")); err != nil {
		t.Fatalf("delete mocked keyring: %v", err)
	}
	if _, err := store.Get(credentialKey("dolibarr", "api-key")); codeOf(err) != CodeCredentialMissing {
		t.Fatalf("missing keyring code = %q err=%v", codeOf(err), err)
	}
}

func TestSuccessfulAdminJSONDoesNotContainOK(t *testing.T) {
	result := runCLI(t, testCLIOptions{profilesPath: writeCredentialCatalog(t)}, "profile", "list", "--json")
	var payload map[string]json.RawMessage
	mustJSON(t, result.stdout, &payload)
	if _, exists := payload["ok"]; exists {
		t.Fatalf("successful admin JSON should not contain ok: %s", result.stdout)
	}
}

type panicOnGetStore struct {
	CredentialStore
}

func (s *panicOnGetStore) Get(key string) (string, error) {
	panic("secret list without --check must not read credential store")
}

func writeCredentialCatalog(t *testing.T) string {
	t.Helper()
	return writeCatalog(t, `{
		"schemaVersion": 1,
		"profiles": {
			"dolibarr": {
				"service": "dolibarr",
				"baseURL": "https://dolibarr.example.test/api/index.php",
				"auth": {"scheme": "header", "header": "DOLAPIKEY", "credential": "api-key"}
			},
			"crisp": {
				"service": "crisp",
				"baseURL": "https://api.crisp.example.test/v1",
				"auth": {"scheme": "basic", "usernameCredential": "identifier", "passwordCredential": "key"}
			}
		}
	}`)
}

func NewMockedKeyringStoreForTest(t *testing.T) KeyringCredentialStore {
	t.Helper()
	keyring.MockInit()
	return KeyringCredentialStore{}
}
