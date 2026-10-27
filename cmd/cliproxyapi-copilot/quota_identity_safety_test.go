package main

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

func TestQuotaRejectsConflictingCredentialIdentities(t *testing.T) {
	for _, tc := range []struct {
		name, storage string
		metadata      map[string]any
	}{
		{"different logins", `{"github_access_token":"synthetic","github_login":"alice"}`, map[string]any{"github_login": "bob"}},
		{"wrong storage type", `{"type":"other","github_access_token":"synthetic"}`, nil},
		{"wrong metadata type", `{"github_access_token":"synthetic"}`, map[string]any{"type": "other"}},
		{"invalid metadata token", `{"github_access_token":"synthetic"}`, map[string]any{"github_access_token": 123}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			client := quotaTestTransport{quotaRoundTripFunc(func(*http.Request) (*http.Response, error) { called = true; return nil, errors.New("unexpected call") })}
			_, err := fetchQuota(context.Background(), quotaFetchRequest{StorageJSON: []byte(tc.storage), Metadata: tc.metadata}, client)
			if err == nil || called {
				t.Fatalf("invalid identity must fail before network request: err=%v called=%v", err, called)
			}
		})
	}
}
