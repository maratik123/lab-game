package commentref

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// ExtractShell returns every comment in a shell script, parsed with the
// grammar `mvdan.cc/sh/v3` implements for `shfmt`. A `#` inside a quoted
// string, a heredoc body, `$#`, and `${#a[@]}` are never reported, because
// the parser resolves them by grammar rather than by scanning for the
// marker. The shebang line is reported like any other comment; callers that
// need to exempt it do so by its own leading text.
func ExtractShell(src []byte) ([]Comment, error) {
	parser := syntax.NewParser(syntax.KeepComments(true))
	f, err := parser.Parse(bytes.NewReader(src), "")
	if err != nil {
		return nil, fmt.Errorf("shell source: %w", err)
	}

	var raw []syntax.Comment
	syntax.Walk(f, func(n syntax.Node) bool {
		if stmt, ok := n.(*syntax.Stmt); ok {
			raw = append(raw, stmt.Comments...)
		}
		return true
	})
	raw = append(raw, f.Last...)

	out := make([]Comment, 0, len(raw))
	for _, c := range raw {
		out = append(out, Comment{Line: int(c.Hash.Line()), Text: strings.TrimPrefix(c.Text, " ")})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Line < out[j].Line })
	return out, nil
}
