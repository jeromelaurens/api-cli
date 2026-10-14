package runner

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jeromelaurens/api-cli/internal/apicall"
)

const (
	defaultTimeout      = 30 * time.Second
	defaultMaxBodyBytes = 5 * 1024 * 1024
)

type requestCommand struct {
	Profile      string
	Method       string
	Path         string
	Query        string
	BodyPath     string
	Headers      []string
	Timeout      time.Duration
	MaxBodyBytes int64
}

type requestMeta struct {
	Profile string
	Method  string
	Path    string
}

func (c *CLI) runRequest(ctx context.Context, args []string) int {
	if hasHelp(args) {
		fmt.Fprint(c.Out, HelpText("request"))
		return 0
	}

	cmd, err := parseRequestArgs(args)
	meta := requestMeta{}
	if err != nil {
		return c.writeRequestError(err, nil, meta)
	}
	meta = requestMeta{Profile: cmd.Profile, Method: cmd.Method, Path: cmd.Path}

	if err := validateRequestPath(cmd.Path); err != nil {
		return c.writeRequestError(err, nil, meta)
	}
	if strings.HasPrefix(cmd.Query, "?") {
		return c.writeRequestError(codedError(CodeInvalidQuery, "--query must not start with ?"), nil, meta)
	}

	catalog, err := LoadCatalog(c.ProfilesPath)
	if err != nil {
		return c.writeRequestError(err, nil, meta)
	}
	profile, err := lookupProfile(catalog, cmd.Profile)
	if err != nil {
		return c.writeRequestError(err, nil, meta)
	}
	if profile.ReadOnly && isMutation(cmd.Method) {
		return c.writeRequestError(codedError(CodeProfileReadOnly, "profile %s is read-only and rejects %s", cmd.Profile, cmd.Method), nil, meta)
	}
	if profile.Auth.Scheme == "query" {
		collides, err := queryHasParam(cmd.Query, profile.Auth.Param)
		if err != nil {
			return c.writeRequestError(err, nil, meta)
		}
		if collides {
			return c.writeRequestError(codedError(CodeAuthQueryCollision, "query already contains auth parameter %s", profile.Auth.Param), nil, meta)
		}
	}

	agentHeaders, err := parseAgentHeaders(cmd.Headers, profile.Auth)
	if err != nil {
		return c.writeRequestError(err, nil, meta)
	}
	body, err := c.readRequestBody(cmd.BodyPath)
	if err != nil {
		return c.writeRequestError(err, nil, meta)
	}

	redactor := NewOutputRedactor()
	credentials, err := c.loadRequestCredentials(cmd.Profile, profile, redactor)
	if err != nil {
		return c.writeRequestError(err, redactor, meta)
	}

	rawQuery := cmd.Query
	if profile.Auth.Scheme == "query" {
		value := credentials[profile.Auth.Credential]
		rawQuery = appendRawQuery(rawQuery, profile.Auth.Param, value)
	}
	target, err := apicall.BuildTarget(profile.BaseURL, cmd.Path, rawQuery)
	if err != nil {
		return c.writeRequestError(codedError(CodeInvalidRequestPath, "%s", err.Error()), redactor, meta)
	}
	redactor.Register(target.String())

	reqCtx, cancel := context.WithTimeout(ctx, cmd.Timeout)
	defer cancel()

	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(reqCtx, cmd.Method, target.String(), bodyReader)
	if err != nil {
		return c.writeRequestError(wrapError(CodeInternalError, err, "cannot build HTTP request"), redactor, meta)
	}
	applyDefaultHeaders(req)
	for name, value := range profile.Headers {
		req.Header.Set(name, value)
	}
	for name, values := range agentHeaders {
		req.Header.Del(name)
		for _, value := range values {
			req.Header.Add(name, value)
		}
	}
	if body != nil && req.Header.Get("Content-Type") == "" && json.Valid(bytes.TrimSpace(body)) {
		req.Header.Set("Content-Type", "application/json")
	}
	applyAuth(req, target, profile.Auth, credentials, redactor)
	redactor.Register(target.String())

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(reqCtx.Err(), context.DeadlineExceeded) {
			return c.writeRequestError(codedError(CodeRequestTimeout, "request timed out after %s", cmd.Timeout), redactor, meta)
		}
		return c.writeRequestError(wrapError(CodeNetworkError, err, "network request failed"), redactor, meta)
	}
	defer resp.Body.Close()

	data, err := readLimitedResponseBody(resp.Body, cmd.MaxBodyBytes)
	if err != nil {
		if errors.Is(err, errResponseTooLarge) {
			return c.writeRequestError(codedError(CodeResponseTooLarge, "response body exceeded %d bytes", cmd.MaxBodyBytes), redactor, meta)
		}
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(reqCtx.Err(), context.DeadlineExceeded) {
			return c.writeRequestError(codedError(CodeRequestTimeout, "request timed out after %s", cmd.Timeout), redactor, meta)
		}
		return c.writeRequestError(wrapError(CodeNetworkError, err, "cannot read HTTP response"), redactor, meta)
	}

	envelope := buildResponseEnvelope(meta, resp, data)
	writeRedactedJSON(c.Out, envelope, redactor)
	if resp.StatusCode >= 200 && resp.StatusCode <= 299 {
		return 0
	}
	return 1
}

