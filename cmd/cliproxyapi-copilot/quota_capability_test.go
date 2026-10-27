package main

import (
	"encoding/json"
	"testing"

	"github.com/arthur-sommer-etc/cliproxyapi-copilot-plugin/internal/provider"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
)

func TestQuotaCapabilityFollowsOptInAcrossLifecycle(t *testing.T) {
	previous := pluginService
	t.Cleanup(func() { pluginService = previous })
	// Use a separate service so this test does not retain configuration changes.
	pluginService = provider.New(hostTransport{})
	steps := []struct {
		method, config string
		want           bool
	}{
		{pluginabi.MethodPluginRegister, "", false},
		{pluginabi.MethodPluginReconfigure, "quota.direct-github: true\n", true},
		{pluginabi.MethodPluginReconfigure, "quota.direct-github: false\n", false},
	}
	for _, step := range steps {
		request, err := json.Marshal(lifecycleRequest{ConfigYAML: []byte(step.config)})
		if err != nil {
			t.Fatal(err)
		}
		result, err := dispatch(step.method, request)
		if err != nil {
			t.Fatal(err)
		}
		got := result.(registration).Capabilities.QuotaProvider
		if got != step.want {
			t.Fatalf("%s config %q: quota_provider = %v, want %v", step.method, step.config, got, step.want)
		}
	}
}
