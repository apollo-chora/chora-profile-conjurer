// Package agent holds the profile_conjurer P1 Single crew's
// CREATE-pattern composer + D6 attribute contract.
//
// P1 Single pattern per crew-composition SKILL §1 + §2:
//   - Conjurer (T2 Flash Preview) — extracts taxonomy-constrained interest tags
//     from a free-text bio AND infers proficiency from completed course titles.
//
// profile_conjurer is a SINGLE-agent crew: the conjurer's output IS the final
// answer surfaced to the caller. No reflection pair, no sequential pipeline.
//
// Replaces chora-sharing's clients.StaticTagExtractor (the placeholder wiring
// with the "LLM extractor deferred" comment) and the unused in-process
// clients/tag_extractor.go. Output feeds the C+ profiler page (profiler_profiles)
// and, via the existing enterQueue profile-tag fallback, duel matchmaking.
//
// Unlike Moderation (P6 Reflection, author-facing) and Familiar (P1 +
// per-instance, learner-facing), the profile_conjurer is a one-shot learner-facing
// extraction call — the conjurer emits the final tags+proficiency JSON directly.
package agent

import (
	"encoding/json"
	"fmt"
	"strings"
)

// TaskContext is the per-call profile_conjurer context.
type TaskContext struct {
	TenantID string
	UserGCID string
	// Bio is the learner's free-text bio (the source of interest tags).
	Bio string
	// CourseTitles are the titles of courses the learner has completed (the
	// source of the proficiency inference).
	CourseTitles []string
}

// AgentRole identifies which composer to use. The profile_conjurer is a P1 Single crew
// so there is exactly one role: the conjurer.
type AgentRole string

const (
	RoleConjurer AgentRole = "conjurer"
)

// StateReader is the minimal readonly session-state seam used to build a
// per-turn TaskContext. It matches the ADK ReadonlyState contract
// (Get(key) → (value, error); error when the key is absent), so an
// llmagent.InstructionProvider can pass `rc.ReadonlyState()` directly while
// tests substitute a fake.
type StateReader interface {
	Get(key string) (any, error)
}

// TaskContextFromState builds a per-call TaskContext from the session state the
// chora-sharing caller populated at async_create_session (tenant_id /
// user_gcid / bio / course_titles_json). Missing / non-string keys yield empty
// fields (safe fallback handled downstream by safe()). course_titles_json is a
// JSON array string serialised by the caller; a missing/malformed value yields
// an empty slice (the conjurer still emits a valid instruction).
func TaskContextFromState(state StateReader) TaskContext {
	if state == nil {
		return TaskContext{}
	}
	return TaskContext{
		TenantID:     stateString(state, "tenant_id"),
		UserGCID:     stateString(state, "user_gcid"),
		Bio:          stateString(state, "bio"),
		CourseTitles: readCourseTitles(state, "course_titles_json"),
	}
}

// readCourseTitles decodes the JSON array of completed course titles the caller
// threads into session state under course_titles_json. Absent / malformed /
// non-array ⇒ empty slice — fail-soft so a broken caller payload never panics
// the composer (the conjurer still emits a valid instruction, just with an empty
// course list → beginner proficiency by default).
func readCourseTitles(state StateReader, key string) []string {
	raw, err := state.Get(key)
	if err != nil {
		return nil
	}
	var jsonBytes []byte
	switch v := raw.(type) {
	case string:
		if strings.TrimSpace(v) == "" {
			return nil
		}
		jsonBytes = []byte(v)
	case []string:
		if len(v) == 0 {
			return nil
		}
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, e := range v {
			if s, ok := e.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		if len(out) == 0 {
			return nil
		}
		return out
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return nil
		}
		jsonBytes = b
	}
	var out []string
	if err := json.Unmarshal(jsonBytes, &out); err != nil {
		return nil // malformed → empty (fail-soft)
	}
	return out
}

// stateString reads a string value from session state; absent / wrong-typed
// keys return "".
func stateString(state StateReader, key string) string {
	raw, err := state.Get(key)
	if err != nil {
		return ""
	}
	s, _ := raw.(string)
	return s
}

// ComposeInstruction emits the deterministic 6-block CREATE prompt for
// the specified role + task context. Pure function (IMDA D2 transparency).
func ComposeInstruction(role AgentRole, ctx TaskContext) string {
	// The profile_conjurer is a P1 Single crew — one role. Default to conjurer for any
	// unknown role so a misconfigured caller still gets a valid instruction
	// rather than an empty prompt.
	return composeConjurer(ctx)
}

