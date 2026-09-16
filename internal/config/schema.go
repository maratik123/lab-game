package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"go.yaml.in/yaml/v3"
)

// leafKind identifies which YAML node shape a schema leaf's binder
// accepts. Most keys bind a scalar; a leaf may also bind a whole sequence
// (a list, such as the bestiary) or a whole mapping (an open map whose own
// keys are authored content the schema does not declare, such as the
// lexicon) node — the walk still stops descending at that leaf, handing
// the entire subtree to the leaf's own binder.
type leafKind int

// The three leaf kinds. leafScalar is the zero value, so every existing
// scalar schema entry needs no change to declare it.
const (
	leafScalar leafKind = iota
	leafSequence
	leafMap
)

// schemaEntry is one leaf of a document schema: a dotted key path, the
// YAML node shape its binder accepts, and a binder that validates the
// node's YAML tag, decodes it into the project's Go destination, and
// checks the project's own predicate for that key.
type schemaEntry struct {
	path []string
	kind leafKind
	bind func(n *yaml.Node) error
}

// schemaNode is one level of the tree built from the flat schemaEntry list
// (buildSchemaTree), so the walk can match it against the document's
// mapping nodes segment by segment. order preserves the schema's own
// declaration order, since map iteration order is never used for anything
// observable.
type schemaNode struct {
	entry    *schemaEntry
	children map[string]*schemaNode
	order    []string
}

// tagInt and tagFloat are the YAML scalar tags every integer/decimal
// binder in this package checks against, named once so goconst has one
// declaration to point repeated uses at rather than a literal per binder.
const (
	tagInt   = "!!int"
	tagFloat = "!!float"
)

// bindInt returns a binder that accepts only a "!!int" node (rejecting a
// "!!null" node and a truncating "!!float" node alike), decodes it into
// dst, and checks predicate.
func bindInt(dst *int, predicate func(int) bool, want string) func(*yaml.Node) error {
	return func(n *yaml.Node) error {
		if n.Tag != tagInt {
			return fmt.Errorf("must be an integer, got %s", n.Tag)
		}
		var v int
		if err := n.Decode(&v); err != nil {
			return fmt.Errorf("decode int: %w", err)
		}
		if !predicate(v) {
			return fmt.Errorf("must be %s, got %d", want, v)
		}
		*dst = v
		return nil
	}
}

// bindDuration returns a binder that accepts only a "!!str" node — a bare
// "!!int" is rejected rather than silently becoming nanoseconds — and
// checks predicate on the decoded value.
func bindDuration(dst *time.Duration, predicate func(time.Duration) bool, want string) func(*yaml.Node) error {
	return func(n *yaml.Node) error {
		if n.Tag != "!!str" {
			return fmt.Errorf("must be a duration string, got %s", n.Tag)
		}
		var v time.Duration
		if err := n.Decode(&v); err != nil {
			return fmt.Errorf("decode duration: %w", err)
		}
		if !predicate(v) {
			return fmt.Errorf("must be %s, got %s", want, v)
		}
		*dst = v
		return nil
	}
}

// bindDecimal returns a binder that accepts a "!!int" or "!!float" node —
// rejecting a quoted "!!str" number, so every number has one spelling —
// and checks predicate on the decoded value.
func bindDecimal(dst *decimal.Decimal, predicate func(decimal.Decimal) bool, want string) func(*yaml.Node) error {
	return func(n *yaml.Node) error {
		if n.Tag != tagInt && n.Tag != tagFloat {
			return fmt.Errorf("must be a number, got %s", n.Tag)
		}
		var v decimal.Decimal
		if err := n.Decode(&v); err != nil {
			return fmt.Errorf("decode decimal: %w", err)
		}
		if !predicate(v) {
			return fmt.Errorf("must be %s, got %s", want, v)
		}
		*dst = v
		return nil
	}
}

// buildSchemaTree groups the flat schema entry list into a tree keyed by
// path segment, so walkNode can match it against the document's mapping
// nodes one segment at a time.
func buildSchemaTree(entries []schemaEntry) *schemaNode {
	root := &schemaNode{children: map[string]*schemaNode{}}
	for i := range entries {
		e := &entries[i]
		cur := root
		for _, seg := range e.path[:len(e.path)-1] {
			next, ok := cur.children[seg]
			if !ok {
				next = &schemaNode{children: map[string]*schemaNode{}}
				cur.children[seg] = next
				cur.order = append(cur.order, seg)
			}
			cur = next
		}
		last := e.path[len(e.path)-1]
		cur.children[last] = &schemaNode{entry: e}
		cur.order = append(cur.order, last)
	}
	return root
}

