package agent

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestComposeConjurer_emitsSixCreateBlocksInOrder(t *testing.T) {
	ctx := TaskContext{
		TenantID:     "tenant-x",
		UserGCID:     "gcid-y",
		Bio:          "I love astronomy and physics",
		CourseTitles: []string{"Intro to Physics", "Calculus I"},
	}
	got := ComposeInstruction(RoleConjurer, ctx)
	assertCreateBlocksOrdered(t, "conjurer", got)
}

func assertCreateBlocksOrdered(t *testing.T, role, got string) {
	t.Helper()
	blocks := []string{
		"[CONTEXT]",
		"[ROLE]",
		"[EXAMPLES]",
		"[AUDIENCE]",
		"[TASK]",
		"[EXPECTED OUTPUT]",
	}
	lastIdx := -1
	for _, b := range blocks {
		i := strings.Index(got, b)
		if i < 0 {
			t.Errorf("%s: CREATE block %q missing", role, b)
			continue
		}
		if i <= lastIdx {
			t.Errorf("%s: %q at %d after %d (out of order)", role, b, i, lastIdx)
		}
		lastIdx = i
	}
}

func TestCompose_isDeterministic(t *testing.T) {
	ctx := TaskContext{
		TenantID:     "t",
		UserGCID:     "u",
		Bio:          "hi",
		CourseTitles: []string{"Intro to Physics"},
	}
	x := ComposeInstruction(RoleConjurer, ctx)
	y := ComposeInstruction(RoleConjurer, ctx)
	if x != y {
		t.Errorf("conjurer: not deterministic (IMDA D2 break)")
	}
}

func TestCompose_bioSurfacesInTaskBlock(t *testing.T) {
	bio := "specific-marker-bio-7f3a2c"
	got := ComposeInstruction(RoleConjurer, TaskContext{
		TenantID: "t",
		UserGCID: "u",
		Bio:      bio,
	})
	if !strings.Contains(got, bio) {
		t.Errorf("bio must surface in TASK block; got prompt without %q", bio)
	}
}

func TestCompose_courseTitlesSurfaceInTaskBlock(t *testing.T) {
	course := "Specific Course Title Marker 9b1d"
	got := ComposeInstruction(RoleConjurer, TaskContext{
		TenantID:     "t",
		UserGCID:     "u",
		Bio:          "hi",
		CourseTitles: []string{course},
	})
	if !strings.Contains(got, course) {
		t.Errorf("course title must surface in TASK block; got prompt without %q", course)
	}
}

func TestCompose_taxonomyPresent(t *testing.T) {
	got := ComposeInstruction(RoleConjurer, TaskContext{
		TenantID: "t",
		UserGCID: "u",
		Bio:      "hi",
	})
	// All 6 categories must be present — they're the enforced structure
	// (free-form tags live within a category; the category is the
	// matchmaking signal).
	categories := []string{
		"programming",
		"mathematics",
		"science",
		"humanities",
		"arts",
		"languages",
	}
	for _, c := range categories {
		if !strings.Contains(got, c) {
			t.Errorf("taxonomy category %q must be present in the prompt", c)
		}
	}
	// The prompt must mention free-form tags + slugification — the new
	// contract (closed vocabulary was loosened so "golang" etc. work).
	freeFormMarkers := []string{
		"FREE-FORM", // ROLE section: "Tags are FREE-FORM within..."
		"slugified", // TASK section: slugify guidance
		"golang",    // Example 4 uses golang as the free-form case
	}
	for _, marker := range freeFormMarkers {
		if !strings.Contains(got, marker) {
			t.Errorf("free-form tag marker %q must be present in the prompt (taxonomy was loosened)", marker)
		}
	}
}

func TestCompose_proficiencyRubricPresent(t *testing.T) {
	got := ComposeInstruction(RoleConjurer, TaskContext{
		TenantID: "t",
		UserGCID: "u",
		Bio:      "hi",
	})
	for _, level := range []string{"beginner", "intermediate", "advanced"} {
		if !strings.Contains(got, level) {
			t.Errorf("proficiency rubric must mention level %q", level)
		}
	}
}

