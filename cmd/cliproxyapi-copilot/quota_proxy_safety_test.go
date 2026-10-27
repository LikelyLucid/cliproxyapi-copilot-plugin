package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestQuotaDispatchFailsClosedWhenProxyPolicyUnknown(t *testing.T) {
	for _, proxy := range []string{"", "http://proxy.example:8080", "socks5://proxy.example:1080"} {
		t.Run(proxy, func(t *testing.T) {
			original := http.DefaultTransport
			calls := 0
			http.DefaultTransport = quotaRoundTripFunc(func(*http.Request) (*http.Response, error) {
				calls++
				t.Fatal("quota request escaped without verified host proxy policy")
				return nil, nil
			})
			defer func() { http.DefaultTransport = original }()
			req, err := json.Marshal(quotaFetchRequest{Metadata: map[string]any{"github_access_token": "synthetic", "proxy_url": proxy}, HostCallbackID: "synthetic"})
			if err != nil {
				t.Fatal(err)
			}
			_, err = dispatch("quota.fetch", req)
			if err == nil || !strings.Contains(err.Error(), "proxy") {
				t.Fatalf("expected proxy policy error, got %v", err)
			}
			if calls != 0 {
				t.Fatalf("unexpected outbound calls: %d", calls)
			}
		})
	}
}
