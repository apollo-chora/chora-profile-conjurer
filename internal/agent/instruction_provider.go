// instruction_provider.go — per-turn InstructionProvider wiring for the
// profile_conjurer crew (ADR-169 web-mode migration).
//
// The conjurer composer interpolates the bio + course titles + identity from a
// TaskContext. At boot those values are unknown, so the agent must recompose its
// instruction PER TURN from the session state the chora-sharing caller
// populated at async_create_session (bio / course_titles_json / tenant_id /
// user_gcid). llmagent.Config.InstructionProvider takes precedence over the
// static Instruction field, so wiring this keeps the prompt live rather than
// freezing a boot-time placeholder into every call.
package agent

import (
	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
)

// NewInstructionProvider returns an llmagent.InstructionProvider that composes
// the conjurer's instruction from the live session state on each turn. Wire it
// into llmagent.Config.InstructionProvider.
func NewInstructionProvider(role AgentRole) llmagent.InstructionProvider {
	return func(rc agent.ReadonlyContext) (string, error) {
		var tc TaskContext
		if rc != nil {
			if st := rc.ReadonlyState(); st != nil {
				tc = TaskContextFromState(st)
			}
		}
		return ComposeInstruction(role, tc), nil
	}
}
