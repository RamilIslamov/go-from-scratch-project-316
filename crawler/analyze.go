package crawler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

type Options struct {
	URL         string
	Depth       int
	Retries     int
	Delay       time.Duration
	Timeout     time.Duration
	UserAgent   string
	Concurrency int
	IndentJSON  bool
	HTTPClient  *http.Client
}

type Report struct {
	RootURL     string    `json:"root_url"`
	Depth       int       `json:"depth"`
	GeneratedAt time.Time `json:"generated_at"`
	Pages       []Page    `json:"pages"`
}

type Page struct {
	URL        string `json:"url"`
	Depth      int    `json:"depth"`
	HTTPStatus int    `json:"http_status"`
	Status     string `json:"status"`
	Error      string `json:"error"`
}

func Analyze(ctx context.Context, opts Options) ([]byte, error) {
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, opts.URL, nil)
	if err != nil {
		return nil, err
	}

	if opts.UserAgent != "" {
		req.Header.Set("User-Agent", opts.UserAgent)
	}

	page := Page{
		URL:   opts.URL,
		Depth: 0,
	}

	resp, err := client.Do(req)
	if err != nil {
		page.Status = "error"
		page.Error = err.Error()
	} else {
		defer resp.Body.Close()

		page.HTTPStatus = resp.StatusCode
		page.Status = "ok"
	}

	report := Report{
		RootURL:     opts.URL,
		Depth:       opts.Depth,
		GeneratedAt: time.Now(),
		Pages:       []Page{page},
	}

	if opts.IndentJSON {
		return json.MarshalIndent(report, "", "  ")
	}

	return json.Marshal(report)
}
