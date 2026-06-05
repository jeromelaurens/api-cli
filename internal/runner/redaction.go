package runner

import (
	"encoding/json"
	"io"
	"strings"
)

type OutputRedactor struct {
	values []string
}

func NewOutputRedactor(values ...string) *OutputRedactor {
	r := &OutputRedactor{}
	for _, value := range values {
		r.Register(value)
	}
	return r
}

func (r *OutputRedactor) Register(value string) {
	if r == nil || value == "" {
		return
	}
	r.values = append(r.values, value)
}

func (r *OutputRedactor) Redact(text string) string {
	if r == nil {
		return text
	}
	for _, value := range r.values {
		text = strings.ReplaceAll(text, value, "[REDACTED]")
	}
	return text
}

func writeRedactedJSON(out io.Writer, payload any, redactor *OutputRedactor) {
	data, err := json.Marshal(payload)
	if err != nil {
		data = []byte(`{"ok":false,"error":{"code":"internal_error","message":"cannot encode JSON"}}`)
	}
	_, _ = out.Write([]byte(redactor.Redact(string(data))))
	_, _ = out.Write([]byte("\n"))
}
