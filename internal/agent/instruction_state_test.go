package agent

import (
	"errors"
	"testing"
)

// fakeState is a minimal StateReader for testing TaskContextFromState without
// the ADK runtime. Get returns ErrAbsent for keys not in the map (mirroring
// the ADK session store's key-absent error).
type fakeState struct{ m map[string]any }

var errAbsent = errors.New("absent")

func (f fakeState) Get(key string) (any, error) {
	if v, ok := f.m[key]; ok {
		return v, nil
	}
	return nil, errAbsent
}

func TestTaskContextFromState_readsAllKeys(t *testing.T) {
	st := fakeState{m: map[string]any{
		"tenant_id":          "11111111-1111-7111-8111-111111111111",
		"user_gcid":          "00000000-0000-7000-8000-000000001999",
		"bio":                "I love astronomy and physics",
		"course_titles_json": `["Intro to Physics","Calculus I"]`,
	}}
	tc := TaskContextFromState(st)
	if tc.TenantID != "11111111-1111-7111-8111-111111111111" {
		t.Errorf("TenantID = %q", tc.TenantID)
	}
	if tc.UserGCID != "00000000-0000-7000-8000-000000001999" {
		t.Errorf("UserGCID = %q", tc.UserGCID)
	}
	if tc.Bio != "I love astronomy and physics" {
		t.Errorf("Bio = %q; want the real bio (not a placeholder)", tc.Bio)
	}
	if len(tc.CourseTitles) != 2 {
		t.Fatalf("CourseTitles len = %d; want 2", len(tc.CourseTitles))
	}
	if tc.CourseTitles[0] != "Intro to Physics" || tc.CourseTitles[1] != "Calculus I" {
		t.Errorf("CourseTitles = %v; want [Intro to Physics, Calculus I]", tc.CourseTitles)
	}
}

func TestTaskContextFromState_missingKeysAreEmpty(t *testing.T) {
	tc := TaskContextFromState(fakeState{m: map[string]any{}})
	if tc.TenantID != "" || tc.UserGCID != "" || tc.Bio != "" || tc.CourseTitles != nil {
		t.Errorf("missing keys should yield empty fields; got %+v", tc)
	}
}

func TestTaskContextFromState_nilStateIsEmpty(t *testing.T) {
	tc := TaskContextFromState(nil)
	if tc.TenantID != "" || tc.UserGCID != "" || tc.Bio != "" || tc.CourseTitles != nil {
		t.Errorf("nil state should yield zero TaskContext; got %+v", tc)
	}
}

func TestTaskContextFromState_courseTitlesJSON_parsed(t *testing.T) {
	// course_titles_json is a JSON array STRING serialised by the caller. The
	// composer must parse it to []string so the course titles surface in the
	// TASK block rather than as a raw JSON blob.
	st := fakeState{m: map[string]any{
		"course_titles_json": `["Calculus I","Calculus II","Linear Algebra"]`,
	}}
	tc := TaskContextFromState(st)
	if len(tc.CourseTitles) != 3 {
		t.Fatalf("CourseTitles len = %d; want 3 (JSON array must be parsed)", len(tc.CourseTitles))
	}
	want := []string{"Calculus I", "Calculus II", "Linear Algebra"}
	for i, w := range want {
		if tc.CourseTitles[i] != w {
			t.Errorf("CourseTitles[%d] = %q; want %q", i, tc.CourseTitles[i], w)
		}
	}
}

func TestTaskContextFromState_courseTitlesJSON_nativeArray(t *testing.T) {
	// The ReasoningEngine JSON round-trip may hand back a native []any rather
	// than a JSON string. The composer must still extract the string titles.
	st := fakeState{m: map[string]any{
		"course_titles_json": []any{"Intro to Physics", "Calculus I"},
	}}
	tc := TaskContextFromState(st)
	if len(tc.CourseTitles) != 2 {
		t.Fatalf("CourseTitles len = %d; want 2 (native array must be extracted)", len(tc.CourseTitles))
	}
}

func TestTaskContextFromState_courseTitlesJSON_malformed_isEmpty(t *testing.T) {
	// A malformed JSON string must degrade to an empty slice (fail-soft), never
	// panic the composer.
	st := fakeState{m: map[string]any{
		"course_titles_json": `not valid json`,
	}}
	tc := TaskContextFromState(st)
	if tc.CourseTitles != nil {
		t.Errorf("malformed JSON should yield nil CourseTitles; got %v", tc.CourseTitles)
	}
}

// The composed instruction must carry the real bio — the regression that
// stranded moderation (static placeholder "<filled at runtime>" reached the
// model). The profile_conjurer must not repeat that mistake.
func TestComposeInstruction_fromState_carriesBio(t *testing.T) {
	st := fakeState{m: map[string]any{"bio": "I love astronomy"}}
	got := ComposeInstruction(RoleConjurer, TaskContextFromState(st))
	if !contains(got, "I love astronomy") {
		t.Errorf("conjurer instruction must contain the runtime bio")
	}
	if contains(got, "filled at runtime") {
		t.Errorf("conjurer instruction must NOT contain a boot placeholder")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
