package main

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func quotaFromBody(body string) (quotaFetchResponse, error) {
	rt := quotaRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	return fetchQuota(context.Background(), quotaFetchRequest{Metadata: map[string]any{"github_access_token": "synthetic"}}, quotaTestTransport{rt})
}

func TestQuotaRejectsEmptyResponses(t *testing.T) {
	for _, body := range []string{`{}`, `null`, `{"quota_snapshots":{}}`, `{"quota_snapshots":{"chat":{}}}`} {
		_, err := quotaFromBody(body)
		if err == nil {
			t.Fatalf("accepted empty quota response %s", body)
		}
	}
}

func TestQuotaRejectsOutOfRangePercentages(t *testing.T) {
	for _, percent := range []string{"-0.01", "100.01"} {
		_, err := quotaFromBody(`{"quota_snapshots":{"chat":{"percent_remaining":` + percent + `}}}`)
		if err == nil {
			t.Fatalf("accepted invalid percent %s", percent)
		}
	}
}

func TestQuotaUnlimitedDoesNotAppearExhausted(t *testing.T) {
	for _, body := range []string{`{"quota_snapshots":{"chat":{"unlimited":true,"remaining":0,"percent_remaining":0}}}`, `{"quota_snapshots":{"chat":{"has_quota":false,"remaining":0,"percent_remaining":0}}}`} {
		out, err := quotaFromBody(body)
		if err != nil {
			t.Fatal(err)
		}
		if len(out.Summary) != 0 {
			t.Fatalf("unlimited has misleading remaining: %+v", out)
		}
		for _, group := range out.Groups {
			for _, bucket := range group.Buckets {
				if bucket.RemainingFraction != nil {
					t.Fatalf("unlimited has misleading fraction: %+v", out)
				}
			}
		}
	}
}
