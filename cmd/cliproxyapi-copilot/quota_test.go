package main

import (
	"encoding/json"
	"testing"
)

func TestQuotaRegistrationAndDescription(t *testing.T) {
	raw, err := json.Marshal(pluginRegistration())
	if err != nil {
		t.Fatal(err)
	}
	var registration map[string]any
	if err := json.Unmarshal(raw, &registration); err != nil {
		t.Fatal(err)
	}
	if registration["capabilities"].(map[string]any)["quota_provider"] != bool(pluginService.Config().QuotaDirectGitHub) {
		t.Fatal("quota provider capability does not match opt-in")
	}
	result, err := dispatch("quota.identifier", []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	raw, err = json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"identifier":"copilot"}` {
		t.Fatalf("identifier: %s", raw)
	}
	result, err = dispatch("quota.describe", []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	raw, err = json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var desc struct {
		Supported []string `json:"supported_providers"`
		Reset     bool     `json:"supports_reset"`
	}
	if err := json.Unmarshal(raw, &desc); err != nil {
		t.Fatal(err)
	}
	if len(desc.Supported) != 1 || desc.Supported[0] != "copilot" || desc.Reset {
		t.Fatalf("description: %s", raw)
	}
	if _, err := dispatch("quota.reset", []byte(`{}`)); err == nil {
		t.Fatal("reset must be unsupported")
	}
}
