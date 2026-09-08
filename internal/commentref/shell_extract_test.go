package commentref

import "testing"

func TestExtractShell(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		src  string
		want []Comment
	}{
		{
			name: "standalone_and_trailing",
			src:  "#!/bin/bash\n# standalone\necho hi # trailing\n",
			want: []Comment{
				{Line: 1, Text: "!/bin/bash"},
				{Line: 2, Text: "standalone"},
				{Line: 3, Text: "trailing"},
			},
		},
		{
			name: "hash_inside_quotes_and_parameter_expansion_not_a_comment",
			src:  "a=(1 2)\necho \"$# and ${#a[@]}\"\necho 'literal # not comment'\n",
			want: nil,
		},
		{
			name: "heredoc_body_not_a_comment",
			src:  "cat <<'EOF'\n# not a comment\nEOF\n# after heredoc\n",
			want: []Comment{{Line: 4, Text: "after heredoc"}},
		},
		{
			name: "unquoted_heredoc_body_not_a_comment",
			src:  "cat <<EOF\n# not a comment\nEOF\n",
			want: nil,
		},
		{
			name: "eof_comment",
			src:  "echo hi\n# trailing eof comment",
			want: []Comment{{Line: 2, Text: "trailing eof comment"}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := ExtractShell([]byte(tc.src))
			if err != nil {
				t.Fatalf("ExtractShell() error = %v", err)
			}
			assertComments(t, got, tc.want)
		})
	}
}

// TestExtractShell_ReportsSomething is the instrument check.
func TestExtractShell_ReportsSomething(t *testing.T) {
	t.Parallel()
	got, err := ExtractShell([]byte("# x\necho hi\n"))
	if err != nil {
		t.Fatalf("ExtractShell() error = %v", err)
	}
	if len(got) == 0 {
		t.Fatal("ExtractShell() returned no comments for a script that has one")
	}
}

// TestExtractShell_WholeTree parses every tracked shell script in the
// repository and asserts no parse error.
func TestExtractShell_WholeTree(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	files := gitLsFiles(t, root, "*.sh")
	if len(files) == 0 {
		t.Fatal("git ls-files reported no .sh files")
	}
	for _, f := range files {
		content := readRepoFile(t, root, f)
		if _, err := ExtractShell(content); err != nil {
			t.Errorf("ExtractShell(%s): %v", f, err)
		}
	}
}
