package commentref

import (
	"errors"
	"fmt"
	"io"
	"strings"

	yaml "go.yaml.in/yaml/v3"
)

// ErrYAMLCommentUnreconciled reports that a YAML comment the parser
// attached to a node could not be matched back to its own source line. The
// gate treats this as an instrument failure rather than a guess, because a
// silently wrong line is worse than a refusal to run.
var ErrYAMLCommentUnreconciled = errors.New("yaml comment did not reconcile to a source line")

// ExtractYAML returns every comment in a YAML file, across every document
// of the stream. The underlying parser
// reports a comment on the node it attaches to, and only a same-line
// comment shares that node's own line; a comment written above or below its
// node is recovered by matching the retained text back against the source,
// nearest match first, never by a fixed offset from the node's line.
func ExtractYAML(src []byte) ([]Comment, error) {
	if len(strings.TrimSpace(string(src))) == 0 {
		return nil, nil
	}

	var out []Comment
	for _, span := range splitYAMLDocuments(strings.Split(string(src), "\n")) {
		cs, err := extractYAMLDocument(span)
		if err != nil {
			return nil, err
		}
		out = append(out, cs...)
	}
	return out, nil
}

// yamlDocSpan is one document of a stream together with the 0-based index
// of its first line in the whole source, so a comment reconciled inside the
// span can be reported at its real line.
type yamlDocSpan struct {
	offset int
	lines  []string
}

// splitYAMLDocuments cuts the source at the document markers that may only
// appear unindented, so each document reconciles against its own lines. A
// comment written before the first marker is attached by the parser to the
// following document, and its lines are not contiguous with that document's
// own — reconciling against the whole source therefore fails on a legal file.
func splitYAMLDocuments(lines []string) []yamlDocSpan {
	var spans []yamlDocSpan
	start := 0
	for i, l := range lines {
		if i > start && isYAMLDocMarker(l) {
			spans = append(spans, yamlDocSpan{offset: start, lines: lines[start:i]})
			start = i
		}
	}
	return append(spans, yamlDocSpan{offset: start, lines: lines[start:]})
}

// isYAMLDocMarker reports whether the line is an unindented document start
// or end marker.
func isYAMLDocMarker(l string) bool {
	t := strings.TrimRight(l, " \t\r")
	return t == "---" || t == "..." || strings.HasPrefix(t, "--- ")
}

// extractYAMLDocument returns the comments of one document. A span the
// parser yields no node for cannot contain a block scalar, because a block
// scalar needs a key to hang from, so every marker line in it is a comment
// and is read directly rather than reported as an instrument failure.
func extractYAMLDocument(span yamlDocSpan) ([]Comment, error) {
	text := strings.Join(span.lines, "\n")
	if strings.TrimSpace(text) == "" {
		return nil, nil
	}

	var doc yaml.Node
	err := yaml.NewDecoder(strings.NewReader(text)).Decode(&doc)
	switch {
	case errors.Is(err, io.EOF):
		return extractYAMLNodeless(span), nil
	case err != nil:
		return nil, fmt.Errorf("yaml source: %w", err)
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
			cs, err := reconcileYAMLComment(block.pos, n.Line, block.text, span.lines)
			if err != nil {
				walkErr = err
				return
			}
			for _, c := range cs {
				out = append(out, Comment{Line: c.Line + span.offset, Text: c.Text})
			}
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

// extractYAMLNodeless reads the comments of a span holding no node.
func extractYAMLNodeless(span yamlDocSpan) []Comment {
	var out []Comment
	for i, l := range span.lines {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, "#") {
			out = append(out, Comment{Line: span.offset + i + 1, Text: stripYAMLMarker(t)})
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
