package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/arthur-sommer-etc/cliproxyapi-copilot-plugin/internal/transport"
)

func TestQuotaDirectTransportSafety(t *testing.T) {
	d := newDirectQuotaTransport()
	tr := d.client.Transport.(*http.Transport)
	if tr.Proxy != nil {
		t.Fatal("environment proxy must be disabled")
	}
	if d.client.Timeout <= 0 || d.client.Timeout > 30*time.Second {
		t.Fatal("missing bounded timeout")
	}
	for _, u := range []string{"http://api.github.com/copilot_internal/user", "https://evil.example/copilot_internal/user", "https://api.github.com:443/copilot_internal/user", "https://api.github.com/copilot_internal/user?x=1"} {
		if _, err := d.Do(context.Background(), "", transport.Request{Method: "GET", URL: u}); err == nil {
			t.Fatalf("accepted %q", u)
		}
	}
}

func TestQuotaDirectResponseAndRedirect(t *testing.T) {
	proxyHits := 0
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { proxyHits++; w.WriteHeader(500) }))
	defer proxy.Close()
	t.Setenv("HTTPS_PROXY", proxy.URL)
	t.Setenv("NO_PROXY", "")
	var hits int
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		switch r.Header.Get("X-Test") {
		case "redirect":
			http.Redirect(w, r, "https://example.com/", http.StatusFound)
		case "large":
			_, _ = io.WriteString(w, strings.Repeat("x", quotaMaxResponseBytes+1))
		default:
			_, _ = io.WriteString(w, "ok")
		}
	}))
	defer s.Close()
	d := newDirectQuotaTransport()
	tr := d.client.Transport.(*http.Transport)
	tr.TLSClientConfig = s.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
	tr.TLSClientConfig.ServerName = "127.0.0.1"
	tr.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, s.Listener.Addr().String())
	}
	req := transport.Request{Method: "GET", URL: "https://api.github.com/copilot_internal/user", Headers: http.Header{}}
	res, err := d.Do(context.Background(), "", req)
	if err != nil || string(res.Body) != "ok" {
		t.Fatalf("response %q error %v", res.Body, err)
	}
	if proxyHits != 0 {
		t.Fatalf("environment proxy received %d requests", proxyHits)
	}
	req.Headers.Set("X-Test", "redirect")
	res, err = d.Do(context.Background(), "", req)
	if err != nil || res.StatusCode != 302 || hits != 2 {
		t.Fatalf("redirect followed or failed: %d %v hits %d", res.StatusCode, err, hits)
	}
	req.Headers.Set("X-Test", "large")
	if _, err = d.Do(context.Background(), "", req); err == nil {
		t.Fatal("accepted oversized response")
	}
}
