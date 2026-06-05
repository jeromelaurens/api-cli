package runner

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRequestAppliesSupportedAuthSchemes(t *testing.T) {
	tests := []struct {
		name       string
		profile    string
		auth       string
		store      map[string]string
		assertAuth func(t *testing.T, r *http.Request)
	}{
		{
			name:    "bearer",
			profile: "mailerlite",
			auth:    `"auth": {"scheme": "bearer", "credential": "api-token"}`,
			store:   map[string]string{"mailerlite/api-token": "bearer-token"},
			assertAuth: func(t *testing.T, r *http.Request) {
				if got := r.Header.Get("Authorization"); got != "Bearer bearer-token" {
					t.Fatalf("authorization = %q", got)
				}
			},
		},
		{
			name:    "header",
			profile: "dolibarr",
			auth:    `"auth": {"scheme": "header", "header": "DOLAPIKEY", "credential": "api-key"}`,
			store:   map[string]string{"dolibarr/api-key": "dolibarr-token"},
			assertAuth: func(t *testing.T, r *http.Request) {
				if got := r.Header.Get("DOLAPIKEY"); got != "dolibarr-token" {
					t.Fatalf("DOLAPIKEY = %q", got)
				}
			},
		},
		{
			name:    "query",
			profile: "pipedrive",
			auth:    `"auth": {"scheme": "query", "param": "api_token", "credential": "api-token"}`,
			store:   map[string]string{"pipedrive/api-token": "query-token"},
			assertAuth: func(t *testing.T, r *http.Request) {
				if got := r.URL.Query().Get("api_token"); got != "query-token" {
					t.Fatalf("api_token = %q", got)
				}
				if got := r.URL.RawQuery; !strings.HasPrefix(got, "limit=10&") {
					t.Fatalf("query order should preserve agent query first, got %q", got)
				}
			},
		},
		{
			name:    "basic",
			profile: "crisp",
			auth:    `"auth": {"scheme": "basic", "usernameCredential": "identifier", "passwordCredential": "key"}`,
			store:   map[string]string{"crisp/identifier": "crisp-id", "crisp/key": "crisp-key"},
			assertAuth: func(t *testing.T, r *http.Request) {
				want := "Basic " + base64.StdEncoding.EncodeToString([]byte("crisp-id:crisp-key"))
				if got := r.Header.Get("Authorization"); got != want {
					t.Fatalf("authorization = %q, want %q", got, want)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				tt.assertAuth(t, r)
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("X-Request-Id", "req-auth")
				fmt.Fprint(w, `{"seen":true}`)
			}))
			t.Cleanup(server.Close)

			catalogPath := writeSingleProfileCatalog(t, tt.profile, server.URL, false, tt.auth)
			store := NewMemoryCredentialStore()
			for key, value := range tt.store {
				if err := store.Set(key, value); err != nil {
					t.Fatalf("store set: %v", err)
				}
			}

			result := runCLI(t, testCLIOptions{profilesPath: catalogPath, store: store}, "request", tt.profile, "get", "/items", "--query", "limit=10")
			if result.code != 0 || result.stderr != "" {
				t.Fatalf("exit=%d stdout=%q stderr=%q", result.code, result.stdout, result.stderr)
			}
			envelope := decodeEnvelope(t, result.stdout)
			if envelope["ok"] != true || envelope["status"].(float64) != 200 {
				t.Fatalf("envelope = %+v", envelope)
			}
			headers := envelope["headers"].(map[string]any)
			if headers["x-request-id"] != "req-auth" {
				t.Fatalf("allowlisted headers = %+v", headers)
			}
		})
	}
}

