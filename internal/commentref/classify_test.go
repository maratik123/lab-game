package commentref

import "testing"

func modulePkgs(names ...string) map[string]struct{} {
	m := make(map[string]struct{}, len(names))
	for _, n := range names {
		m[n] = struct{}{}
	}
	return m
}

func classesOf(fs []Finding) []Class {
	out := make([]Class, len(fs))
	for i, f := range fs {
		out[i] = f.Class
	}
	return out
}

func containsClass(fs []Finding, want Class) bool {
	for _, f := range fs {
		if f.Class == want {
			return true
		}
	}
	return false
}

func TestClassify_OnePerClass(t *testing.T) {
	t.Parallel()
	pkgs := modulePkgs("scheduler", "store")

	tests := []struct {
		name string
		text string
		want Class
	}{
		{"locator_positive", "see cmd/bot/main.go:20 for the loop", ClassLocator},
		{"markdown_path_positive", "see docs/DESIGN.md for the shape", ClassMarkdownPath},
		{"ac_id_positive", "covers AC6 of the spec", ClassACID},
		{"decision_anchor_kd_positive", "per KD-9 the symbol stays", ClassDecisionAnchor},
		{"decision_anchor_d_positive", "design D4 decides this", ClassDecisionAnchor},
		{"section_positive", "docs/DESIGN.md §3.5", ClassSection},
		{"issue_positive", "opened as issue #18 originally", ClassIssue},
		{"repo_path_positive", "see internal/scheduler/schedule.go for the loop", ClassRepoPath},
		{"url_positive", "see https://api.telegram.org for the API", ClassURL},
		{"module_symbol_positive", "returns the error store.Post reports", ClassModuleSymbol},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := Classify(Comment{Line: 1, Text: tc.text}, "backoff", pkgs)
			if !containsClass(got, tc.want) {
				t.Fatalf("Classify(%q) = %v, want it to include %s", tc.text, classesOf(got), tc.want)
			}
		})
	}
}

func TestClassify_OneNegativePerClass(t *testing.T) {
	t.Parallel()
	pkgs := modulePkgs("scheduler", "store")

	tests := []struct {
		name string
		text string
	}{
		{"no_locator_for_a_plain_sentence", "this returns an error on failure"},
		{"no_markdown_path_for_ordinary_prose", "the format is documented inline"},
		{"no_ac_id_for_alternating_current", "the AC unit failed"},
		{"no_decision_anchor_for_an_ordinary_word", "the door shortens the road"},
		{"no_section_for_plain_prose", "returns an error when stamina is exhausted"},
		{"no_issue_for_a_hashtag_free_sentence", "returns the count of retries"},
		{"no_repo_path_for_an_ordinary_word", "the schedule runs hourly"},
		{"no_url_for_plain_prose", "call the API before returning"},
		{"no_module_symbol_for_bare_name", "returns a Session for the caller"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := Classify(Comment{Line: 1, Text: tc.text}, "backoff", pkgs)
			if len(got) != 0 {
				t.Fatalf("Classify(%q) = %v, want no findings", tc.text, got)
			}
		})
	}
}

func TestClassify_MultipleClassesInOneComment(t *testing.T) {
	t.Parallel()
	pkgs := modulePkgs("scheduler")
	got := Classify(Comment{Line: 1, Text: "see docs/DESIGN.md §3.5 and AC6"}, "backoff", pkgs)
	for _, want := range []Class{ClassMarkdownPath, ClassSection, ClassACID} {
		if !containsClass(got, want) {
			t.Errorf("Classify() = %v, want it to include %s", classesOf(got), want)
		}
	}
}