func TestCompose_expectedOutputContract(t *testing.T) {
	got := ComposeInstruction(RoleConjurer, TaskContext{
		TenantID: "t",
		UserGCID: "u",
		Bio:      "hi",
	})
	if !strings.Contains(got, "\"tags\"") {
		t.Error("EXPECTED OUTPUT must declare the tags field")
	}
	if !strings.Contains(got, "\"proficiency\"") {
		t.Error("EXPECTED OUTPUT must declare the proficiency field")
	}
	if strings.Contains(got, "\"overall\"") {
		t.Error("EXPECTED OUTPUT must NOT declare proficiency.overall (field was removed — per_category is the only proficiency field)")
	}
	if !strings.Contains(got, "\"per_category\"") {
		t.Error("EXPECTED OUTPUT must declare proficiency.per_category (the only proficiency field)")
	}
}

func TestCompose_emptyBioAndCourses_minimalValidInstruction(t *testing.T) {
	// An empty bio + empty course list must still emit all six CREATE blocks
	// (the caller may legitimately send an empty profile; the conjurer emits
	// a default beginner profile). No panic, no empty prompt.
	got := ComposeInstruction(RoleConjurer, TaskContext{
		TenantID: "t",
		UserGCID: "u",
	})
	assertCreateBlocksOrdered(t, "conjurer-empty", got)
	if !strings.Contains(got, "<no bio supplied>") {
		t.Error("empty bio must surface the <no bio supplied> placeholder")
	}
	if !strings.Contains(got, "(none)") {
		t.Error("empty course list must surface the (none) marker")
	}
}

func TestMandatorySpanAttributes_coversProfiler(t *testing.T) {
	required := []string{
		"chora.tenant_id",
		"chora.user_gcid",
		"chora.mana_tier",
		"chora.crew_kind",
		"chora.profile_conjurer.role",
		"gen_ai.request.model",
		"gen_ai.usage.output_tokens",
	}
	got := MandatorySpanAttributes()
	seen := make(map[string]struct{}, len(got))
	for _, k := range got {
		seen[k] = struct{}{}
	}
	for _, r := range required {
		if _, ok := seen[r]; !ok {
			t.Errorf("MandatorySpanAttributes missing %q", r)
		}
	}
}

func TestMandatorySpanAttributes_noDuplicates(t *testing.T) {
	got := MandatorySpanAttributes()
	seen := make(map[string]struct{}, len(got))
	for _, k := range got {
		if _, dup := seen[k]; dup {
			t.Errorf("duplicate attribute %q", k)
		}
		seen[k] = struct{}{}
	}
}

// -----------------------------------------------------------------------------
// Golden byte-equality — the conjurer prompt is frozen against testdata.
// -----------------------------------------------------------------------------

// update regenerates the testdata/golden_conjurer.txt file when an INTENTIONAL
// prompt change lands (run: go test ./internal/agent/ -update).
var update = flag.Bool("update", false, "regenerate golden files")

// goldenCtx is the fixed TaskContext the golden file captures. Any drift here
// means the conjurer prompt changed — exactly the regression the golden guards.
var goldenCtx = TaskContext{
	TenantID:     "tenant-x",
	UserGCID:     "gcid-y",
	Bio:          "I love astronomy and physics",
	CourseTitles: []string{"Intro to Physics", "Calculus I"},
}

func TestComposeInstruction_goldenConjurer(t *testing.T) {
	got := ComposeInstruction(RoleConjurer, goldenCtx)
	path := filepath.Join("testdata", "golden_conjurer.txt")
	if *update {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatalf("write golden %s: %v", path, err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v (run: go test ./internal/agent/ -update to generate)", path, err)
	}
	if got != string(want) {
		t.Errorf("conjurer prompt drifted from the captured golden.\n--- want ---\n%s\n--- got ---\n%s\n%s",
			string(want), got, firstDiff(string(want), got))
	}
}

// firstDiff returns a readable byte-level diff pointer for the golden assertion.
func firstDiff(want, got string) string {
	min := len(want)
	if len(got) < min {
		min = len(got)
	}
	for i := 0; i < min; i++ {
		if want[i] != got[i] {
			start := i - 20
			if start < 0 {
				start = 0
			}
			end := i + 20
			if end > min {
				end = min
			}
			return formatDiff(want, got, start, end, i)
		}
	}
	if len(want) != len(got) {
		i := min
		end := i + 20
		return formatDiff(want, got, i, end, i)
	}
	return ""
}

func formatDiff(want, got string, start, end, pos int) string {
	ws := want[start:end]
	gs := got[start:end]
	return fmt.Sprintf("first diff at byte %d:\nwant: %q\n got: %q\n", pos, ws, gs)
}
