package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/arthur-sommer-etc/cliproxyapi-copilot-plugin/internal/transport"
)

// These types match the v8 host wire protocol without depending on its SDK.
type quotaFetchRequest struct {
	AuthIndex      string            `json:"auth_index"`
	AuthID         string            `json:"auth_id"`
	Provider       string            `json:"provider"`
	StorageJSON    []byte            `json:"storage_json,omitempty"`
	Metadata       map[string]any    `json:"metadata,omitempty"`
	Attributes     map[string]string `json:"attributes,omitempty"`
	HostCallbackID string            `json:"host_callback_id,omitempty"`
}
type quotaSubscription struct {
	Plan string `json:"plan,omitempty"`
}
type quotaMetric struct {
	Key    string  `json:"key"`
	Label  string  `json:"label"`
	Value  float64 `json:"value"`
	Unit   string  `json:"unit,omitempty"`
	Format string  `json:"format,omitempty"`
}
type quotaBucket struct {
	Window            string   `json:"window,omitempty"`
	RemainingFraction *float64 `json:"remainingFraction,omitempty"`
	ResetTime         string   `json:"resetTime,omitempty"`
	Description       string   `json:"description,omitempty"`
}
type quotaGroup struct {
	DisplayName string        `json:"displayName,omitempty"`
	Buckets     []quotaBucket `json:"buckets,omitempty"`
}
type quotaFetchResponse struct {
	Subscription *quotaSubscription `json:"subscription,omitempty"`
	Summary      []quotaMetric      `json:"summary,omitempty"`
	Groups       []quotaGroup       `json:"groups,omitempty"`
}
type quotaHTTPDoer interface {
	Do(context.Context, string, transport.Request) (transport.Response, error)
}
type githubQuotaSnapshot struct {
	Remaining        *float64 `json:"remaining"`
	PercentRemaining *float64 `json:"percent_remaining"`
	Unlimited        bool     `json:"unlimited"`
	HasQuota         *bool    `json:"has_quota"`
}
type githubQuotaResponse struct {
	Plan         string                         `json:"copilot_plan"`
	ResetDate    string                         `json:"quota_reset_date"`
	ResetDateUTC string                         `json:"quota_reset_date_utc"`
	Snapshots    map[string]githubQuotaSnapshot `json:"quota_snapshots"`
}

