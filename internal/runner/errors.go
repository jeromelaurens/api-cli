package runner

import "fmt"

const (
	CodeInvalidArgs           = "invalid_args"
	CodeProfileCatalogMissing = "profile_catalog_missing"
	CodeProfileCatalogInvalid = "profile_catalog_invalid"
	CodeProfileNotFound       = "profile_not_found"
	CodeCredentialMissing     = "credential_missing"
	CodeKeyringUnavailable    = "keyring_unavailable"
	CodeInvalidRequestPath    = "invalid_request_path"
	CodeInvalidQuery          = "invalid_query"
	CodeForbiddenHeader       = "forbidden_header"
	CodeAuthQueryCollision    = "auth_query_collision"
	CodeProfileReadOnly       = "profile_read_only"
	CodeRequestTimeout        = "request_timeout"
	CodeResponseTooLarge      = "response_too_large"
	CodeHTTPError             = "http_error"
	CodeNetworkError          = "network_error"
	CodeInternalError         = "internal_error"
)

type Error struct {
	Code    string
	Message string
	Err     error
}

func codedError(code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

func wrapError(code string, err error, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...), Err: err}
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Err != nil {
		return e.Message + ": " + e.Err.Error()
	}
	return e.Message
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func codeOf(err error) string {
	if err == nil {
		return ""
	}
	if e, ok := err.(*Error); ok {
		return e.Code
	}
	return CodeInternalError
}
