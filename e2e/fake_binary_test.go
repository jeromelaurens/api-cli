package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestAPIBinaryAgainstFakeHTTPServerWithoutOSKeyring(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ping" {
			t.Fatalf("path = %q, want /ping", r.URL.Path)
		}
		if r.URL.Query().Get("limit") != "1" {
			t.Fatalf("limit query = %q", r.URL.Query().Get("limit"))
		}
		if got := r.Header.Get("Authorization"); got != "Bearer e2e-bearer-token" {
			t.Fatalf("authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-Id", "req-e2e")
		fmt.Fprintf(w, `{"authorization":%q,"result":"ok"}`, r.Header.Get("Authorization"))
	}))
	t.Cleanup(server.Close)

	root := repoRoot(t)
	binary := filepath.Join(t.TempDir(), "api")
	build := exec.Command("go", "build", "-tags", "apicli_e2e_mock_keyring", "-o", binary, "./cmd/api")
	build.Dir = root
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build api binary: %v\n%s", err, output)
	}

	catalogPath := filepath.Join(t.TempDir(), "profiles.json")
	catalog := fmt.Sprintf(`{
		"schemaVersion": 1,
		"profiles": {
			"e2e": {
				"service": "e2e",
				"baseURL": "%s",
				"auth": {"scheme": "bearer", "credential": "api-token"}
			}
		}
	}`, server.URL)
	if err := os.WriteFile(catalogPath, []byte(catalog), 0o600); err != nil {
		t.Fatalf("write catalog: %v", err)
	}

	run := exec.Command(binary, "--profiles", catalogPath, "request", "e2e", "GET", "/ping", "--query", "limit=1")
	var stdout, stderr bytes.Buffer
	run.Stdout = &stdout
	run.Stderr = &stderr
	if err := run.Run(); err != nil {
		t.Fatalf("run api binary: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	if stderr.String() != "" {
		t.Fatalf("stderr should be empty, got %q", stderr.String())
	}
	if strings.Contains(stdout.String(), "e2e-bearer-token") || strings.Contains(stderr.String(), "e2e-bearer-token") {
		t.Fatalf("fake credential leaked\nstdout=%s\nstderr=%s", stdout.String(), stderr.String())
	}

	var envelope map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, stdout.String())
	}
	if envelope["ok"] != true || envelope["status"].(float64) != 200 {
		t.Fatalf("envelope = %+v", envelope)
	}
	body := envelope["body"].(map[string]any)
	if body["authorization"] != "Bearer [REDACTED]" || body["result"] != "ok" {
		t.Fatalf("body = %+v", body)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve caller path")
	}
	return filepath.Dir(filepath.Dir(file))
}