func TestClassify_DirectiveExemptions(t *testing.T) {
	t.Parallel()
	pkgs := modulePkgs("scheduler")

	tests := []struct {
		name string
		text string
		want []Class // nil means no findings
	}{
		{"go_directive_fully_exempt", "go:generate mockgen -destination=internal/scheduler/mock.go", nil},
		{"build_tag_fully_exempt", "+build linux,darwin", nil},
		{"goose_annotation_fully_exempt", "+goose Up", nil},
		{"shellcheck_directive_fully_exempt", "shellcheck disable=SC2086", nil},
		{"shebang_fully_exempt", "!/bin/bash", nil},
		{"nolint_directive_exempt_no_reason", "nolint:gosec", nil},
		{"nolint_reason_text_still_classified", "nolint:gosec -- see AC6 for the reason", []Class{ClassACID}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := Classify(Comment{Line: 1, Text: tc.text}, "backoff", pkgs)
			if tc.want == nil {
				if len(got) != 0 {
					t.Fatalf("Classify(%q) = %v, want no findings", tc.text, got)
				}
				return
			}
			for _, want := range tc.want {
				if !containsClass(got, want) {
					t.Errorf("Classify(%q) = %v, want it to include %s", tc.text, classesOf(got), want)
				}
			}
		})
	}
}

func TestClassify_TODOKeepsItsIssueNumberButABareOneIsCaught(t *testing.T) {
	t.Parallel()
	pkgs := modulePkgs()

	got := Classify(Comment{Line: 1, Text: "TODO(#18): revisit this"}, "backoff", pkgs)
	if containsClass(got, ClassIssue) {
		t.Fatalf("Classify(TODO(#18): …) = %v, want no issue finding", classesOf(got))
	}

	got = Classify(Comment{Line: 1, Text: "TODO(#18): also see #19"}, "backoff", pkgs)
	if !containsClass(got, ClassIssue) {
		t.Fatalf("Classify(…): got %v, want the bare #19 caught as issue", classesOf(got))
	}
}

func TestClassify_ModuleSymbol_SamePackagePasses(t *testing.T) {
	t.Parallel()
	pkgs := modulePkgs("scheduler")
	got := Classify(Comment{Line: 1, Text: "Schedule reports scheduler.ErrStale on a stale task"}, "scheduler", pkgs)
	if containsClass(got, ClassModuleSymbol) {
		t.Fatalf("Classify() = %v, want the same-package symbol exempt", classesOf(got))
	}
}

func TestClassify_ModuleSymbol_TestPackageSuffixStripped(t *testing.T) {
	t.Parallel()
	pkgs := modulePkgs("scheduler")
	got := Classify(Comment{Line: 1, Text: "fixture mirrors scheduler.ErrStale"}, "scheduler_test", pkgs)
	if containsClass(got, ClassModuleSymbol) {
		t.Fatalf("Classify() = %v, want scheduler_test's own package treated as scheduler", classesOf(got))
	}
}

func TestClassify_ModuleSymbol_StandardLibraryPasses(t *testing.T) {
	t.Parallel()
	pkgs := modulePkgs("scheduler")
	got := Classify(Comment{Line: 1, Text: "cancelled when errors.Is matches context.Canceled"}, "scheduler", pkgs)
	if containsClass(got, ClassModuleSymbol) {
		t.Fatalf("Classify() = %v, want stdlib-qualified symbols exempt", classesOf(got))
	}
}

func TestClassify_ModuleSymbol_NonGoFileHasNoOwnPackage(t *testing.T) {
	t.Parallel()
	pkgs := modulePkgs("config")
	got := Classify(Comment{Line: 1, Text: "loaded the same way config.Load does"}, "", pkgs)
	if !containsClass(got, ClassModuleSymbol) {
		t.Fatalf("Classify() = %v, want the module symbol caught in a file with no own package", classesOf(got))
	}
}

// TestClassify_ModuleSymbol_Collision is the D4 collision case: a
// qualifier that is both an internal package of this module and a
// third-party module name is flagged, because the classifier resolves
// only against this module's own package names and the direction is
// deliberately conservative.
func TestClassify_ModuleSymbol_Collision(t *testing.T) {
	t.Parallel()
	pkgs := modulePkgs("backoff")
	got := Classify(Comment{Line: 1, Text: "retries with backoff.NewExponentialBackOff"}, "scheduler", pkgs)
	if !containsClass(got, ClassModuleSymbol) {
		t.Fatalf("Classify() = %v, want backoff.X caught even though it may name the third-party module", classesOf(got))
	}
}