// pathKey renders a schema path for a *KeyError. An empty path (the
// document root itself) renders as "<root>" rather than the empty string,
// so a root-level failure still names something in its message.
func pathKey(path []string) string {
	if len(path) == 0 {
		return "<root>"
	}
	return strings.Join(path, ".")
}

// nodeKindName renders a yaml.Kind for an error message.
func nodeKindName(k yaml.Kind) string {
	switch k {
	case yaml.DocumentNode:
		return "document"
	case yaml.SequenceNode:
		return "sequence"
	case yaml.MappingNode:
		return "mapping"
	case yaml.ScalarNode:
		return "scalar"
	case yaml.AliasNode:
		return "alias"
	default:
		return fmt.Sprintf("kind(%d)", k)
	}
}

// walkNode dispatches to walkLeaf or walkInterior depending on whether node
// is a schema leaf or an interior (mapping) level. noun is the schema's own
// name for the unknown-key message ("balance", "world").
func walkNode(doc *yaml.Node, node *schemaNode, path []string, noun string) []error {
	if node.entry != nil {
		return walkLeaf(doc, node.entry)
	}
	return walkInterior(doc, node, path, noun)
}

// walkLeaf validates and binds one schema entry against its declared leaf
// kind. doc is nil when the key is absent at this path.
func walkLeaf(doc *yaml.Node, e *schemaEntry) []error {
	key := pathKey(e.path)
	if doc == nil {
		return []error{keyErrorf(key, ErrMissing, "required")}
	}
	if doc.Kind == yaml.AliasNode {
		return []error{keyErrorf(key, ErrInvalidValue, "must not be an alias")}
	}
	switch e.kind {
	case leafSequence:
		if doc.Kind != yaml.SequenceNode {
			return []error{keyErrorf(key, ErrInvalidValue, "must be a sequence, got %s", nodeKindName(doc.Kind))}
		}
	case leafMap:
		if doc.Kind != yaml.MappingNode {
			return []error{keyErrorf(key, ErrInvalidValue, "must be a mapping, got %s", nodeKindName(doc.Kind))}
		}
	default: // leafScalar
		if doc.Kind == yaml.MappingNode || doc.Kind == yaml.SequenceNode {
			return []error{keyErrorf(key, ErrInvalidValue, "must be a scalar, got %s", nodeKindName(doc.Kind))}
		}
	}
	if err := e.bind(doc); err != nil {
		return []error{keyErrorf(key, ErrInvalidValue, "%s", err)}
	}
	return nil
}

// walkInterior validates one mapping-level schema node against doc (nil when
// the whole subtree is absent, in which case every leaf beneath it reports
// missing — this is how an "everything missing" report falls out of the
// same code path as a normal partial mapping), reports every key doc
// carries that the schema does not declare — naming the cause as an
// unrecognised key of noun's own schema — and recurses into every schema
// child in declaration order.
func walkInterior(doc *yaml.Node, node *schemaNode, path []string, noun string) []error {
	var errs []error
	if doc != nil {
		switch doc.Kind {
		case yaml.AliasNode:
			return []error{keyErrorf(pathKey(path), ErrInvalidValue, "must not be an alias")}
		case yaml.MappingNode:
			// fall through to the walk below
		default:
			return []error{keyErrorf(pathKey(path), ErrInvalidValue, "must be a mapping, got %s", nodeKindName(doc.Kind))}
		}
	}

	valuesByKey := map[string]*yaml.Node{}
	if doc != nil {
		for i := 0; i+1 < len(doc.Content); i += 2 {
			k := doc.Content[i].Value
			valuesByKey[k] = doc.Content[i+1]
			if _, known := node.children[k]; !known {
				childPath := append(append([]string{}, path...), k)
				errs = append(errs, keyErrorf(pathKey(childPath), ErrUnknownKey, "not a recognised %s key", noun))
			}
		}
	}

	for _, k := range node.order {
		child := node.children[k]
		childPath := append(append([]string{}, path...), k)
		errs = append(errs, walkNode(valuesByKey[k], child, childPath, noun)...)
	}
	return errs
}

// checkDuplicateKeys unmarshals data into a map, which — unlike unmarshalling
// into a yaml.Node — reports a duplicated mapping key at any depth. A
// duplicate is returned as the library's own error, unmodified; every other
// unmarshal failure (a syntax error, a non-mapping root) is left for the
// node-based parse to report with better context, so it is deliberately
// swallowed here.
func checkDuplicateKeys(data []byte) error {
	var m map[string]any
	if err := yaml.Unmarshal(data, &m); err != nil && strings.Contains(err.Error(), "already defined") {
		return fmt.Errorf("config: duplicate key: %w", err)
	}
	return nil
}
