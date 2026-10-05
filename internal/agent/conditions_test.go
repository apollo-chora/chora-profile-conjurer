package agent

// RED-first test for the ADR-197 M-A condition extractor for the
// profile_conjurer crew. The only prompt discriminant is the role (conjurer)
// — the bio + course titles are the input content, NOT discriminants, and are
// never surfaced as conditions.

import "testing"

func TestProfilerConditions_role(t *testing.T) {
	if c := ProfilerConditions(RoleConjurer)(nil); c["role"] != "conjurer" {
		t.Errorf("conjurer role: got %q", c["role"])
	}
}
