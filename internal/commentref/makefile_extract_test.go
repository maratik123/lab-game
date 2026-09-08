package commentref

import "testing"

func TestExtractMakefile(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		src  string
		want []Comment
	}{
		{
			name: "line_start_comment",
			src:  "# header\nbuild:\n",
			want: []Comment{{Line: 1, Text: "header"}},
		},
		{
			name: "midline_comment",
			src:  "VAR = value # explains VAR\n",
			want: []Comment{{Line: 1, Text: "explains VAR"}},
		},
		{
			name: "comment_in_recipe_line",
			src:  "build:\n\techo hi # note\n",
			want: []Comment{{Line: 2, Text: "note"}},
		},
		{
			name: "escaped_hash_not_a_comment",
			src:  "target: \\#literal\n",
			want: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := ExtractMakefile([]byte(tc.src))
			if err != nil {
				t.Fatalf("ExtractMakefile() error = %v", err)
			}
			assertComments(t, got, tc.want)
		})
	}
}
