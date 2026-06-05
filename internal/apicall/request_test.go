package apicall

import "testing"

func TestBuildTargetRejectsAbsoluteURL(t *testing.T) {
	_, err := BuildTarget("https://example.test/api", "https://evil.test/invoices", "limit=10")
	if err == nil {
		t.Fatal("expected absolute model-provided URL to be rejected")
	}
}

func TestBuildTargetRejectsQueryInRequestPath(t *testing.T) {
	_, err := BuildTarget("https://example.test/api", "/invoices?status=unpaid", "")
	if err == nil {
		t.Fatal("expected query in request path to be rejected")
	}
}

func TestBuildTargetPreservesRawQuery(t *testing.T) {
	target, err := BuildTarget("https://example.test/api", "/invoices", "status=unpaid&filter=a%2Fb")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	const want = "https://example.test/api/invoices?status=unpaid&filter=a%2Fb"
	if target.String() != want {
		t.Fatalf("target URL = %q, want %q", target.String(), want)
	}
}

func TestRedactorMasksRegisteredSecrets(t *testing.T) {
	redactor := NewRedactor("dolibarr-token", "pipedrive-token")

	got := redactor.Redact("Authorization: Bearer dolibarr-token\napi_key=pipedrive-token")
	const want = "Authorization: Bearer [REDACTED]\napi_key=[REDACTED]"
	if got != want {
		t.Fatalf("redacted text = %q, want %q", got, want)
	}
}
