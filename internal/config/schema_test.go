package config

import (
	"errors"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// parseNode parses contents as a single YAML document and returns its root
// content node (nil for an empty document), the shape every walkNode entry
// point receives.
func parseNode(t *testing.T, contents string) *yaml.Node {
	t.Helper()
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(contents), &doc); err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	if doc.IsZero() || len(doc.Content) == 0 {
		return nil
	}
	return doc.Content[0]
}

func noopBind(*yaml.Node) error { return nil }

func TestWalkNode_ScalarLeaf_RefusesMappingAndSequence(t *testing.T) {
	t.Parallel()
	tree := buildSchemaTree([]schemaEntry{{path: []string{"k"}, kind: leafScalar, bind: noopBind}})

	t.Run("mapping", func(t *testing.T) {
		t.Parallel()
		errs := walkNode(parseNode(t, "k:\n  a: 1\n"), tree, nil, "test")
		assertOneKeyError(t, errs, ErrInvalidValue, "k", "must be a scalar, got mapping")
	})
	t.Run("sequence", func(t *testing.T) {
		t.Parallel()
		errs := walkNode(parseNode(t, "k:\n  - 1\n"), tree, nil, "test")
		assertOneKeyError(t, errs, ErrInvalidValue, "k", "must be a scalar, got sequence")
	})
}

func TestWalkNode_UnknownKeyCause_IsPerSchemaNoun(t *testing.T) {
	t.Parallel()
	tree := buildSchemaTree([]schemaEntry{{path: []string{"known"}, kind: leafScalar, bind: noopBind}})

	for _, noun := range []string{"balance", "world"} {
		t.Run(noun, func(t *testing.T) {
			t.Parallel()
			errs := walkNode(parseNode(t, "known: 1\nbogus: 1\n"), tree, nil, noun)
			assertOneKeyError(t, errs, ErrUnknownKey, "bogus", "not a recognised "+noun+" key")
		})
	}
}

func TestWalkNode_SequenceLeaf(t *testing.T) {
	t.Parallel()
	tree := buildSchemaTree([]schemaEntry{{path: []string{"k"}, kind: leafSequence, bind: noopBind}})

	t.Run("binds_a_sequence", func(t *testing.T) {
		t.Parallel()
		errs := walkNode(parseNode(t, "k:\n  - 1\n  - 2\n"), tree, nil, "test")
		if len(errs) != 0 {
			t.Fatalf("unexpected errors: %v", errs)
		}
	})
	t.Run("refuses_a_scalar", func(t *testing.T) {
		t.Parallel()
		errs := walkNode(parseNode(t, "k: 1\n"), tree, nil, "test")
		assertOneKeyError(t, errs, ErrInvalidValue, "k", "must be a sequence, got scalar")
	})
	t.Run("refuses_a_mapping", func(t *testing.T) {
		t.Parallel()
		errs := walkNode(parseNode(t, "k:\n  a: 1\n"), tree, nil, "test")
		assertOneKeyError(t, errs, ErrInvalidValue, "k", "must be a sequence, got mapping")
	})
}

func TestWalkNode_MapLeaf(t *testing.T) {
	t.Parallel()
	tree := buildSchemaTree([]schemaEntry{{path: []string{"k"}, kind: leafMap, bind: noopBind}})

	t.Run("binds_a_mapping", func(t *testing.T) {
		t.Parallel()
		errs := walkNode(parseNode(t, "k:\n  a: 1\n"), tree, nil, "test")
		if len(errs) != 0 {
			t.Fatalf("unexpected errors: %v", errs)
		}
	})
	t.Run("refuses_a_scalar", func(t *testing.T) {
		t.Parallel()
		errs := walkNode(parseNode(t, "k: 1\n"), tree, nil, "test")
		assertOneKeyError(t, errs, ErrInvalidValue, "k", "must be a mapping, got scalar")
	})
	t.Run("refuses_a_sequence", func(t *testing.T) {
		t.Parallel()
		errs := walkNode(parseNode(t, "k:\n  - 1\n"), tree, nil, "test")
		assertOneKeyError(t, errs, ErrInvalidValue, "k", "must be a mapping, got sequence")
	})
}

// assertOneKeyError asserts errs holds exactly one *KeyError whose sentinel
// is sentinel, whose Key is wantKey, and whose message contains wantSubstr.
func assertOneKeyError(t *testing.T, errs []error, sentinel error, wantKey, wantSubstr string) {
	t.Helper()
	if len(errs) != 1 {
		t.Fatalf("errs = %v, want exactly one", errs)
	}
	var kerr *KeyError
	if !errors.As(errs[0], &kerr) {
		t.Fatalf("errs[0] = %T, want *KeyError", errs[0])
	}
	if kerr.Key != wantKey {
		t.Errorf("Key = %q, want %q", kerr.Key, wantKey)
	}
	if !errors.Is(kerr, sentinel) {
		t.Errorf("error %v does not wrap sentinel %v", kerr, sentinel)
	}
	if !strings.Contains(kerr.Error(), wantSubstr) {
		t.Errorf("error %q does not contain %q", kerr.Error(), wantSubstr)
	}
}
