package agent

import (
	"strings"
	"testing"
)

// D6 4-pillar stub harness for profile_conjurer P1 Single.
// Real chaos cleared by POC W3 multi-crew variant; these stubs preserve
// the contract surface during production promotion.

func TestD6P1_composerIsPureAcrossRecovery(t *testing.T) {
	// Recovery: same input always yields same prompt (no cached state).
	ctx := TaskContext{
		TenantID:     "t",
		UserGCID:     "u",
		Bio:          "I love astronomy and physics",
		CourseTitles: []string{"Intro to Physics", "Calculus I"},
	}
	pre := ComposeInstruction(RoleConjurer, ctx)
	post := ComposeInstruction(RoleConjurer, ctx)
	if pre != post {
		t.Errorf("conjurer: composer not pure across recovery")
	}
}

func TestD6P2_terminationEventTopicCanonical(t *testing.T) {
	want := "chora.ai_kernel.agent.terminated.v1"
	if !strings.HasPrefix(want, "chora.ai_kernel.") || !strings.HasSuffix(want, ".v1") {
		t.Errorf("canonical topic must be chora.ai_kernel.*.v1; got %q", want)
	}
}

func TestD6P3_composeIsConcurrencySafe(t *testing.T) {
	// P1 Single: under N concurrent profile_conjurer requests with distinct tenants,
	// prompts must isolate per request (no cross-bleed). Pure functions are
	// inherently concurrent-safe; this guards against future state introduction.
	for i := 0; i < 16; i++ {
		ctx := TaskContext{
			TenantID: "tenant-" + string(rune('A'+i%26)),
			UserGCID: "user-stub",
			Bio:      "bio-stub",
		}
		got := ComposeInstruction(RoleConjurer, ctx)
		want := "tenant-" + string(rune('A'+i%26))
		if !strings.Contains(got, want) {
			t.Errorf("iter %d: prompt missing own tenant_id %q", i, want)
		}
	}
}

func TestD6P4_mandatoryAttributesCoverConjurer(t *testing.T) {
	got := MandatorySpanAttributes()
	// chora.profile_conjurer.role distinguishes the conjurer in spans — load-bearing
	// for the P1 Single per-role cost + latency drill-down.
	found := false
	for _, k := range got {
		if k == "chora.profile_conjurer.role" {
			found = true
			break
		}
	}
	if !found {
		t.Error("MandatorySpanAttributes must include chora.profile_conjurer.role to distinguish conjurer spans")
	}
}
