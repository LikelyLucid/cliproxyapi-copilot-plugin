package main

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestFetchQuotaCredentialPrecedence(t *testing.T) {
	for _, tc := range []struct {
		name, storage string
		metadata      any
		want          string
	}{
		{"conflicting tokens rejected", `{"github_access_token":"synthetic-storage"}`, "synthetic-metadata", ""},
		{"matching tokens accepted", `{"github_access_token":" synthetic-token "}`, "synthetic-token", "synthetic-token"},
		{"storage only", `{"github_access_token":"synthetic-storage"}`, nil, "synthetic-storage"},
		{"metadata only", "", "synthetic-metadata", "synthetic-metadata"},
		{"blank storage uses metadata", `{"github_access_token":"  "}`, "synthetic-metadata", "synthetic-metadata"},
		{"malformed storage rejected", `{`, "synthetic-metadata", ""},
		{"neither", `{"github_access_token":"  "}`, "  ", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			rt := quotaRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				called = true
				if r.Header.Get("Authorization") != "token "+tc.want {
					t.Fatal("wrong credential selected")
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"copilot_plan":"individual","quota_snapshots":{"chat":{"remaining":1}}}`))}, nil
			})
			_, err := fetchQuota(context.Background(), quotaFetchRequest{StorageJSON: []byte(tc.storage), Metadata: map[string]any{"github_access_token": tc.metadata}}, quotaTestTransport{rt})
			if tc.want == "" {
				if err == nil || called {
					t.Fatal("invalid credentials should fail without a request")
				}
				if tc.name == "conflicting tokens rejected" && err.Error() != "copilot quota: conflicting GitHub access tokens" {
					t.Fatalf("expected sanitized conflict error, got %q", err)
				}
			} else if err != nil || !called {
				t.Fatalf("request failed: %v", err)
			}
		})
	}
}
