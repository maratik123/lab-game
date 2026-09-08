package commentref

import "strings"

// ExtractEnvExample returns every comment in an example environment file,
// using the same rule `github.com/joho/godotenv` applies when it reads
// such a file for real: a line-start `#`, or a `#` preceded by whitespace once an
// unquoted value has ended. A `#` inside a quoted value is part of the
// value and is never reported.
func ExtractEnvExample(src []byte) ([]Comment, error) {
	lines := strings.Split(string(src), "\n")
	out := make([]Comment, 0, len(lines))
	for i, l := range lines {
		if c, ok := envLineComment(l); ok {
			out = append(out, Comment{Line: i + 1, Text: c})
		}
	}
	return out, nil
}

// envLineComment finds the comment, if any, carried by one line of a
// `.env`-shaped file.
func envLineComment(l string) (string, bool) {
	trimmed := strings.TrimLeft(l, " \t")
	if strings.HasPrefix(trimmed, "#") {
		return strings.TrimPrefix(strings.TrimPrefix(trimmed, "#"), " "), true
	}

	eq := strings.IndexByte(l, '=')
	if eq == -1 {
		return "", false
	}
	val := l[eq+1:]
	trimmedVal := strings.TrimLeft(val, " \t")
	if len(trimmedVal) > 0 && (trimmedVal[0] == '"' || trimmedVal[0] == '\'') {
		q := trimmedVal[0]
		rest := trimmedVal[1:]
		closeIdx := strings.IndexByte(rest, q)
		if closeIdx == -1 {
			return "", false
		}
		val = rest[closeIdx+1:]
	}
	idx := envHashAfterWhitespace(val)
	if idx == -1 {
		return "", false
	}
	return strings.TrimPrefix(val[idx+1:], " "), true
}

// envHashAfterWhitespace finds the `#` that ends an unquoted value: one
// preceded by whitespace, or one at the very start of s.
func envHashAfterWhitespace(s string) int {
	if len(s) > 0 && s[0] == '#' {
		return 0
	}
	for i := 1; i < len(s); i++ {
		if s[i] == '#' && (s[i-1] == ' ' || s[i-1] == '\t') {
			return i
		}
	}
	return -1
}
