package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/arthur-sommer-etc/cliproxyapi-copilot-plugin/internal/transport"
)

type quotaRoundTripFunc func(*http.Request) (*http.Response, error)

func (f quotaRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type quotaTestTransport struct{ rt http.RoundTripper }

func (h quotaTestTransport) Do(ctx context.Context, callback string, req transport.Request) (transport.Response, error) {
	r, err := http.NewRequestWithContext(ctx, req.Method, req.URL, nil)
	if err != nil {
		return transport.Response{}, err
	}
	r.Header = req.Headers
	resp, err := h.rt.RoundTrip(r)
	if err != nil {
		return transport.Response{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	return transport.Response{StatusCode: resp.StatusCode, Body: body}, err
}
func TestFetchQuotaSelectedCredential(t *testing.T) {
	rt := quotaRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != "https://api.github.com/copilot_internal/user" || r.Method != "GET" || r.Header.Get("Authorization") != "token synthetic-beta" {
			t.Fatalf("unexpected request %s %s %q", r.Method, r.URL, r.Header.Get("Authorization"))
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"copilot_plan":"individual","quota_reset_date":"2030-01-01","quota_snapshots":{"premium_interactions":{"entitlement":300,"remaining":87,"percent_remaining":29}}}`))}, nil
	})
	got, err := fetchQuota(context.Background(), quotaFetchRequest{AuthIndex: "beta", Metadata: map[string]any{"github_access_token": "synthetic-beta"}, HostCallbackID: "callback-beta"}, quotaTestTransport{rt})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Groups) != 1 || len(got.Groups[0].Buckets) != 1 || got.Groups[0].Buckets[0].Window != "monthly" {
		t.Fatalf("premium quota must have a monthly window: %+v", got.Groups)
	}
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err = json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"plan":"individual"`) || !strings.Contains(string(raw), `"remainingFraction":0.29`) || !strings.Contains(string(raw), `87`) || !strings.Contains(string(raw), `"resetTime":"2030-01-01"`) {
		t.Fatalf("bad mapping: %s", raw)
	}
}

func TestFetchQuotaResetDatePreference(t *testing.T) {
	for _, tc := range []struct {
		name, fields, want string
	}{
		{"utc only", `"quota_reset_date_utc":"2030-01-01T00:00:00Z"`, "2030-01-01T00:00:00Z"},
		{"both", `"quota_reset_date":"2030-01-01","quota_reset_date_utc":"2030-01-01T12:00:00Z"`, "2030-01-01T12:00:00Z"},
		{"legacy only", `"quota_reset_date":"2030-01-01"`, "2030-01-01"},
		{"empty utc", `"quota_reset_date":"2030-01-01","quota_reset_date_utc":""`, "2030-01-01"},
		{"missing", `"copilot_plan":"individual"`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := `{` + tc.fields + `,"quota_snapshots":{"chat":{"percent_remaining":50}}}`
			rt := quotaRoundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
			})
			out, err := fetchQuota(context.Background(), quotaFetchRequest{Metadata: map[string]any{"github_access_token": "test"}}, quotaTestTransport{rt})
			if err != nil {
				t.Fatal(err)
			}
			if len(out.Groups) != 1 || len(out.Groups[0].Buckets) != 1 {
				t.Fatalf("unexpected groups: %+v", out.Groups)
			}
			if got := out.Groups[0].Buckets[0].ResetTime; got != tc.want {
				t.Fatalf("reset time = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFetchQuotaUnknownAndZero(t *testing.T) {
	for _, tc := range []struct {
		body    string
		present bool
	}{
		{`{"quota_snapshots":{"chat":{"remaining":0}}}`, false},
		{`{"quota_snapshots":{"chat":{"percent_remaining":0}}}`, true},
	} {
		rt := quotaRoundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
		})
		out, err := fetchQuota(context.Background(), quotaFetchRequest{Metadata: map[string]any{"github_access_token": "test"}}, quotaTestTransport{rt})
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(out)
		s := string(raw)
		if strings.Contains(s, `"subscription"`) || strings.Contains(s, `"window"`) || strings.Contains(s, `"resetTime"`) {
			t.Fatalf("invented fields: %s", s)
		}
		if strings.Contains(s, `"remainingFraction":0`) != tc.present {
			t.Fatalf("unknown/zero mapping: %s", s)
		}
	}
}
func TestFetchQuotaSanitizesErrors(t *testing.T) {
	secret := "synthetic-secret"
	for _, tc := range []struct {
		status int
		body   string
		err    error
	}{
		{401, secret, nil}, {200, secret, nil}, {0, "", fmt.Errorf("failure %s", secret)},
	} {
		rt := quotaRoundTripFunc(func(*http.Request) (*http.Response, error) {
			if tc.err != nil {
				return nil, tc.err
			}
			return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
		})
		_, err := fetchQuota(context.Background(), quotaFetchRequest{Metadata: map[string]any{"github_access_token": secret}}, quotaTestTransport{rt})
		if err == nil || strings.Contains(err.Error(), secret) {
			t.Fatalf("unsafe error: %v", err)
		}
	}
	_, err := fetchQuota(context.Background(), quotaFetchRequest{}, quotaTestTransport{quotaRoundTripFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("should not call") })})
	if err == nil {
		t.Fatal("missing token accepted")
	}
}
func TestFetchQuotaUsesCredentialStorageWhenMetadataLacksToken(t *testing.T) {
	rt := quotaRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Authorization") != "token synthetic-storage" {
			t.Fatal("wrong credential")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"copilot_plan":"individual","quota_snapshots":{"chat":{"remaining":1}}}`))}, nil
	})
	_, err := fetchQuota(context.Background(), quotaFetchRequest{Metadata: map[string]any{"github_login": "alice"}, StorageJSON: []byte(`{"github_access_token":"synthetic-storage"}`)}, quotaTestTransport{rt})
	if err != nil {
		t.Fatal(err)
	}
}

func TestFetchQuotaSeparateCredentials(t *testing.T) {
	for _, token := range []string{"synthetic-alpha", "synthetic-beta"} {
		rt := quotaRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.Header.Get("Authorization") != "token "+token {
				t.Fatal("wrong credential")
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"copilot_plan":"individual","quota_snapshots":{"chat":{"remaining":1}}}`))}, nil
		})
		_, err := fetchQuota(context.Background(), quotaFetchRequest{AuthIndex: token, Metadata: map[string]any{"github_access_token": token}}, quotaTestTransport{rt})
		if err != nil {
			t.Fatal(err)
		}
	}
}