func TestRequestRejectsUnsafeInputsBeforeHTTPCall(t *testing.T) {
	catalogPath := writeSingleProfileCatalog(t, "dolibarr", "https://example.test/api", true, `"auth": {"scheme": "header", "header": "DOLAPIKEY", "credential": "api-key"}`)
	store := NewMemoryCredentialStore()
	_ = store.Set("dolibarr/api-key", "token")

	tests := []struct {
		name string
		args []string
		code string
	}{
		{"absolute URL", []string{"request", "dolibarr", "GET", "https://evil.test/path"}, CodeInvalidRequestPath},
		{"network path", []string{"request", "dolibarr", "GET", "//evil.test/path"}, CodeInvalidRequestPath},
		{"missing slash", []string{"request", "dolibarr", "GET", "thirdparties"}, CodeInvalidRequestPath},
		{"query in path", []string{"request", "dolibarr", "GET", "/invoices?status=unpaid"}, CodeInvalidRequestPath},
		{"leading query mark", []string{"request", "dolibarr", "GET", "/thirdparties", "--query", "?limit=10"}, CodeInvalidQuery},
		{"forbidden authorization", []string{"request", "dolibarr", "GET", "/thirdparties", "--header", "Authorization=Bearer x"}, CodeForbiddenHeader},
		{"forbidden cookie", []string{"request", "dolibarr", "GET", "/thirdparties", "--header", "Cookie=a=b"}, CodeForbiddenHeader},
		{"forbidden user agent", []string{"request", "dolibarr", "GET", "/thirdparties", "--header", "User-Agent=test"}, CodeForbiddenHeader},
		{"forbidden managed header", []string{"request", "dolibarr", "GET", "/thirdparties", "--header", "DOLAPIKEY=x"}, CodeForbiddenHeader},
		{"read only mutation", []string{"request", "dolibarr", "POST", "/thirdparties"}, CodeProfileReadOnly},
		{"unsupported method", []string{"request", "dolibarr", "TRACE", "/thirdparties"}, CodeInvalidArgs},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := runCLI(t, testCLIOptions{profilesPath: catalogPath, store: store}, tt.args...)
			if result.code == 0 || result.stderr != "" {
				t.Fatalf("exit=%d stdout=%q stderr=%q", result.code, result.stdout, result.stderr)
			}
			assertJSONError(t, result.stdout, tt.code)
			assertNoLeak(t, result.stdout+result.stderr, "token")
		})
	}
}

func TestRequestRejectsQueryAuthCollision(t *testing.T) {
	catalogPath := writeSingleProfileCatalog(t, "pipedrive", "https://example.test/v1", false, `"auth": {"scheme": "query", "param": "api_token", "credential": "api-token"}`)
	store := NewMemoryCredentialStore()
	_ = store.Set("pipedrive/api-token", "query-token")

	result := runCLI(t, testCLIOptions{profilesPath: catalogPath, store: store}, "request", "pipedrive", "GET", "/deals", "--query", "api_token=agent-token")
	if result.code == 0 || result.stderr != "" {
		t.Fatalf("exit=%d stdout=%q stderr=%q", result.code, result.stdout, result.stderr)
	}
	assertJSONError(t, result.stdout, CodeAuthQueryCollision)
	assertNoLeak(t, result.stdout, "query-token")
}

