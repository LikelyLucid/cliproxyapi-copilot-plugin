//go:build quota_host_abi

package main

import (
	"encoding/json"
	"testing"

	hostapi "github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

// This opt-in test imports the actual v8 host SDK. Run with a temporary modfile
// that requires v8 and replaces it with the host checkout being tested.
func TestQuotaActualHostABIRoundTrip(t *testing.T) {
	cases := []struct {
		name, body string
		buckets    int
		fraction   float64
	}{
		{"unknown", `{"quota_snapshots":{"chat":{"remaining":0}}}`, 0, 0},
		{"unlimited", `{"quota_snapshots":{"chat":{"unlimited":true,"percent_remaining":0}}}`, 0, 0},
		{"no quota", `{"quota_snapshots":{"chat":{"has_quota":false,"percent_remaining":0}}}`, 0, 0},
		{"zero", `{"quota_snapshots":{"chat":{"percent_remaining":0}}}`, 1, 0},
		{"positive", `{"quota_snapshots":{"chat":{"percent_remaining":29}}}`, 1, 0.29},
		{"real-shaped mixed", `{"copilot_plan":"individual","quota_reset_date_utc":"2030-01-01T00:00:00Z","quota_snapshots":{"chat":{"entitlement":0,"remaining":0,"percent_remaining":100,"unlimited":true},"completions":{"remaining":0,"percent_remaining":0,"unlimited":true},"premium_interactions":{"entitlement":300,"remaining":87,"percent_remaining":29},"unknown":{"remaining":0}}}`, 1, 0.29},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := quotaFromBody(tc.body)
			if err != nil {
				t.Fatal(err)
			}
			wire, err := json.Marshal(out)
			if err != nil {
				t.Fatal(err)
			}
			var host hostapi.QuotaFetchResponse
			if err := json.Unmarshal(wire, &host); err != nil {
				t.Fatal(err)
			}
			wire, err = json.Marshal(host)
			if err != nil {
				t.Fatal(err)
			}
			var decoded struct {
				Groups []struct {
					Buckets []struct {
						RemainingFraction *float64 `json:"remainingFraction"`
					} `json:"buckets"`
				} `json:"groups"`
			}
			if err := json.Unmarshal(wire, &decoded); err != nil {
				t.Fatal(err)
			}
			var fractions []*float64
			for _, group := range decoded.Groups {
				for _, bucket := range group.Buckets {
					fractions = append(fractions, bucket.RemainingFraction)
				}
			}
			if len(fractions) != tc.buckets {
				t.Fatalf("host roundtrip fabricated a bucket: %s", wire)
			}
			if tc.buckets != 0 && (fractions[0] == nil || *fractions[0] != tc.fraction) {
				t.Fatalf("fraction not preserved: %s", wire)
			}
		})
	}
}