// interestTagTaxonomy lists the 6 valid categories. Tag STRINGS are
// free-form within a category (e.g. programming accepts "golang",
// "rust", "system_design", "recursion" — any non-empty slug). This
// mirrors chora-sharing's profiler.IsValidTag, which validates the
// category + non-empty tag, NOT a closed vocabulary. The closed
// per-category tag lists were loosened because they blocked legitimate
// interests the taxonomy author didn't foresee (a bio about "golang"
// yielded zero tags).
//
// Categories are still enforced — they're the matchmaking signal (two
// "programming" learners still match). Tag canonicalisation (golang vs
// go vs Go) is a future Meilisearch-normalisation concern.
var interestTagTaxonomy = `Allowed categories (tag strings are free-form within each category — slugify: lowercase, spaces → underscores):
- programming — software, coding, CS theory, specific languages/tools (golang, rust, react, system_design, recursion, async)
- mathematics — calculus, linear_algebra, statistics, probability, discrete_math, number_theory, topology, etc.
- science — physics, chemistry, biology, astronomy, earth_science, neuroscience, etc.
- humanities — history, philosophy, literature, linguistics, sociology, economics, etc.
- arts — music_theory, visual_arts, digital_design, film_studies, creative_writing, etc.
- languages — english, mandarin, spanish, french, german, japanese, etc.`

