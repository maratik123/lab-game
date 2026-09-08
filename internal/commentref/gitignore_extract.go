package commentref

import "strings"

// ExtractGitignore returns every comment in a `.gitignore` file. Git treats
// only a line whose very first character is `#` as a comment; a `#` after
// a pattern is part of the pattern, and a leading `\#` escapes a pattern
// that begins with a hash, so neither is reported.
func ExtractGitignore(src []byte) ([]Comment, error) {
	lines := strings.Split(string(src), "\n")
	out := make([]Comment, 0, len(lines))
	for i, l := range lines {
		if strings.HasPrefix(l, `\#`) {
			continue
		}
		if strings.HasPrefix(l, "#") {
			out = append(out, Comment{Line: i + 1, Text: strings.TrimPrefix(l[1:], " ")})
		}
	}
	return out, nil
}
