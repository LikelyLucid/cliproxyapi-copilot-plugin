package main

import (
	"context"
	"testing"
)

func TestQuotaRejectsInvalidStoredCredentialType(t *testing.T) {
	for _, storage := range []string{
		`{"github_access_token":123}`,
		`{"github_access_token":"stored-secret","github_access_token":false}`,
		`[]`,
	} {
		_, err := fetchQuota(context.Background(), quotaFetchRequest{
			StorageJSON: []byte(storage), Metadata: map[string]any{"github_access_token": "metadata-secret"},
		}, nil)
		if err == nil || err.Error() != "copilot quota: invalid credential storage" {
			t.Errorf("storage %s: expected invalid credential storage, got %v", storage, err)
		}
	}
}

func TestQuotaRejectsNegativeRemaining(t *testing.T) {
	for _, body := range []string{
		`{"quota_snapshots":{"chat":{"remaining":-1}}}`,
		`{"copilot_plan":"individual","quota_snapshots":{"chat":{"remaining":-0.01,"percent_remaining":50}}}`,
	} {
		if _, err := quotaFromBody(body); err == nil {
			t.Errorf("accepted negative remaining in %s", body)
		}
	}
}