func parseRequestArgs(args []string) (requestCommand, error) {
	cmd := requestCommand{Timeout: defaultTimeout, MaxBodyBytes: defaultMaxBodyBytes}
	positionals := make([]string, 0, 3)
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--query":
			value, next, err := requireFlagValue(args, i, "--query")
			if err != nil {
				return cmd, err
			}
			cmd.Query = value
			i = next
		case strings.HasPrefix(arg, "--query="):
			cmd.Query = strings.TrimPrefix(arg, "--query=")
		case arg == "--body":
			value, next, err := requireFlagValue(args, i, "--body")
			if err != nil {
				return cmd, err
			}
			cmd.BodyPath = value
			i = next
		case strings.HasPrefix(arg, "--body="):
			cmd.BodyPath = strings.TrimPrefix(arg, "--body=")
		case arg == "--header":
			value, next, err := requireFlagValue(args, i, "--header")
			if err != nil {
				return cmd, err
			}
			cmd.Headers = append(cmd.Headers, value)
			i = next
		case strings.HasPrefix(arg, "--header="):
			cmd.Headers = append(cmd.Headers, strings.TrimPrefix(arg, "--header="))
		case arg == "--timeout":
			value, next, err := requireFlagValue(args, i, "--timeout")
			if err != nil {
				return cmd, err
			}
			timeout, err := time.ParseDuration(value)
			if err != nil || timeout <= 0 {
				return cmd, codedError(CodeInvalidArgs, "--timeout must be a positive duration")
			}
			cmd.Timeout = timeout
			i = next
		case strings.HasPrefix(arg, "--timeout="):
			timeout, err := time.ParseDuration(strings.TrimPrefix(arg, "--timeout="))
			if err != nil || timeout <= 0 {
				return cmd, codedError(CodeInvalidArgs, "--timeout must be a positive duration")
			}
			cmd.Timeout = timeout
		case arg == "--max-body-bytes":
			value, next, err := requireFlagValue(args, i, "--max-body-bytes")
			if err != nil {
				return cmd, err
			}
			maxBodyBytes, err := strconv.ParseInt(value, 10, 64)
			if err != nil || maxBodyBytes <= 0 {
				return cmd, codedError(CodeInvalidArgs, "--max-body-bytes must be a positive integer")
			}
			cmd.MaxBodyBytes = maxBodyBytes
			i = next
		case strings.HasPrefix(arg, "--max-body-bytes="):
			maxBodyBytes, err := strconv.ParseInt(strings.TrimPrefix(arg, "--max-body-bytes="), 10, 64)
			if err != nil || maxBodyBytes <= 0 {
				return cmd, codedError(CodeInvalidArgs, "--max-body-bytes must be a positive integer")
			}
			cmd.MaxBodyBytes = maxBodyBytes
		case strings.HasPrefix(arg, "-"):
			return cmd, codedError(CodeInvalidArgs, "unknown request flag: %s", arg)
		default:
			positionals = append(positionals, arg)
		}
	}
	if len(positionals) != 3 {
		return cmd, codedError(CodeInvalidArgs, "usage: api request <profile> <METHOD> <path>")
	}
	method, err := normalizeMethod(positionals[1])
	if err != nil {
		return cmd, err
	}
	cmd.Profile = positionals[0]
	cmd.Method = method
	cmd.Path = positionals[2]
	return cmd, nil
}

