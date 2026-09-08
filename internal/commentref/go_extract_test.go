package commentref

import (
	"testing"
)

func TestExtractGo(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		src  string
		want []Comment
	}{
		{
			name: "doc_comment",
			src:  "// Foo does a thing.\npackage p\n",
			want: []Comment{{Line: 1, Text: "Foo does a thing."}},
		},
		{
			name: "trailing_comment",
			src:  "package p\n\nvar x = 1 // trailing\n",
			want: []Comment{{Line: 3, Text: "trailing"}},
		},
		{
			name: "block_comment_multiline",
			src:  "package p\n\n/* line one\nline two */\n",
			want: []Comment{
				{Line: 3, Text: "line one"},
				{Line: 4, Text: "line two"},
			},
		},
		{
			name: "url_inside_interpreted_string_not_a_comment",
			src: `package p

var u = "https://api.telegram.org"
`,
			want: nil,
		},
		{
			name: "url_inside_raw_string_not_a_comment",
			src:  "package p\n\nvar u = `https://api.telegram.org`\n",
			want: nil,
		},
		{
			name: "comment_beside_string_with_slashes",
			src: `package p

var u = "https://a" // real comment
`,
			want: []Comment{{Line: 3, Text: "real comment"}},
		},
		{
			name: "last_line_is_comment",
			src:  "package p\n// eof",
			want: []Comment{{Line: 2, Text: "eof"}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := ExtractGo([]byte(tc.src))
			if err != nil {
				t.Fatalf("ExtractGo() error = %v", err)
			}
			assertComments(t, got, tc.want)
		})
	}
}

// TestExtractGo_ReportsSomething is the instrument check: a case with a
// non-empty expected list beside the empty-list cases above, so an
// extractor that always returns nothing cannot pass this file's table.
func TestExtractGo_ReportsSomething(t *testing.T) {
	t.Parallel()
	got, err := ExtractGo([]byte("// x\npackage p\n"))
	if err != nil {
		t.Fatalf("ExtractGo() error = %v", err)
	}
	if len(got) == 0 {
		t.Fatal("ExtractGo() returned no comments for a file that has one")
	}
}

// TestExtractGo_WholeTree parses every tracked Go file in the repository
// and asserts it does not error, exercising the extractor against real
// source rather than only fixtures.
func TestExtractGo_WholeTree(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	files := gitLsFiles(t, root, "*.go")
	if len(files) == 0 {
		t.Fatal("git ls-files reported no .go files")
	}
	for _, f := range files {
		content := readRepoFile(t, root, f)
		if _, err := ExtractGo(content); err != nil {
			t.Errorf("ExtractGo(%s): %v", f, err)
		}
	}
}
