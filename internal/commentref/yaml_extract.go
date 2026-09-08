package commentref

import (
	"errors"
	"fmt"
	"strings"

	yaml "go.yaml.in/yaml/v3"
)

// ErrYAMLCommentUnreconciled reports that a YAML comment the parser
// attached to a node could not be matched back to its own source line. The
// gate treats this as an instrument failure rather than a guess, because a
// silently wrong line is worse than a refusal to run.
var ErrYAMLCommentUnreconciled = errors.New("yaml comment did not reconcile to a source line")

// ErrYAMLUnsupportedStream reports a YAML source carrying a document
// marker or a directive. The extractor reads a single plain document; the
// gate refuses anything else rather than reporting a file it cannot place
// comments in, because a guard that guesses is worse than one that stops.
var ErrYAMLUnsupportedStream = errors.New("yaml source is not a single plain document")

// ExtractYAML returns every comment in a YAML file holding one plain
// document, and refuses any other stream shape with ErrYAMLUnsupportedStream.
// The underlying parser reports a comment on the node it attaches to, and
// only a same-line comment shares that node's own line; a comment written
// above or below its node is recovered by matching the retained text back
// against the source, nearest match first, never by a fixed offset from the
// node's line. A source the parser yields no node for holds no key, so it
// holds no block scalar, and its comments are read directly.
func ExtractYAML(src []byte) ([]Comment, error) {
	if len(strings.TrimSpace(string(src))) == 0 {
		return nil, nil
	}

	lines := strings.Split(string(src), "\n")
	if n, marker := findYAMLStreamMarker(lines); n > 0 {
		return nil, fmt.Errorf("%w: line %d begins %q", ErrYAMLUnsupportedStream, n, marker)
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(src, &doc); err != nil {
		return nil, fmt.Errorf("yaml source: %w", err)
	}
	if doc.Kind == 0 {
		return extractYAMLNodeless(lines), nil
	}

	var out []Comment
	var walkErr error
	var walk func(n *yaml.Node)
	walk = func(n *yaml.Node) {
		if n == nil || walkErr != nil {
			return
		}
		for _, block := range []struct {
			text string
			pos  yamlCommentPos
		}{
			{n.HeadComment, yamlHead},
			{n.LineComment, yamlLine},
			{n.FootComment, yamlFoot},
		} {
			if block.text == "" {
				continue
			}
			cs, err := reconcileYAMLComment(block.pos, n.Line, block.text, lines)
			if err != nil {
				walkErr = err
				return
			}
			out = append(out, cs...)
		}
		for _, c := range n.Content {
			walk(c)
			if walkErr != nil {
				return
			}
		}
	}
	walk(&doc)
	if walkErr != nil {
		return nil, walkErr
	}
	return out, nil
}

// findYAMLStreamMarker returns the 1-based line number and the leading text
// of the first unindented document marker or directive, or zero when the
// source is a single plain document. A marker may only appear unindented, so
// a column-zero match cannot be block-scalar content.
func findYAMLStreamMarker(lines []string) (int, string) {
	for i, l := range lines {
		t := strings.TrimRight(l, " \t\r")
		switch {
		case t == "---" || t == "..." || strings.HasPrefix(t, "--- ") || strings.HasPrefix(t, "... "):
			return i + 1, strings.SplitN(t, " ", 2)[0]
		case strings.HasPrefix(t, "%"):
			return i + 1, strings.SplitN(t, " ", 2)[0]
		}
	}
	return 0, ""
}

// extractYAMLNodeless reads the comments of a source the parser yields no
// node for. Such a source holds no key, so it holds no block scalar, and
// every marker line in it is a comment.
func extractYAMLNodeless(lines []string) []Comment {
	var out []Comment
	for i, l := range lines {
		if t := strings.TrimSpace(l); strings.HasPrefix(t, "#") {
			out = append(out, Comment{Line: i + 1, Text: stripYAMLMarker(t)})
		}
	}
	return out
}

// yamlCommentPos names which of a node's three comment fields is being
// reconciled, since each searches the source in a different direction.
type yamlCommentPos int

const (
	yamlHead yamlCommentPos = iota
	yamlLine
	yamlFoot
)

// reconcileYAMLComment recovers the real source line(s) of one node
// comment. A line comment shares its node's own line. A head or foot
// comment is a run of consecutive lines whose whitespace-trimmed content
// equals the retained text, one for one, searched nearest-first above
// (head) or below (foot) the node's line — never assumed to be adjacent,
// since the parser attaches a head comment across an intervening blank
// line.
func reconcileYAMLComment(pos yamlCommentPos, nodeLine int, raw string, lines []string) ([]Comment, error) {
	rawLines := strings.Split(raw, "\n")

	if pos == yamlLine {
		if nodeLine < 1 || nodeLine > len(lines) {
			return nil, fmt.Errorf("%w: line comment at reported line %d is out of range", ErrYAMLCommentUnreconciled, nodeLine)
		}
		if !strings.HasSuffix(strings.TrimRight(lines[nodeLine-1], " \t\r"), strings.TrimSpace(rawLines[0])) {
			return nil, fmt.Errorf("%w: line comment does not match source line %d", ErrYAMLCommentUnreconciled, nodeLine)
		}
		return []Comment{{Line: nodeLine, Text: stripYAMLMarker(rawLines[0])}}, nil
	}

	want := make([]string, len(rawLines))
	for i, l := range rawLines {
		want[i] = strings.TrimSpace(l)
	}
	k := len(want)

	start, ok := searchYAMLBlock(pos, nodeLine, want, lines)
	if !ok {
		return nil, fmt.Errorf("%w: block comment near line %d has no matching source run", ErrYAMLCommentUnreconciled, nodeLine)
	}

	out := make([]Comment, 0, k)
	for i := 0; i < k; i++ {
		out = append(out, Comment{Line: start + i, Text: stripYAMLMarker(rawLines[i])})
	}
	return out, nil
}

// searchYAMLBlock finds the nearest run of len(want) consecutive lines
// whose whitespace-trimmed content equals want, one for one, searching
// upward from nodeLine for a head comment and downward for a foot comment.
// It returns the run's one-based start line.
func searchYAMLBlock(pos yamlCommentPos, nodeLine int, want []string, lines []string) (int, bool) {
	k := len(want)
	blockMatches := func(start int) bool {
		for i := 0; i < k; i++ {
			idx := start - 1 + i
			if idx < 0 || idx >= len(lines) {
				return false
			}
			if strings.TrimSpace(lines[idx]) != want[i] {
				return false
			}
		}
		return true
	}

	switch pos {
	case yamlHead:
		for end := nodeLine - 1; end >= k; end-- {
			start := end - k + 1
			if blockMatches(start) {
				return start, true
			}
		}
	case yamlFoot:
		for start := nodeLine + 1; start+k-1 <= len(lines); start++ {
			if blockMatches(start) {
				return start, true
			}
		}
	case yamlLine:
		// A same-line comment is reconciled by reconcileYAMLComment
		// directly, never by a block search; this case exists only so the
		// switch is total over yamlCommentPos.
	}
	return 0, false
}

// stripYAMLMarker removes the "#" comment marker and one leading space from
// a single retained comment line.
func stripYAMLMarker(line string) string {
	trimmed := strings.TrimPrefix(strings.TrimLeft(line, " \t"), "#")
	return strings.TrimPrefix(trimmed, " ")
}