func requireFlagValue(args []string, index int, name string) (string, int, error) {
	if index+1 >= len(args) {
		return "", index, codedError(CodeInvalidArgs, "%s requires a value", name)
	}
	return args[index+1], index + 1, nil
}

func normalizeMethod(method string) (string, error) {
	upper := strings.ToUpper(method)
	switch upper {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodHead, http.MethodOptions:
		return upper, nil
	default:
		return "", codedError(CodeInvalidArgs, "HTTP method is not allowed: %s", method)
	}
}

func validateRequestPath(requestPath string) error {
	if requestPath == "" {
		return codedError(CodeInvalidRequestPath, "path is required")
	}
	if strings.Contains(requestPath, "://") || strings.HasPrefix(requestPath, "//") {
		return codedError(CodeInvalidRequestPath, "absolute URLs are not allowed")
	}
	if strings.Contains(requestPath, "?") {
		return codedError(CodeInvalidRequestPath, "path must not contain ?; use --query for query parameters")
	}
	if !strings.HasPrefix(requestPath, "/") {
		return codedError(CodeInvalidRequestPath, "path must start with /")
	}
	return nil
}

func isMutation(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func parseAgentHeaders(inputs []string, auth AuthConfig) (http.Header, error) {
	headers := make(http.Header)
	for _, input := range inputs {
		name, value, ok := strings.Cut(input, "=")
		if !ok || name == "" {
			return nil, codedError(CodeInvalidArgs, "headers must use Name=Value")
		}
		if !allowedHeader(name, value, auth) {
			return nil, codedError(CodeForbiddenHeader, "header %s is not allowed", name)
		}
		headers.Set(http.CanonicalHeaderKey(name), value)
	}
	return headers, nil
}

func allowedHeader(name string, value string, auth AuthConfig) bool {
	return validHeaderName(name) && !strings.ContainsAny(value, "\r\n") && !isForbiddenHeader(name, auth)
}

func isForbiddenHeader(name string, auth AuthConfig) bool {
	lower := strings.ToLower(name)
	switch lower {
	case "authorization", "cookie", "set-cookie", "user-agent":
		return true
	}
	return auth.Scheme == "header" && strings.EqualFold(name, auth.Header)
}

func queryHasParam(rawQuery string, param string) (bool, error) {
	if rawQuery == "" {
		return false, nil
	}
	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		return false, wrapError(CodeInvalidQuery, err, "query string is invalid")
	}
	_, exists := values[param]
	return exists, nil
}

func (c *CLI) readRequestBody(bodyPath string) ([]byte, error) {
	if bodyPath == "" {
		return nil, nil
	}
	if bodyPath == "-" {
		data, err := io.ReadAll(c.In)
		if err != nil {
			return nil, wrapError(CodeInvalidArgs, err, "cannot read request body from stdin")
		}
		return data, nil
	}
	data, err := os.ReadFile(bodyPath)
	if err != nil {
		return nil, wrapError(CodeInvalidArgs, err, "cannot read request body file %s", bodyPath)
	}
	return data, nil
}

func (c *CLI) loadRequestCredentials(profileName string, profile Profile, redactor *OutputRedactor) (map[string]string, error) {
	credentials := make(map[string]string)
	for _, credential := range requiredCredentials(profile) {
		value, err := c.CredentialStore.Get(credentialKey(profileName, credential))
		if err != nil {
			if codeOf(err) == CodeCredentialMissing {
				return nil, codedError(CodeCredentialMissing, "credential %s is missing for profile %s", credential, profileName)
			}
			return nil, err
		}
		credentials[credential] = value
		redactor.Register(value)
	}
	return credentials, nil
}

func appendRawQuery(rawQuery string, name string, value string) string {
	pair := url.QueryEscape(name) + "=" + url.QueryEscape(value)
	if rawQuery == "" {
		return pair
	}
	return rawQuery + "&" + pair
}

func applyDefaultHeaders(req *http.Request) {
	req.Header.Set("Accept", "application/json, */*;q=0.1")
	req.Header.Set("User-Agent", "api-cli/0.1")
}