func TestRequestBodyFileAndStdinContentTypePolicy(t *testing.T) {
	var seen []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := readAllString(t, r)
		seen = append(seen, r.Header.Get("Content-Type")+"|"+body)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"ok":true}`)
	}))
	t.Cleanup(server.Close)

	catalogPath := writeSingleProfileCatalog(t, "mailerlite", server.URL, false, `"auth": {"scheme": "bearer", "credential": "api-token"}`)
	store := NewMemoryCredentialStore()
	_ = store.Set("mailerlite/api-token", "bearer-token")

	bodyPath := filepath.Join(t.TempDir(), "body.json")
	if err := os.WriteFile(bodyPath, []byte(`{"name":"Ada"}`), 0o600); err != nil {
		t.Fatalf("write body: %v", err)
	}
	fileResult := runCLI(t, testCLIOptions{profilesPath: catalogPath, store: store}, "request", "mailerlite", "POST", "/subscribers", "--body", bodyPath)
	if fileResult.code != 0 {
		t.Fatalf("file body exit=%d stdout=%q stderr=%q", fileResult.code, fileResult.stdout, fileResult.stderr)
	}

	stdinResult := runCLI(t, testCLIOptions{profilesPath: catalogPath, store: store, stdin: "plain text"}, "request", "mailerlite", "POST", "/subscribers", "--body", "-", "--header", "Content-Type=text/plain")
	if stdinResult.code != 0 {
		t.Fatalf("stdin body exit=%d stdout=%q stderr=%q", stdinResult.code, stdinResult.stdout, stdinResult.stderr)
	}

	if len(seen) != 2 {
		t.Fatalf("requests seen = %d", len(seen))
	}
	if seen[0] != `application/json|{"name":"Ada"}` {
		t.Fatalf("file body seen = %q", seen[0])
	}
	if seen[1] != "text/plain|plain text" {
		t.Fatalf("stdin body seen = %q", seen[1])
	}
}

func TestResponseEnvelopeBodyVariantsAndHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/json":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"items":[1]}`)
		case "/text":
			w.Header().Set("Content-Type", "text/plain")
			fmt.Fprint(w, "hello")
		case "/binary":
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write([]byte{0xff, 0x00, 0x01})
		case "/empty":
			w.WriteHeader(204)
		case "/error":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			fmt.Fprint(w, `{"message":"missing"}`)
		}
	}))
	t.Cleanup(server.Close)

	catalogPath := writeSingleProfileCatalog(t, "mailerlite", server.URL, false, `"auth": {"scheme": "bearer", "credential": "api-token"}`)
	store := NewMemoryCredentialStore()
	_ = store.Set("mailerlite/api-token", "bearer-token")

	jsonResult := requestOK(t, catalogPath, store, "/json")
	if body := jsonResult["body"].(map[string]any); len(body["items"].([]any)) != 1 {
		t.Fatalf("json body = %+v", body)
	}

	textResult := requestOK(t, catalogPath, store, "/text")
	if textResult["bodyText"] != "hello" {
		t.Fatalf("text envelope = %+v", textResult)
	}

	binaryResult := requestOK(t, catalogPath, store, "/binary")
	if binaryResult["bodyEncoding"] != "base64" || binaryResult["bodyBase64"] != "/wAB" {
		t.Fatalf("binary envelope = %+v", binaryResult)
	}

	emptyResult := requestOK(t, catalogPath, store, "/empty")
	if _, exists := emptyResult["body"]; exists {
		t.Fatalf("empty envelope should not contain body: %+v", emptyResult)
	}

	errorResult := runCLI(t, testCLIOptions{profilesPath: catalogPath, store: store}, "request", "mailerlite", "GET", "/error")
	if errorResult.code == 0 || errorResult.stderr != "" {
		t.Fatalf("http error exit=%d stdout=%q stderr=%q", errorResult.code, errorResult.stdout, errorResult.stderr)
	}
	envelope := decodeEnvelope(t, errorResult.stdout)
	if envelope["ok"] != false || envelope["status"].(float64) != 404 {
		t.Fatalf("http error envelope = %+v", envelope)
	}
	if _, exists := envelope["error"]; exists {
		t.Fatalf("HTTP non-2xx should not include CLI error object: %+v", envelope)
	}
}

func TestRequestTimeoutAndResponseTooLarge(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/slow":
			time.Sleep(50 * time.Millisecond)
			fmt.Fprint(w, `{"late":true}`)
		case "/large":
			fmt.Fprint(w, strings.Repeat("x", 20))
		}
	}))
	t.Cleanup(server.Close)

	catalogPath := writeSingleProfileCatalog(t, "mailerlite", server.URL, false, `"auth": {"scheme": "bearer", "credential": "api-token"}`)
	store := NewMemoryCredentialStore()
	_ = store.Set("mailerlite/api-token", "bearer-token")

	timeout := runCLI(t, testCLIOptions{profilesPath: catalogPath, store: store}, "request", "mailerlite", "GET", "/slow", "--timeout", "1ms")
	if timeout.code == 0 || timeout.stderr != "" {
		t.Fatalf("timeout exit=%d stdout=%q stderr=%q", timeout.code, timeout.stdout, timeout.stderr)
	}
	assertJSONError(t, timeout.stdout, CodeRequestTimeout)

	large := runCLI(t, testCLIOptions{profilesPath: catalogPath, store: store}, "request", "mailerlite", "GET", "/large", "--max-body-bytes", "5")
	if large.code == 0 || large.stderr != "" {
		t.Fatalf("large exit=%d stdout=%q stderr=%q", large.code, large.stdout, large.stderr)
	}
	assertJSONError(t, large.stdout, CodeResponseTooLarge)
}

