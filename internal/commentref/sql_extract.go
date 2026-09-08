package commentref

import "strings"

// ExtractSQL returns every comment in a SQL source file: a `--` line
// comment and a `/* */` block comment, neither recognised inside a
// single-quoted string literal. A goose annotation (`-- +goose Up`) is
// reported like any other line comment; its own exemption is a
// classification concern, not an extraction one.
func ExtractSQL(src []byte) ([]Comment, error) {
	s := string(src)
	n := len(s)
	line := 1
	var out []Comment

	for i := 0; i < n; {
		switch c := s[i]; {
		case c == '\n':
			line++
			i++
		case c == '\'':
			i, line = skipSQLString(s, i, line)
		case c == '-' && i+1 < n && s[i+1] == '-':
			start := i + 2
			end := start
			for end < n && s[end] != '\n' {
				end++
			}
			out = append(out, Comment{Line: line, Text: strings.TrimPrefix(s[start:end], " ")})
			i = end
		case c == '/' && i+1 < n && s[i+1] == '*':
			startLine := line
			i += 2
			var buf strings.Builder
			for i < n && (s[i] != '*' || i+1 >= n || s[i+1] != '/') {
				if s[i] == '\n' {
					line++
				}
				buf.WriteByte(s[i])
				i++
			}
			if i < n {
				i += 2
			}
			for j, l := range strings.Split(buf.String(), "\n") {
				out = append(out, Comment{Line: startLine + j, Text: strings.TrimPrefix(strings.TrimRight(l, " \t\r"), " ")})
			}
		default:
			i++
		}
	}
	return out, nil
}

// skipSQLString advances past a single-quoted string literal starting at
// s[i], which must be the opening quote, handling a doubled quote (”) as
// an escaped quote rather than the string's end. It returns the index just
// past the string and the line number reached.
func skipSQLString(s string, i, line int) (int, int) {
	n := len(s)
	i++
	for i < n {
		if s[i] == '\'' {
			if i+1 < n && s[i+1] == '\'' {
				i += 2
				continue
			}
			return i + 1, line
		}
		if s[i] == '\n' {
			line++
		}
		i++
	}
	return i, line
}
