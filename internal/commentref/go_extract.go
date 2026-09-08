package commentref

import (
	"fmt"
	"go/scanner"
	"go/token"
	"strings"
)

// ExtractGo returns every comment in a Go source file. It relies on the
// standard library's own scanner, so a marker inside a string or rune
// literal is never mistaken for one, and a block comment spanning several
// lines is split into one Comment per line.
func ExtractGo(src []byte) ([]Comment, error) {
	fset := token.NewFileSet()
	file := fset.AddFile("", fset.Base(), len(src))

	var firstErr error
	var errs scanner.ErrorList
	var sc scanner.Scanner
	sc.Init(file, src, func(pos token.Position, msg string) {
		errs.Add(pos, msg)
	}, scanner.ScanComments)

	var out []Comment
	for {
		pos, tok, lit := sc.Scan()
		if tok == token.EOF {
			break
		}
		if tok != token.COMMENT {
			continue
		}
		out = append(out, splitGoComment(file.Position(pos).Line, lit)...)
	}
	if len(errs) > 0 {
		firstErr = fmt.Errorf("go source: %w", errs[0])
	}
	return out, firstErr
}

// splitGoComment turns one scanned comment literal, which still carries its
// marker, into one Comment per physical line, numbered from startLine.
func splitGoComment(startLine int, lit string) []Comment {
	if strings.HasPrefix(lit, "//") {
		return []Comment{{Line: startLine, Text: strings.TrimPrefix(strings.TrimPrefix(lit, "//"), " ")}}
	}
	body := strings.TrimSuffix(strings.TrimPrefix(lit, "/*"), "*/")
	lines := strings.Split(body, "\n")
	out := make([]Comment, 0, len(lines))
	for i, line := range lines {
		out = append(out, Comment{
			Line: startLine + i,
			Text: strings.TrimPrefix(strings.TrimRight(line, " \t\r"), " "),
		})
	}
	return out
}