func applyAuth(req *http.Request, target *url.URL, auth AuthConfig, credentials map[string]string, redactor *OutputRedactor) {
	switch auth.Scheme {
	case "bearer":
		value := "Bearer " + credentials[auth.Credential]
		redactor.Register(value)
		req.Header.Set("Authorization", value)
	case "header":
		value := credentials[auth.Credential]
		req.Header.Set(auth.Header, value)
	case "query":
		req.URL.RawQuery = appendRawQuery(stripExistingAuthQuery(req.URL.RawQuery, auth.Param), auth.Param, credentials[auth.Credential])
		target.RawQuery = req.URL.RawQuery
	case "basic":
		username := credentials[auth.UsernameCredential]
		password := credentials[auth.PasswordCredential]
		pair := username + ":" + password
		encoded := base64.StdEncoding.EncodeToString([]byte(pair))
		value := "Basic " + encoded
		redactor.Register(pair)
		redactor.Register(encoded)
		redactor.Register(value)
		req.Header.Set("Authorization", value)
	}
}

func stripExistingAuthQuery(rawQuery string, param string) string {
	// Query auth collision is checked before credentials are loaded. This helper
	// keeps applyAuth idempotent if future callers pass a target that already has
	// the appended credential.
	if rawQuery == "" {
		return rawQuery
	}
	needle := url.QueryEscape(param) + "="
	parts := strings.Split(rawQuery, "&")
	kept := parts[:0]
	for _, part := range parts {
		if !strings.HasPrefix(part, needle) {
			kept = append(kept, part)
		}
	}
	return strings.Join(kept, "&")
}

var errResponseTooLarge = errors.New("response body too large")

func readLimitedResponseBody(body io.Reader, maxBytes int64) ([]byte, error) {
	limited := io.LimitReader(body, maxBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, errResponseTooLarge
	}
	return data, nil
}

func buildResponseEnvelope(meta requestMeta, resp *http.Response, body []byte) map[string]any {
	envelope := map[string]any{
		"ok":      resp.StatusCode >= 200 && resp.StatusCode <= 299,
		"profile": meta.Profile,
		"method":  meta.Method,
		"path":    meta.Path,
		"status":  resp.StatusCode,
	}
	if contentType := resp.Header.Get("Content-Type"); contentType != "" {
		envelope["contentType"] = contentType
	}
	if headers := responseHeaders(resp.Header); len(headers) > 0 {
		envelope["headers"] = headers
	}
	addBodyFields(envelope, body)
	return envelope
}

func addBodyFields(envelope map[string]any, body []byte) {
	if len(body) == 0 {
		return
	}
	trimmed := bytes.TrimSpace(body)
	var parsed any
	if len(trimmed) > 0 && json.Unmarshal(trimmed, &parsed) == nil {
		envelope["body"] = parsed
		return
	}
	if utf8.Valid(body) {
		envelope["bodyText"] = string(body)
		return
	}
	envelope["bodyEncoding"] = "base64"
	envelope["bodyBase64"] = base64.StdEncoding.EncodeToString(body)
}

var allowedResponseHeaders = map[string]struct{}{
	"request-id":            {},
	"x-request-id":          {},
	"x-correlation-id":      {},
	"x-ratelimit-limit":     {},
	"x-ratelimit-remaining": {},
	"x-ratelimit-reset":     {},
	"retry-after":           {},
}

func responseHeaders(headers http.Header) map[string]string {
	out := make(map[string]string)
	for name, values := range headers {
		lower := strings.ToLower(name)
		if _, ok := allowedResponseHeaders[lower]; ok && len(values) > 0 {
			out[lower] = strings.Join(values, ",")
		}
	}
	return out
}

func (c *CLI) writeRequestError(err error, redactor *OutputRedactor, meta requestMeta) int {
	payload := errorPayload(err)
	if meta.Profile != "" {
		payload["profile"] = meta.Profile
	}
	if meta.Method != "" {
		payload["method"] = meta.Method
	}
	if meta.Path != "" {
		payload["path"] = meta.Path
	}
	if redactor != nil {
		writeRedactedJSON(c.Out, payload, redactor)
		return 1
	}
	writeJSON(c.Out, payload)
	return 1
}