// fetchQuota uses the specific credential supplied by the host, never global state.
func fetchQuota(ctx context.Context, req quotaFetchRequest, client quotaHTTPDoer) (quotaFetchResponse, error) {
	var out quotaFetchResponse
	var token, storageLogin string
	if req.Provider != "" && !strings.EqualFold(strings.TrimSpace(req.Provider), "copilot") {
		return out, errors.New("copilot quota: invalid credential identity")
	}
	if len(req.StorageJSON) != 0 {
		if !validQuotaStorageIdentity(req.StorageJSON) {
			return out, errors.New("copilot quota: invalid credential storage")
		}
		var storage struct {
			GitHubAccessToken string `json:"github_access_token"`
			GitHubLogin       string `json:"github_login"`
			Type              string `json:"type"`
		}
		if err := json.Unmarshal(req.StorageJSON, &storage); err == nil {
			token = strings.TrimSpace(storage.GitHubAccessToken)
			storageLogin = strings.TrimSpace(storage.GitHubLogin)
			if storage.Type != "" && !strings.EqualFold(strings.TrimSpace(storage.Type), "copilot") {
				return out, errors.New("copilot quota: invalid credential identity")
			}
		} else {
			// Invalid storage must not silently select another credential.
			return out, errors.New("copilot quota: invalid credential storage")
		}
	}
	for _, field := range []string{"type", "github_login", "github_access_token"} {
		if value := req.Metadata[field]; value != nil {
			if _, ok := value.(string); !ok {
				return out, errors.New("copilot quota: invalid credential identity")
			}
		}
	}
	metadataType, _ := req.Metadata["type"].(string)
	if metadataType != "" && !strings.EqualFold(strings.TrimSpace(metadataType), "copilot") {
		return out, errors.New("copilot quota: invalid credential identity")
	}
	metadataLogin, _ := req.Metadata["github_login"].(string)
	if storageLogin != "" && strings.TrimSpace(metadataLogin) != "" && !strings.EqualFold(storageLogin, strings.TrimSpace(metadataLogin)) {
		return out, errors.New("copilot quota: conflicting credential identities")
	}
	metadataToken, _ := req.Metadata["github_access_token"].(string)
	metadataToken = strings.TrimSpace(metadataToken)
	if token != "" && metadataToken != "" && token != metadataToken {
		return out, errors.New("copilot quota: conflicting GitHub access tokens")
	}
	if token == "" {
		token = metadataToken
	}
	if token == "" {
		return out, errors.New("copilot quota: missing GitHub access token")
	}
	if client == nil {
		return out, errors.New("copilot quota: HTTP transport unavailable")
	}
	headers := make(http.Header)
	headers.Set("Authorization", "token "+strings.TrimSpace(token))
	headers.Set("Accept", "application/json")
	headers.Set("User-Agent", "cliproxyapi-copilot")
	resp, err := client.Do(ctx, req.HostCallbackID, transport.Request{Method: http.MethodGet, URL: "https://api.github.com/copilot_internal/user", Headers: headers})
	if err != nil {
		return out, errors.New("copilot quota: upstream request failed")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return out, fmt.Errorf("copilot quota: upstream returned HTTP %d", resp.StatusCode)
	}
	var upstream githubQuotaResponse
	if err = json.Unmarshal(resp.Body, &upstream); err != nil {
		return out, errors.New("copilot quota: invalid upstream response")
	}
	if upstream.Plan != "" {
		out.Subscription = &quotaSubscription{Plan: upstream.Plan}
	}
	resetDate := upstream.ResetDateUTC
	if resetDate == "" {
		resetDate = upstream.ResetDate
	}
	keys := make([]string, 0, len(upstream.Snapshots))
	for k := range upstream.Snapshots {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		snapshot := upstream.Snapshots[key]
		label := strings.ReplaceAll(key, "_", " ")
		if snapshot.Unlimited || (snapshot.HasQuota != nil && !*snapshot.HasQuota) {
			description := "No quota"
			if snapshot.Unlimited {
				description = "Unlimited"
			}
			// The host ABI uses a non-pointer float64 and re-encodes missing
			// fractions as zero. Never emit a bucket without a known fraction.
			out.Groups = append(out.Groups, quotaGroup{DisplayName: label + " (" + description + ")"})
			continue
		}
		if snapshot.Remaining != nil {
			if *snapshot.Remaining < 0 {
				return quotaFetchResponse{}, errors.New("copilot quota: invalid upstream remaining")
			}
			out.Summary = append(out.Summary, quotaMetric{Key: key + "_remaining", Label: label + " remaining", Value: *snapshot.Remaining, Unit: "requests", Format: "number"})
		}
		// An absent percentage is unknown, not exhausted. Do not fabricate a bucket.
		if snapshot.PercentRemaining != nil {
			if *snapshot.PercentRemaining < 0 || *snapshot.PercentRemaining > 100 {
				return quotaFetchResponse{}, errors.New("copilot quota: invalid upstream percentage")
			}
			fraction := *snapshot.PercentRemaining / 100
			window := ""
			if key == "premium_interactions" {
				// GitHub documents premium-request allowances as monthly.
				window = "monthly"
			}
			out.Groups = append(out.Groups, quotaGroup{DisplayName: label, Buckets: []quotaBucket{{Window: window, RemainingFraction: &fraction, ResetTime: resetDate}}})
		}
	}
	// A plan is metadata, not evidence of a supported quota response.
	if len(out.Summary) == 0 && len(out.Groups) == 0 {
		return quotaFetchResponse{}, errors.New("copilot quota: empty upstream response")
	}
	return out, nil
}
