package agent

import "google.golang.org/adk/session"

// conditions.go — ADR-197 M-A condition extractor for the profile_conjurer
// crew.
//
// The only prompt-shaping discriminant is the role (conjurer); the bio + course
// titles are the input content (never surfaced as a condition). The returned
// function matches promptstamping.ConditionExtractor so a main wires it
// directly.
func ProfilerConditions(role AgentRole) func(session.ReadonlyState) map[string]string {
	return func(session.ReadonlyState) map[string]string {
		return map[string]string{"role": string(role)}
	}
}
