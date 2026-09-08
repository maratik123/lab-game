package commentref

import "testing"

func TestExtractGitignore(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		src  string
		want []Comment
	}{
		{
			name: "leading_hash_is_a_comment",
			src:  "# ignore build output\n/dist\n",
			want: []Comment{{Line: 1, Text: "ignore build output"}},
		},
		{
			name: "hash_after_a_pattern_is_part_of_the_pattern",
			src:  "foo # bar\n",
			want: nil,
		},
		{
			name: "escaped_leading_hash_is_a_pattern_not_a_comment",
			src:  "\\#starts-with-hash\n",
			want: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := ExtractGitignore([]byte(tc.src))
			if err != nil {
				t.Fatalf("ExtractGitignore() error = %v", err)
			}
			assertComments(t, got, tc.want)
		})
	}
}
