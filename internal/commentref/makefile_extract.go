package commentref

import "strings"

// ExtractMakefile returns every comment in a build-recipe file, in make's
// own grammar: an unescaped `#` through the end of its line, wherever the
// marker sits, recipe lines included. A `\#` is a literal hash, per make's
// own escaping rule, and is never reported.
func ExtractMakefile(src []byte) ([]Comment, error) {
	lines := strings.Split(string(src), "\n")
	out := make([]Comment, 0, len(lines))
	for i, l := range lines {
		idx := findMakefileHash(l)
		if idx == -1 {
			continue
		}
		out = append(out, Comment{Line: i + 1, Text: strings.TrimPrefix(l[idx+1:], " ")})
	}
	return out, nil
}

// findMakefileHash returns the index of the first unescaped `#` in l, or
// -1 if the line carries none.
func findMakefileHash(l string) int {
	for i := 0; i < len(l); i++ {
		if l[i] == '\\' {
			i++
			continue
		}
		if l[i] == '#' {
			return i
		}
	}
	return -1
}
