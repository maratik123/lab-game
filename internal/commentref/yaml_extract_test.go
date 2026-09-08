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

// TestExtractYAML_ParseError asserts that source the YAML parser itself
// rejects surfaces as an error.
func TestExtractYAML_ParseError(t *testing.T) {
	t.Parallel()
	_, err := ExtractYAML([]byte("a: [1, 2\n"))
	if err == nil {
		t.Fatal("ExtractYAML() error = nil, want the malformed source reported")
	}
}

// TestExtractYAML_RefusesAStreamItCannotPlace asserts that a source
// carrying a document marker or a directive is refused rather than read.
// The extractor reads one plain document; every other shape moves a
// comment's line away from where the parser reports it, and a guard that
// guesses a line is worse than one that stops.
func TestExtractYAML_RefusesAStreamItCannotPlace(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		src  string
	}{
		{name: "document start", src: "a: 1\n---\nb: 2\n"},
		{name: "document start carrying a comment", src: "a: 1\n--- # ref\nb: 2\n"},
		{name: "document start with content", src: "a: 1\n--- b\n"},
		{name: "document end", src: "a: 1\n...\n"},
		{name: "document end carrying a comment", src: "a: 1\n... # ref\n"},
		{name: "document start, tab before a comment", src: "a: 1\n---\t# ref\nb: 2\n"},
		{name: "document end, tab before a comment", src: "a: 1\n...\t# ref\n"},
		{name: "comment above the first marker", src: "# above\n---\na: 1\n"},
		{name: "yaml directive", src: "%YAML 1.1\n---\na: 1\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := ExtractYAML([]byte(tc.src))
			if !errors.Is(err, ErrYAMLUnsupportedStream) {
				t.Fatalf("ExtractYAML() error = %v, want ErrYAMLUnsupportedStream (got %d comments)", err, len(got))
			}
		})
	}
}

// TestExtractYAML_NodelessSource asserts that a source the parser yields no
// node for still reports its comments. The parser returns an empty node and
// no error for such a source, so reading the node alone reports nothing and
// the gate passes a file it never looked at.
func TestExtractYAML_NodelessSource(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		src  string
		want []Comment
	}{
		{
			name: "whole file is one comment",
			src:  "# alone\n",
			want: []Comment{{Line: 1, Text: "alone"}},
		},
		{
			name: "several comments and blank lines",
			src:  "# one\n\n#   two\n",
			want: []Comment{{Line: 1, Text: "one"}, {Line: 3, Text: "  two"}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := ExtractYAML([]byte(tc.src))
			if err != nil {
				t.Fatalf("ExtractYAML() error = %v, want nil", err)
			}
			assertComments(t, got, tc.want)
		})
	}
}
