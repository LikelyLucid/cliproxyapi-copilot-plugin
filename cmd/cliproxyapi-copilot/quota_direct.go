package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/arthur-sommer-etc/cliproxyapi-copilot-plugin/internal/transport"
)

const quotaMaxResponseBytes = 1 << 20
const githubQuotaURL = "https://api.github.com/copilot_internal/user"

type directQuotaTransport struct{ client *http.Client }

func newDirectQuotaTransport() directQuotaTransport {
	return directQuotaTransport{client: &http.Client{
		Transport:     &http.Transport{Proxy: nil, DisableKeepAlives: true, MaxResponseHeaderBytes: 64 << 10},
		Timeout:       15 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

func (d directQuotaTransport) Do(ctx context.Context, _ string, req transport.Request) (transport.Response, error) {
	if req.Method != http.MethodGet || req.URL != githubQuotaURL || len(req.Body) != 0 {
		return transport.Response{}, errors.New("copilot quota: invalid direct request")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, githubQuotaURL, nil)
	if err != nil {
		return transport.Response{}, errors.New("copilot quota: invalid direct request")
	}
	request.Header = req.Headers.Clone()
	response, err := d.client.Do(request)
	if err != nil {
		return transport.Response{}, errors.New("copilot quota: direct request failed")
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, quotaMaxResponseBytes+1))
	if err != nil || len(body) > quotaMaxResponseBytes {
		return transport.Response{}, errors.New("copilot quota: invalid or oversized direct response")
	}
	return transport.Response{StatusCode: response.StatusCode, Headers: response.Header, Body: body}, nil
}
