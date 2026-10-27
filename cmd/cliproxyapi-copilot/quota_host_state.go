package main

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

// hostQuotaAuthList returns the host's current snapshot, not quota request metadata.
func hostQuotaAuthList(_ context.Context) ([]pluginapi.HostAuthFileEntry, error) {
	raw, err := callHost(pluginabi.MethodHostAuthList, struct{}{})
	if err != nil {
		return nil, errors.New("copilot quota: cannot verify host credential state")
	}
	var result struct {
		Files []pluginapi.HostAuthFileEntry `json:"files"`
	}
	if json.Unmarshal(raw, &result) != nil {
		return nil, errors.New("copilot quota: cannot verify host credential state")
	}
	return result.Files, nil
}

func fetchQuotaWithHostState(ctx context.Context, req quotaFetchRequest, client quotaHTTPDoer, list func(context.Context) ([]pluginapi.HostAuthFileEntry, error)) (quotaFetchResponse, error) {
	if list == nil || req.AuthIndex == "" {
		return quotaFetchResponse{}, errors.New("copilot quota: cannot verify host credential state")
	}
	entries, err := list(ctx)
	if err != nil {
		return quotaFetchResponse{}, errors.New("copilot quota: cannot verify host credential state")
	}
	count := 0
	allowed := false
	for _, entry := range entries {
		if entry.AuthIndex == req.AuthIndex {
			count++
			allowed = !entry.Disabled && entry.Provider == "copilot" && (req.AuthID == "" || entry.ID == req.AuthID)
		}
	}
	if count != 1 || !allowed {
		return quotaFetchResponse{}, errors.New("copilot quota: host credential unavailable or disabled")
	}
	return fetchQuota(ctx, req, client)
}
