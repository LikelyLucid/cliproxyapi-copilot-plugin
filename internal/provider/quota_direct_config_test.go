package provider

import "testing"

func TestQuotaDirectConfigRejectsNonLiteralBoolean(t *testing.T) {
	for _, raw := range []string{"yes", "on", "TRUE", "True", "\"true\""} {
		if _, err := ParseConfig([]byte("quota.direct-github: " + raw)); err == nil {
			t.Errorf("accepted non-literal opt-in %q", raw)
		}
	}
}

func TestQuotaDirectConfigExplicitOptIn(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want bool
	}{
		{"", false}, {"quota.direct-github: false", false}, {"quota.direct-github: true", true}, {"quota:\n  direct-github: true", false},
	} {
		cfg, err := ParseConfig([]byte(tc.raw))
		if err != nil {
			t.Fatal(err)
		}
		if bool(cfg.QuotaDirectGitHub) != tc.want {
			t.Fatalf("%q: got %v, want %v", tc.raw, cfg.QuotaDirectGitHub, tc.want)
		}
	}
}
