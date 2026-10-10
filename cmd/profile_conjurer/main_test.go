package main

// Interim W3 stamp (ADR-254 D7, coordinator ruling 2026-08-22 17:57Z): every
// gateway client of this web-mode binary names its crew as the Invoke
// surface; the D7 gateway refuses an unstamped call. The tests pin the stamp
// per client so a future edit cannot drop it silently.

import (
	"testing"

	"github.com/apollo-chora/chora-profile-conjurer/internal/agentconfig"
)

func Test_conjurerGatewayConfig_stampsTheSurface(t *testing.T) {
	cfg := conjurerGatewayConfig(agentconfig.SubAgentConfig{FallbackModels: []string{"fb"}}, "longcat-2.5-preview", "crew-x", "gw:443", "gcid-1", "tenant-1", "https://gateway.test.invalid")
	if cfg.Surface != crewSurface || crewSurface != "profile_conjurer" {
		t.Fatalf("surface = %q, want the ADR-254 D7 crew id profile_conjurer", cfg.Surface)
	}
	if cfg.AgentID != "profile_conjurer" || cfg.Endpoint != "gw:443" || cfg.TenantID != "tenant-1" || cfg.GCID != "gcid-1" || cfg.CrewKind != "crew-x" || cfg.LogicalModelID != "longcat-2.5-preview" || len(cfg.FallbackModelIDs) != 1 || cfg.FallbackModelIDs[0] != "fb" {
		t.Fatalf("identity changed: %+v", cfg)
	}
	if cfg.ActionCode != "profile_conjurer" {
		t.Fatalf("action code = %q, want %q", cfg.ActionCode, "profile_conjurer")
	}
}
