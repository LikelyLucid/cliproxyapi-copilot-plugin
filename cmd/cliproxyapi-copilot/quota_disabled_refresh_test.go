package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

func TestQuotaHostStateFailClosed(t *testing.T) {
	valid := pluginapi.HostAuthFileEntry{ID: "credential", AuthIndex: "index", Provider: "copilot"}
	for _, tc := range []struct {
		name      string
		entries   []pluginapi.HostAuthFileEntry
		listErr   error
		index, id string
	}{
		{"missing", nil, nil, "index", "credential"},
		{"list error", nil, errors.New("sensitive host error"), "index", "credential"},
		{"no index", []pluginapi.HostAuthFileEntry{valid}, nil, "", "credential"},
		{"wrong provider", []pluginapi.HostAuthFileEntry{{ID: "credential", AuthIndex: "index", Provider: "other"}}, nil, "index", "credential"},
		{"wrong id", []pluginapi.HostAuthFileEntry{valid}, nil, "index", "different"},
		{"duplicate", []pluginapi.HostAuthFileEntry{valid, valid}, nil, "index", "credential"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			client := quotaTestTransport{quotaRoundTripFunc(func(*http.Request) (*http.Response, error) { calls++; return nil, errors.New("unexpected") })}
			list := func(context.Context) ([]pluginapi.HostAuthFileEntry, error) { return tc.entries, tc.listErr }
			_, err := fetchQuotaWithHostState(context.Background(), quotaFetchRequest{AuthID: tc.id, AuthIndex: tc.index, StorageJSON: json.RawMessage(`{"github_access_token":"synthetic"}`)}, client, list)
			if err == nil || calls != 0 {
				t.Fatalf("err=%v, calls=%d", err, calls)
			}
		})
	}
}

func TestQuotaEnabledCredentialReachesUpstream(t *testing.T) {
	calls := 0
	client := quotaTestTransport{quotaRoundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return nil, errors.New("synthetic upstream failure")
	})}
	list := func(context.Context) ([]pluginapi.HostAuthFileEntry, error) {
		return []pluginapi.HostAuthFileEntry{{ID: "credential", AuthIndex: "index", Provider: "copilot"}}, nil
	}
	_, err := fetchQuotaWithHostState(context.Background(), quotaFetchRequest{AuthID: "credential", AuthIndex: "index", StorageJSON: json.RawMessage(`{"github_access_token":"synthetic"}`)}, client, list)
	if err == nil || calls != 1 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
}

func TestQuotaDisabledCredentialNeverReachesUpstream(t *testing.T) {
	calls := 0
	client := quotaTestTransport{quotaRoundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return nil, errors.New("unexpected upstream call")
	})}
	list := func(context.Context) ([]pluginapi.HostAuthFileEntry, error) {
		return []pluginapi.HostAuthFileEntry{{ID: "credential", AuthIndex: "index", Provider: "copilot", Disabled: true}}, nil
	}
	_, err := fetchQuotaWithHostState(context.Background(), quotaFetchRequest{AuthID: "credential", AuthIndex: "index", StorageJSON: json.RawMessage(`{"github_access_token":"synthetic"}`)}, client, list)
	if err == nil || calls != 0 {
		t.Fatalf("disabled credential: err=%v upstream calls=%d", err, calls)
	}
}