func composeConjurer(ctx TaskContext) string {
	var b strings.Builder

	// [CONTEXT]
	b.WriteString("## [CONTEXT]\n")
	b.WriteString("You are running inside the Chora learning platform as the Conjurer " +
		"sub-agent of the profile_conjurer crew (P1 Single). You conjure a learner's " +
		"interest tags from their free-text bio AND infer their proficiency from the titles " +
		"of courses they have completed. Your output populates the C+ profiler page and " +
		"feeds duel matchmaking via the profile-tag fallback.\n")
	fmt.Fprintf(&b, "Tenant: %s. User GCID: %s.\n\n",
		safe(ctx.TenantID, "<unset>"), safe(ctx.UserGCID, "<unset>"))

	// [ROLE]
	b.WriteString("## [ROLE]\n")
	b.WriteString("You are the Conjurer — a single-shot extractor. You read a learner's bio " +
		"and completed-course list and emit ONE JSON object: interest tags grouped by " +
		"category, plus an inferred proficiency level. You are NOT generating learning " +
		"content, moderating, or tutoring — you are extracting a structured profile. " +
		"Tags are FREE-FORM within the 6 categories below (e.g. programming/golang, " +
		"programming/rust, science/astrophysics) — emit the specific interest the user " +
		"declared, slugified (lowercase, spaces → underscores). Never invent a CATEGORY; " +
		"the 6 categories are the matchmaking signal + must stay canonical.")
	b.WriteString("\n\n")

	// [EXAMPLES]
	b.WriteString("## [EXAMPLES]\n")
	b.WriteString("Example 1: bio=\"I love astronomy and physics\", courses=[\"Intro to " +
		"Physics\",\"Calculus I\"] → {\"tags\":{\"science\":[\"astronomy\",\"physics\"]," +
		"\"mathematics\":[\"calculus\"]},\"proficiency\":{\"per_category\":{\"science\":" +
		"\"beginner\",\"mathematics\":\"beginner\"}}}\n")
	b.WriteString("Example 2: bio=\"Full-stack dev into functional programming\", " +
		"courses=[\"Intro to Physics\",\"Calculus I\",\"Calculus II\",\"Linear Algebra\"," +
		"\"Discrete Math\",\"Design Patterns\"] → {\"tags\":{\"programming\":" +
		"[\"functional_programming\",\"design_patterns\"],\"mathematics\":[\"calculus\"," +
		"\"linear_algebra\",\"discrete_math\"]},\"proficiency\":{\"per_category\":" +
		"{\"programming\":\"intermediate\",\"mathematics\":\"advanced\"}}}\n")
	b.WriteString("Example 3: bio=\"\", courses=[] → {\"tags\":{},\"proficiency\":" +
		"{\"per_category\":{}}} (empty profile — no signal, empty proficiency map " +
		"is valid; the smith agent defaults to beginner-level atoms)\n")
	b.WriteString("Example 4: bio=\"I love software engineering, especially golang and rust\", " +
		"courses=[\"Distributed Systems\"] → {\"tags\":{\"programming\":" +
		"[\"golang\",\"rust\",\"distributed_systems\"]},\"proficiency\":" +
		"{\"per_category\":{\"programming\":\"intermediate\"}}} (free-form tags within " +
		"programming — the specific tools/languages the learner declared, slugified)\n\n")

	// [AUDIENCE]
	b.WriteString("## [AUDIENCE]\n")
	b.WriteString("Your JSON is the FINAL answer — there is no second agent. The " +
		"chora-sharing service validates the category (unknown categories are dropped) " +
		"and accepts the per-category proficiency map free-form (an empty map is valid " +
		"— a new user with no signal). Tag strings are accepted free-form. It persists " +
		"the profile to profiler_profiles and renders it on the C+ profiler page.\n\n")

	// [TASK]
	b.WriteString("## [TASK]\n")
	fmt.Fprintf(&b, "Bio:\n```\n%s\n```\n", safe(ctx.Bio, "<no bio supplied>"))
	b.WriteString("Completed course titles:\n")
	if len(ctx.CourseTitles) == 0 {
		b.WriteString("(none)\n")
	} else {
		for i, t := range ctx.CourseTitles {
			fmt.Fprintf(&b, "%d. %s\n", i+1, t)
		}
	}
	b.WriteString("\n")
	b.WriteString(interestTagTaxonomy)
	b.WriteString("\n\n")
	b.WriteString("Extract interest tags + infer proficiency:\n")
	b.WriteString("- Read the bio for self-declared interests (e.g. \"I love astronomy\" → " +
		"science/astronomy, \"I'm into golang\" → programming/golang). Emit the SPECIFIC " +
		"interest the user declared, slugified (lowercase, spaces → underscores, e.g. " +
		"\"distributed systems\" → distributed_systems). If a bio topic does not map to " +
		"ANY of the 6 categories, OMIT it — but most technical/hobby interests fit " +
		"programming, science, arts, or humanities.\n")
	b.WriteString("- Read the completed-course titles for additional interest signals (a course " +
		"titled \"Calculus I\" implies mathematics/calculus) AND for the proficiency " +
		"inference below.\n")
	b.WriteString("- Proficiency rubric (per category — emit a level for each category " +
		"where you have signal from the bio or course titles):\n")
	b.WriteString("  * beginner     — 0–2 completed courses in that category (or shallow " +
		"topic coverage).\n")
	b.WriteString("  * intermediate — 3–5 completed courses in that category.\n")
	b.WriteString("  * advanced     — 6+ completed courses in that category, adjusted " +
		"upward by topic depth signalled in the titles (e.g. \"Calculus II\" + " +
		"\"Linear Algebra\" + \"Discrete Math\" together signal mathematics depth).\n")
	b.WriteString("- per_category is the ONLY proficiency field — there is no overall " +
		"\"beginner/intermediate/advanced\" (\"beginner in what?\" was meaningless " +
		"without category context). Emit a per_category entry for each category where " +
		"the learner has signal. An empty per_category map is valid (a new user with no " +
		"courses + no categorisable bio).\n")
	b.WriteString("- Tags are grouped by category; emit each category at most once with its " +
		"tag list. Emit a category only when at least one tag applies.\n\n")

	// [EXPECTED OUTPUT]
	b.WriteString("## [EXPECTED OUTPUT]\n")
	b.WriteString("JSON only, no markdown fences, no commentary outside the JSON. Shape:\n")
	b.WriteString("{\"tags\": {\"<category>\": [\"<tag>\"]}, " +
		"\"proficiency\": {\"per_category\": {\"<category>\": \"<level>\"}}}\n")
	b.WriteString("Where <category> is one of the taxonomy categories above, <tag> is a " +
		"free-form slug (lowercase, underscores for spaces — e.g. golang, " +
		"distributed_systems, astrophysics) representing the specific interest, and " +
		"<level> is one of beginner|intermediate|advanced. per_category is the ONLY " +
		"proficiency field (no overall). Emit a per_category entry for each category " +
		"where the learner has signal; an empty per_category map {} is valid when there " +
		"is no signal. tags may be an empty object {} when no category applies.\n")

	return b.String()
}

func safe(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}
