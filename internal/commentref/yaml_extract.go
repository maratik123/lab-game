package commentref

import (
	"bytes"
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

	lines := strings.Split(string(src), "\n")
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
	dec := yaml.NewDecoder(bytes.NewReader(src))
	for {
		var doc yaml.Node
		if err := dec.Decode(&doc); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("yaml source: %w", err)
		}
		walk(&doc)
		if walkErr != nil {
			return nil, walkErr
		}
	}
	return out, nil
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
