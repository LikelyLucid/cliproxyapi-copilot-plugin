package main

import (
	"context"
	"net/http"
	"testing"

	"github.com/arthur-sommer-etc/cliproxyapi-copilot-plugin/internal/transport"
)

func TestGenericTransportQuotaURLNeverUsesDirectHTTP(t *testing.T) {
	original := http.DefaultTransport
	direct := false
	http.DefaultTransport = quotaRoundTripFunc(func(*http.Request) (*http.Response, error) {
		direct = true
		t.Fatal("generic request bypassed host callback")
		return nil, nil
	})
	defer func() { http.DefaultTransport = original }()
	// No host callback is configured in this test process; a callback error is expected.
	_, err := (hostTransport{}).Do(context.Background(), "synthetic-callback", transport.Request{Method: http.MethodGet, URL: "https://api.github.com/copilot_internal/user", Body: []byte("synthetic-body")})
	if direct || err == nil {
		t.Fatalf("direct=%v err=%v; expected host callback failure", direct, err)
	}
}
