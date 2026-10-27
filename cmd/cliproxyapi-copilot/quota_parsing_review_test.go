package main

import (
	"encoding/json"
	"testing"
)

func TestQuotaRejectsPlanWithoutUsableQuota(t *testing.T) {
	for _, body := range []string{
		`{"copilot_plan":"individual"}`,
		`{"copilot_plan":"individual","quota_snapshots":{}}`,
		`{"copilot_plan":"individual","quota_snapshots":{"chat":{}}}`,
		`{"copilot_plan":"individual","quota_snapshots":{"chat":null}}`,
		`{"copilot_plan":"individual","quota":{"remaining":20}}`,
	} {
		if _, err := quotaFromBody(body); err == nil {
			t.Errorf("accepted unsupported response %s", body)
		}
	}
}

func TestQuotaPreservesFractionalValuesWithoutInventingPeriod(t *testing.T) {
	out, err := quotaFromBody(`{"copilot_plan":"individual","quota_snapshots":{"premium_interactions":{"remaining":12.75,"percent_remaining":29.5}}}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Summary) != 1 || out.Summary[0].Value != 12.75 {
		t.Fatalf("remaining not preserved: %+v", out)
	}
	if len(out.Groups) != 1 || len(out.Groups[0].Buckets) != 1 {
		t.Fatalf("unexpected groups: %+v", out.Groups)
	}
	bucket := out.Groups[0].Buckets[0]
	if bucket.RemainingFraction == nil || *bucket.RemainingFraction != 0.295 {
		t.Fatalf("percentage not preserved: %+v", bucket)
	}
	if bucket.Window != "" || bucket.ResetTime != "" {
		t.Fatalf("invented period: %+v", bucket)
	}
}

func TestQuotaUnknownAndMalformedNeverBecomeZero(t *testing.T) {
	for _, body := range []string{
		`{"quota_snapshots":{"chat":{"remaining":"unknown"}}}`,
		`{"quota_snapshots":{"chat":{"percent_remaining":"unknown"}}}`,
		`{"quota_snapshots":{"chat":{"remaining":-1}}}`,
		`{"quota_snapshots":{"chat":{"percent_remaining":101}}}`,
	} {
		if _, err := quotaFromBody(body); err == nil {
			t.Errorf("accepted malformed quota %s", body)
		}
	}
	out, err := quotaFromBody(`{"quota_snapshots":{"chat":{"remaining":2.5,"percent_remaining":null}}}`)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Groups) != 0 {
		t.Fatalf("unknown percentage became bucket: %s", raw)
	}
}
