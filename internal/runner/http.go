package runner

import "net/http"

type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

func defaultHTTPClient() HTTPDoer {
	return &http.Client{}
}
