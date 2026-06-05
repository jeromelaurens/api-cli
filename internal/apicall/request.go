package apicall

import (
	"errors"
	"net/url"
	"path"
	"strings"
)

func BuildTarget(baseURL string, requestPath string, rawQuery string) (*url.URL, error) {
	if requestPath == "" {
		return nil, errors.New("path is required")
	}
	if strings.Contains(requestPath, "://") || strings.HasPrefix(requestPath, "//") {
		return nil, errors.New("absolute URLs are not allowed")
	}
	if !strings.HasPrefix(requestPath, "/") {
		return nil, errors.New("path must be relative and start with /")
	}

	base, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}
	if base.Scheme == "" || base.Host == "" {
		return nil, errors.New("base URL must be absolute")
	}

	target := *base
	target.Path = path.Join(base.Path, requestPath)
	if strings.HasSuffix(requestPath, "/") && !strings.HasSuffix(target.Path, "/") {
		target.Path += "/"
	}
	target.RawQuery = rawQuery
	return &target, nil
}

type Redactor struct {
	secrets []string
}

func NewRedactor(secrets ...string) Redactor {
	filtered := make([]string, 0, len(secrets))
	for _, secret := range secrets {
		if secret != "" {
			filtered = append(filtered, secret)
		}
	}
	return Redactor{secrets: filtered}
}

func (r Redactor) Redact(text string) string {
	for _, secret := range r.secrets {
		text = strings.ReplaceAll(text, secret, "[REDACTED]")
	}
	return text
}
