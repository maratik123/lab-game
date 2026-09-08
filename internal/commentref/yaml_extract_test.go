package commentref

import (
	"errors"
	"testing"
)

func TestExtractYAML(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		src  string
		want []Comment
	}{
		{
			name: "line_comment",
			src:  "world:\n  seed: 1 # inline\n",
			want: []Comment{{Line: 2, Text: "inline"}},
		},
		{
			name: "head_comment_immediately_above",
			src:  "world:\n  # above name\n  name: hex\n",
			want: []Comment{{Line: 2, Text: "above name"}},
		},
		{
			name: "head_comment_across_blank_line",
			src:  "# file header\n# second header line\n\nworld:\n  cap: 5\n",
			want: []Comment{
				{Line: 1, Text: "file header"},
				{Line: 2, Text: "second header line"},
			},
		},
		{
			name: "foot_comment_below_its_node",
			src:  "config:\n  a: 1\n  # foot for a\nb: 2\n",
			want: []Comment{{Line: 3, Text: "foot for a"}},
		},
		{
			name: "trailing_eof_comment",
			src:  "b: 2\n# trailing eof comment\n",
			want: []Comment{{Line: 2, Text: "trailing eof comment"}},
		},
		{
			name: "hash_inside_quoted_scalar_not_a_comment",
			src:  "url: \"http://example.com#not-a-comment\"\n",
			want: nil,
		},
		{
			name: "comment_lines_inside_block_scalar_not_reported",
			src:  "run: |\n  echo hi\n  # not a comment, inside the block scalar\n",
			want: nil,
		},
		{
			name: "comment_on_the_block_scalar_introducer_is_reported",
			src:  "run: | # introducer comment\n  echo hi\n",
			want: []Comment{{Line: 1, Text: "introducer comment"}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := ExtractYAML([]byte(tc.src))
			if err != nil {
				t.Fatalf("ExtractYAML() error = %v", err)
			}
			assertComments(t, got, tc.want)
		})
	}
}

// TestExtractYAML_Unreconciled asserts that a comment the reconciliation
// pass cannot match back to a source line surfaces as an error rather than
// a guessed line or a silent drop.
func TestExtractYAML_Unreconciled(t *testing.T) {
	t.Parallel()
	// A head comment whose recorded text does not appear verbatim above
	// its node cannot be found by the search, which must fail loudly.
	got, err := reconcileYAMLComment(yamlHead, 1, "# does not exist anywhere", nil)
	if err == nil {
		t.Fatalf("reconcileYAMLComment: got %v, nil error, want ErrYAMLCommentUnreconciled", got)
	}
	if !errors.Is(err, ErrYAMLCommentUnreconciled) {
		t.Fatalf("reconcileYAMLComment: error = %v, want ErrYAMLCommentUnreconciled", err)
	}
}

// TestExtractYAML_ReportsSomething is the instrument check.
func TestExtractYAML_ReportsSomething(t *testing.T) {
	t.Parallel()
	got, err := ExtractYAML([]byte("# x\na: 1\n"))
	if err != nil {
		t.Fatalf("ExtractYAML() error = %v", err)
	}
	if len(got) == 0 {
		t.Fatal("ExtractYAML() returned no comments for a file that has one")
	}
}

// TestExtractYAML_WholeTree parses every tracked YAML file in the
// repository and asserts no reconciliation failure.
func TestExtractYAML_WholeTree(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	files := append(gitLsFiles(t, root, "*.yml"), gitLsFiles(t, root, "*.yaml")...)
	if len(files) == 0 {
		t.Fatal("git ls-files reported no yaml files")
	}
	for _, f := range files {
		content := readRepoFile(t, root, f)
		if _, err := ExtractYAML(content); err != nil {
			t.Errorf("ExtractYAML(%s): %v", f, err)
		}
	}
}
