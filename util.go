package goblue

import (
	"io"
	"net/http"
)

// newHttpRequest builds and executes HTTP request and returns the response
func newHttpRequest(method, uri string, data io.Reader, headers ...map[string]string) (*http.Request, error) {
	req, err := http.NewRequest(method, uri, data)
	if err == nil {
		for _, headers := range headers {
			for k, v := range headers {
				req.Header.Add(k, v)
			}
		}
	}

	return req, err
}
