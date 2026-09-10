package commentref

import (
	"testing"

	"github.com/maratik123/lab-game/internal/repotest"
)

func TestExtractSQL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		src  string
		want []Comment
	}{
		{
			name: "line_start_and_midline_dash_comment",
			src:  "-- header\nSELECT 1; -- trailing\n",
			want: []Comment{{Line: 1, Text: "header"}, {Line: 2, Text: "trailing"}},
		},
		{
			name: "dash_dash_inside_string_not_a_comment",
			src:  "SELECT '--not a comment';\n-- real\n",
			want: []Comment{{Line: 2, Text: "real"}},
		},
		{
			name: "block_comment",
			src:  "/* block\nspanning lines */\nSELECT 1;\n",
			want: []Comment{{Line: 1, Text: "block"}, {Line: 2, Text: "spanning lines"}},
		},
		{
			name: "goose_annotation",
			src:  "-- +goose Up\nCREATE TABLE t (id int);\n",
			want: []Comment{{Line: 1, Text: "+goose Up"}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := ExtractSQL([]byte(tc.src))
			if err != nil {
				t.Fatalf("ExtractSQL() error = %v", err)
			}
			assertComments(t, got, tc.want)
		})
	}
}

// TestExtractSQL_WholeTree parses every tracked migration file and asserts
// no error.
func TestExtractSQL_WholeTree(t *testing.T) {
	t.Parallel()
	root := repotest.Root(t)
	files := gitLsFiles(t, root, "*.sql")
	if len(files) == 0 {
		t.Fatal("git ls-files reported no .sql files")
	}
	for _, f := range files {
		content := readRepoFile(t, root, f)
		if _, err := ExtractSQL(content); err != nil {
			t.Errorf("ExtractSQL(%s): %v", f, err)
		}
	}
}

func TestExtractSQL_DoubledQuoteIsAnEscapedQuoteNotTheStringEnd(t *testing.T) {
	t.Parallel()
	got, err := ExtractSQL([]byte("SELECT 'it''s here -- not a comment';\n-- real\n"))
	if err != nil {
		t.Fatalf("ExtractSQL() error = %v", err)
	}
	assertComments(t, got, []Comment{{Line: 2, Text: "real"}})
}
