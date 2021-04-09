package main

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httputil"
	"os"
	"strings"
	"time"

	"github.com/frzifus/goblue"
)

func main() {
	var (
		cfg    goblue.Config
		logger = log.New(os.Stdout, "", 0)

		filename = "config.json"
	)

	logger.Println("read file:", filename)
	f, err := os.Open(filename)
	if err != nil {
		logger.Fatalln(err)
	}
	defer f.Close()

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(f); err != nil {
		logger.Fatalln(err)
	}

	if err := json.Unmarshal(buf.Bytes(), &cfg); err != nil {
		logger.Fatalln(err)
	}

	bl, err := goblue.NewClient(
		cfg,
		goblue.WithTransport(
			newTripper(
				log.New(os.Stdout, "bluelink-api:", 0),
				http.DefaultTransport,
			),
		),
		goblue.WithTimeout(2*time.Minute),
	)
	if err != nil {
		logger.Fatalln("creating client failed:", err)
	}

	if err := bl.Authenticate(); err != nil {
		logger.Fatalln("authentication failed:", err)
	}

	vs, err := bl.Vehicles()
	if err != nil {
		logger.Fatalln("fetching vehicles information failed:", err)
	}
	for _, v := range vs {
		logger.Println(v)
		logger.Println(v.Status())
	}
}

type logger interface {
	Println(v ...interface{})
}

type roundTripper struct {
	logger    logger
	transport http.RoundTripper
}

const max = 2048

// newTripper creates a logging roundtrip handler
// inspired by: github.com/andig/evcc/util/request/roundtrip.go
func newTripper(logger logger, transport http.RoundTripper) http.RoundTripper {
	tripper := &roundTripper{
		logger:    logger,
		transport: transport,
	}

	return tripper
}

func (r *roundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if body, err := httputil.DumpRequest(req, true); err == nil {
		s := strings.TrimSpace(string(body))
		if len(s) > max {
			s = s[:max]
		}
		r.logger.Println(s)
	}

	resp, err := r.transport.RoundTrip(req)

	if resp != nil {
		if body, err := httputil.DumpResponse(resp, true); err == nil {
			s := strings.TrimSpace(string(body))
			if len(s) > max {
				s = s[:max]
			}
			r.logger.Println(s)
		}
	}

	return resp, err
}
