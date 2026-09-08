package commentref

import "testing"

func TestExtractEnvExample(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		src  string
		want []Comment
	}{
		{
			name: "line_start_comment",
			src:  "# a header\nLAB_GAME_X=1\n",
			want: []Comment{{Line: 1, Text: "a header"}},
		},
		{
			name: "hash_after_unquoted_value",
			src:  "LAB_GAME_X=1 # note\n",
			want: []Comment{{Line: 1, Text: "note"}},
		},
		{
			name: "hash_inside_quoted_value_not_a_comment",
			src:  `LAB_GAME_X="a#b"` + "\n",
			want: nil,
		},
		{
			name: "hash_after_quoted_value",
			src:  `LAB_GAME_X="a#b" # note` + "\n",
			want: []Comment{{Line: 1, Text: "note"}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := ExtractEnvExample([]byte(tc.src))
			if err != nil {
				t.Fatalf("ExtractEnvExample() error = %v", err)
			}
			assertComments(t, got, tc.want)
		})
	}
}

// TestExtractEnvExample_TrackedFile parses the module's own tracked
// example environment file and asserts no error.
func TestExtractEnvExample_TrackedFile(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	content := readRepoFile(t, root, ".env.example")
	if _, err := ExtractEnvExample(content); err != nil {
		t.Errorf("ExtractEnvExample(.env.example): %v", err)
	}
}

func TestExtractEnvExample_EdgeCases(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		src  string
		want []Comment
	}{
		{
			name: "no_equals_sign_is_not_a_comment",
			src:  "not a key value line\n",
			want: nil,
		},
		{
			name: "unterminated_quote_yields_no_comment",
			src:  `LAB_GAME_X="unterminated` + "\n",
			want: nil,
		},
		{
			name: "unquoted_value_with_no_hash",
			src:  "LAB_GAME_X=1\n",
			want: nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := ExtractEnvExample([]byte(tc.src))
			if err != nil {
				t.Fatalf("ExtractEnvExample() error = %v", err)
			}
			assertComments(t, got, tc.want)
		})
	}
}
