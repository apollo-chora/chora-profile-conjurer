// Package agentconfig holds the build-time, per-agent model + prompt
// configuration for the profile_conjurer crew (AGENT-DRIVEN tiering,
// CR mana-is-quota-not-model-selector 2026-06-01).
//
// Mirrors the moderation / qgen agentconfig pattern (see
// moderation_adk_go/internal/agentconfig). Each agent owns its OWN embedded YAML
// (separation of concern) declaring, per sub-agent: tier / primary_model /
// fallback_models / prompt_version. The agent reads its config at boot and:
//   - constructs ONE gemini.NewModel per sub-agent from primary_model routed
//     through chora-model-gateway (ADR-177 full mana umbrella),
//   - carries fallback_models as the agent-DECLARED fallback chain (advisory
//     until a future gateway cutover wires it to
//     InvokeRequest.fallback_logical_model_ids, mirroring qgen),
//   - selects the prompt_version'd template.
//
// The YAML is the single source of truth; ops may override an individual
// primary via env var for quick experiments, but fallback + tier + prompt
// version stay config-declared.
//
// Model selection is AGENT-DRIVEN — mana is a token-budget QUOTA system
// (manaplugin gate), NOT a model selector. The old tieredmodelplugin is no
// longer registered (see feedback_mana_is_quota_not_model_selector).
package agentconfig

import (
	_ "embed"
	"fmt"

	"gopkg.in/yaml.v3"
)

//go:embed profile_conjurer.yaml
var profileConjurerYAML []byte

// SubAgentConfig is one sub-agent's resolved model tier + prompt selection.
type SubAgentConfig struct {
	Tier           string   `yaml:"tier"`
	PrimaryModel   string   `yaml:"primary_model"`
	FallbackModels []string `yaml:"fallback_models"`
	PromptVersion  string   `yaml:"prompt_version"`
}

// AgentConfig is one agent binary's full per-sub-agent config.
type AgentConfig struct {
	Agent     string                    `yaml:"agent"`
	SubAgents map[string]SubAgentConfig `yaml:"sub_agents"`
}

// Sub returns the named sub-agent config, failing loud if the YAML omits it —
// a missing sub-agent is a build/config error, never a silent default (per
// feedback_no_stubs_real_wiring).
func (c AgentConfig) Sub(name string) (SubAgentConfig, error) {
	sc, ok := c.SubAgents[name]
	if !ok {
		return SubAgentConfig{}, fmt.Errorf("agentconfig: agent %q has no sub-agent %q", c.Agent, name)
	}
	if sc.PrimaryModel == "" {
		return SubAgentConfig{}, fmt.Errorf("agentconfig: agent %q sub-agent %q has empty primary_model", c.Agent, name)
	}
	if sc.PromptVersion == "" {
		return SubAgentConfig{}, fmt.Errorf("agentconfig: agent %q sub-agent %q has empty prompt_version", c.Agent, name)
	}
	return sc, nil
}

// ProfileConjurer parses the embedded profile_conjurer config.
func ProfileConjurer() (AgentConfig, error) { return parse(profileConjurerYAML) }

func parse(raw []byte) (AgentConfig, error) {
	var c AgentConfig
	if err := yaml.Unmarshal(raw, &c); err != nil {
		return AgentConfig{}, fmt.Errorf("agentconfig: unmarshal: %w", err)
	}
	if c.Agent == "" {
		return AgentConfig{}, fmt.Errorf("agentconfig: missing top-level agent name")
	}
	if len(c.SubAgents) == 0 {
		return AgentConfig{}, fmt.Errorf("agentconfig: agent %q declares no sub_agents", c.Agent)
	}
	return c, nil
}
