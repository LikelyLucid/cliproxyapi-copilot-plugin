package main

import (
	"strings"
	"testing"

	"github.com/arthur-sommer-etc/cliproxyapi-copilot-plugin/internal/provider"
)

func TestQuotaDispatchDirectRequiresOptIn(t *testing.T) {
	original := pluginService
	pluginService = provider.New(hostTransport{})
	defer func() { pluginService = original }()
	for _, tc := range []struct{ config, message string }{
		{"", "proxy policy"}, {"quota.direct-github: false", "proxy policy"}, {"quota.direct-github: true", "cannot verify host credential state"},
	} {
		if err := pluginService.Configure([]byte(tc.config)); err != nil {
			t.Fatal(err)
		}
		_, err := dispatch("quota.fetch", []byte(`{}`))
		if err == nil || !strings.Contains(err.Error(), tc.message) {
			t.Fatalf("config %q: %v", tc.config, err)
		}
	}
}