func TestRequestRedactsExactAndDerivedCredentialValues(t *testing.T) {
	identifier := "crisp-identifier"
	key := "crisp-key"
	basicValue := base64.StdEncoding.EncodeToString([]byte(identifier + ":" + key))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"authorization": r.Header.Get("Authorization"),
			"identifier":    identifier,
			"key":           key,
			"pair":          identifier + ":" + key,
			"basic":         basicValue,
		})
	}))
	t.Cleanup(server.Close)

	catalogPath := writeSingleProfileCatalog(t, "crisp", server.URL, false, `"auth": {"scheme": "basic", "usernameCredential": "identifier", "passwordCredential": "key"}`)
	store := NewMemoryCredentialStore()
	_ = store.Set("crisp/identifier", identifier)
	_ = store.Set("crisp/key", key)

	result := runCLI(t, testCLIOptions{profilesPath: catalogPath, store: store}, "request", "crisp", "GET", "/website")
	if result.code != 0 || result.stderr != "" {
		t.Fatalf("exit=%d stdout=%q stderr=%q", result.code, result.stdout, result.stderr)
	}
	for _, secret := range []string{identifier, key, identifier + ":" + key, basicValue, "Basic " + basicValue} {
		assertNoLeak(t, result.stdout+result.stderr, secret)
	}
	if !strings.Contains(result.stdout, "[REDACTED]") {
		t.Fatalf("redacted marker missing from stdout: %s", result.stdout)
	}
}

func TestRequestNetworkErrorDoesNotLeakQueryCredential(t *testing.T) {
	catalogPath := writeSingleProfileCatalog(t, "pipedrive", "http://127.0.0.1:1/v1", false, `"auth": {"scheme": "query", "param": "api_token", "credential": "api-token"}`)
	store := NewMemoryCredentialStore()
	_ = store.Set("pipedrive/api-token", "query-token")

	result := runCLI(t, testCLIOptions{profilesPath: catalogPath, store: store}, "request", "pipedrive", "GET", "/deals")
	if result.code == 0 || result.stderr != "" {
		t.Fatalf("exit=%d stdout=%q stderr=%q", result.code, result.stdout, result.stderr)
	}
	assertJSONError(t, result.stdout, CodeNetworkError)
	assertNoLeak(t, result.stdout+result.stderr, "query-token")
}

func requestOK(t *testing.T, catalogPath string, store CredentialStore, path string) map[string]any {
	t.Helper()
	result := runCLI(t, testCLIOptions{profilesPath: catalogPath, store: store}, "request", "mailerlite", "GET", path)
	if result.code != 0 || result.stderr != "" {
		t.Fatalf("request %s exit=%d stdout=%q stderr=%q", path, result.code, result.stdout, result.stderr)
	}
	envelope := decodeEnvelope(t, result.stdout)
	if envelope["ok"] != true || envelope["status"].(float64) < 200 || envelope["status"].(float64) > 299 {
		t.Fatalf("envelope = %+v", envelope)
	}
	return envelope
}

func decodeEnvelope(t *testing.T, text string) map[string]any {
	t.Helper()
	var envelope map[string]any
	mustJSON(t, text, &envelope)
	return envelope
}

func readAllString(t *testing.T, r *http.Request) string {
	t.Helper()
	data, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return string(data)
}

func assertNoLeak(t *testing.T, text string, secret string) {
	t.Helper()
	if secret != "" && strings.Contains(text, secret) {
		t.Fatalf("secret leaked: %q in %s", secret, text)
	}
}

func writeSingleProfileCatalog(t *testing.T, profileName string, baseURL string, readOnly bool, authObject string) string {
	t.Helper()
	return writeCatalog(t, fmt.Sprintf(`{
		"schemaVersion": 1,
		"profiles": {
			"%s": {
				"service": "%s",
				"baseURL": "%s",
				"readOnly": %t,
				%s
			}
		}
	}`, profileName, profileName, baseURL, readOnly, authObject))
}
