package agentconfig_test

import (
	"testing"

	"github.com/apollo-chora/chora-profile-conjurer/internal/agentconfig"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProfileConjurer_TierLadders(t *testing.T) {
	cfg, err := agentconfig.ProfileConjurer()
	require.NoError(t, err)
	assert.Equal(t, "profile_conjurer", cfg.Agent)

	// CHEAP tier — conjurer (bounded-label extraction: bio → taxonomy tags +
	// course titles → proficiency). A bounded-label task, so the cheap tier is
	// the correct fit and keeps this latency-relevant profiler-page call
	// inexpensive. The model id is NOT declared here: it is env-only
	// (PROFILE_CONJURER_MODEL), so no literal model name lives in the YAML.
	conjurer, err := cfg.Sub("conjurer")
	require.NoError(t, err)
	assert.Equal(t, "cheap", conjurer.Tier)
	assert.Equal(t, "v1", conjurer.PromptVersion)
}

func TestSub_MissingSubAgentFailsLoud(t *testing.T) {
	cfg, err := agentconfig.ProfileConjurer()
	require.NoError(t, err)
	_, err = cfg.Sub("nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no sub-agent")
}
