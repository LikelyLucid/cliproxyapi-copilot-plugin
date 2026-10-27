package main

import (
	"encoding/json"
	"testing"
)

func TestQuotaRealShapedResponseOmitsUnavailableBuckets(t *testing.T) {
	const body = `{"copilot_plan":"individual","quota_reset_date":"2030-01-01","quota_reset_date_utc":"2030-01-01T00:00:00Z","quota_snapshots":{"chat":{"entitlement":0,"remaining":0,"percent_remaining":100,"unlimited":true},"completions":{"entitlement":0,"remaining":0,"percent_remaining":0,"unlimited":true},"premium_interactions":{"entitlement":300,"remaining":87,"percent_remaining":29,"unlimited":false},"unknown":{"remaining":0},"no_quota":{"has_quota":false,"percent_remaining":0}}}`
	out, err := quotaFromBody(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Groups) != 4 || out.Groups[3].DisplayName != "premium interactions" || len(out.Groups[3].Buckets) != 1 {
		t.Fatalf("unexpected groups: %+v", out.Groups)
	}
	for _, g := range out.Groups[:3] {
		if len(g.Buckets) != 0 {
			t.Fatalf("unavailable quota has a bucket: %+v", g)
		}
	}
	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	groups := decoded["groups"].([]any)
	bucket := groups[3].(map[string]any)["buckets"].([]any)[0].(map[string]any)
	if bucket["remainingFraction"] != 0.29 || bucket["resetTime"] != "2030-01-01T00:00:00Z" {
		t.Fatalf("incorrect bucket: %s", raw)
	}
}
