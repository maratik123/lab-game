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

// TestExtractYAML_EveryDocumentOfAStream asserts that a comment in the
// second or later document of a multi-document stream is reported with its
// own source line. Decoding only the first document returns no comment and
// no error, so the gate would fail open on such a file.
func TestExtractYAML_EveryDocumentOfAStream(t *testing.T) {
	t.Parallel()
	src := "# first\na: 1\n---\n# second\nb: 2\n---\n# third\nc: 3\n"
	got, err := ExtractYAML([]byte(src))
	if err != nil {
		t.Fatalf("ExtractYAML() error = %v, want nil", err)
	}
	want := []Comment{
		{Line: 1, Text: "first"},
		{Line: 4, Text: "second"},
		{Line: 7, Text: "third"},
	}
	if len(got) != len(want) {
		t.Fatalf("ExtractYAML() returned %d comments, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i].Line != w.Line || got[i].Text != w.Text {
			t.Errorf("comment %d = {Line:%d Text:%q}, want {Line:%d Text:%q}",
				i, got[i].Line, got[i].Text, w.Line, w.Text)
		}
	}
}

// TestExtractYAML_NodelessDocument asserts that a document holding no node
// still yields its comments. The parser reports end-of-stream for such a
// span, so a decoder loop alone returns clean and the gate fails open.
func TestExtractYAML_NodelessDocument(t *testing.T) {
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
			name: "comment-only document between two others",
			src:  "a: 1\n---\n# middle\n---\nb: 2\n",
			want: []Comment{{Line: 3, Text: "middle"}},
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

// TestExtractYAML_CommentBeforeFirstMarker asserts that a comment written
// above the first document marker is reported at its own line. The parser
// hands it back as the following document's head comment, whose lines are
// not contiguous with that document's own, so reconciling against the whole
// source turns a legal file into an instrument failure.
func TestExtractYAML_CommentBeforeFirstMarker(t *testing.T) {
	t.Parallel()
	got, err := ExtractYAML([]byte("# above\n---\n# below\na: 1\n"))
	if err != nil {
		t.Fatalf("ExtractYAML() error = %v, want nil", err)
	}
	assertComments(t, got, []Comment{{Line: 1, Text: "above"}, {Line: 3, Text: "below"}})
}
