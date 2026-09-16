package config

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/shopspring/decimal"
	"go.yaml.in/yaml/v3"
)

// ResourceEntry is one line of a world's resource profile: a resource kind
// and its draw weight.
type ResourceEntry struct {
	Kind   string
	Weight decimal.Decimal
}

// NamingStyle is a world's location-naming vocabulary: a non-empty list of
// templates, and an open map of slot name to non-empty phrase list. Every
// template's slot placeholder must be defined in Parts, and every Parts
// slot must be referenced by at least one template — checkNamingStyleSlots
// is that cross-check.
type NamingStyle struct {
	Templates []string
	Parts     map[string][]string
}

// BestiaryEntry is one monster this world's raids can present: a unique
// id, a display name, and its role.
type BestiaryEntry struct {
	ID   string
	Name string
	Role string
}

// Bestiary roles: the tiers a world's monster roster distinguishes.
const (
	RoleCommon    = "common"
	RoleDangerous = "dangerous"
	RoleBoss      = "boss"
)

// slotPlaceholder matches a "{slot}" template placeholder.
var slotPlaceholder = regexp.MustCompile(`\{([a-z0-9_]+)\}`)

// closedMappingFields returns n's mapping fields keyed by name, refusing
// any key not present in allowed and any node that is not itself a
// mapping — every mapping in a world document is closed, and this is the
// compound-leaf binders' own instance of that rule, since the schema walk
// itself only closes the keys it knows about at the interior level.
func closedMappingFields(n *yaml.Node, allowed ...string) (map[string]*yaml.Node, error) {
	if n.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("must be a mapping, got %s", nodeKindName(n.Kind))
	}
	allowedSet := make(map[string]bool, len(allowed))
	for _, a := range allowed {
		allowedSet[a] = true
	}
	fields := map[string]*yaml.Node{}
	for i := 0; i+1 < len(n.Content); i += 2 {
		k := n.Content[i].Value
		if !allowedSet[k] {
			return nil, fmt.Errorf("unrecognised key %q", k)
		}
		fields[k] = n.Content[i+1]
	}
	return fields, nil
}

// requireField returns fields[key], or an error naming it missing.
func requireField(fields map[string]*yaml.Node, key string) (*yaml.Node, error) {
	v, ok := fields[key]
	if !ok {
		return nil, fmt.Errorf("missing %s", key)
	}
	return v, nil
}

// decodeTaggedString decodes n as a "!!str" node, refusing any other tag.
func decodeTaggedString(n *yaml.Node) (string, error) {
	if n.Tag != tagStr {
		return "", fmt.Errorf("must be a string, got %s", n.Tag)
	}
	var v string
	if err := n.Decode(&v); err != nil {
		return "", fmt.Errorf("decode string: %w", err)
	}
	return v, nil
}

// bindResourceProfile returns a binder for a non-empty sequence of
// {kind, weight} mappings: kind a lower_snake_case token, weight a
// positive number, no kind repeated.
func bindResourceProfile(dst *[]ResourceEntry) func(*yaml.Node) error {
	return func(n *yaml.Node) error {
		if len(n.Content) == 0 {
			return fmt.Errorf("must not be empty")
		}
		seen := map[string]bool{}
		entries := make([]ResourceEntry, 0, len(n.Content))
		for i, el := range n.Content {
			fields, err := closedMappingFields(el, "kind", "weight")
			if err != nil {
				return fmt.Errorf("element %d: %w", i, err)
			}
			kindNode, err := requireField(fields, "kind")
			if err != nil {
				return fmt.Errorf("element %d: %w", i, err)
			}
			kind, err := decodeTaggedString(kindNode)
			if err != nil {
				return fmt.Errorf("element %d: kind: %w", i, err)
			}
			if !lowerSnakeCaseToken.MatchString(kind) {
				return fmt.Errorf("element %d: kind must be a lower_snake_case token, got %q", i, kind)
			}
			weightNode, err := requireField(fields, "weight")
			if err != nil {
				return fmt.Errorf("element %d: %w", i, err)
			}
			if weightNode.Tag != tagInt && weightNode.Tag != tagFloat {
				return fmt.Errorf("element %d: weight must be a number, got %s", i, weightNode.Tag)
			}
			var weight decimal.Decimal
			if err := weightNode.Decode(&weight); err != nil {
				return fmt.Errorf("element %d: decode weight: %w", i, err)
			}
			if !weight.GreaterThan(decimal.Zero) {
				return fmt.Errorf("element %d: weight must be positive, got %s", i, weight)
			}
			if seen[kind] {
				return fmt.Errorf("duplicate kind %q", kind)
			}
			seen[kind] = true
			entries = append(entries, ResourceEntry{Kind: kind, Weight: weight})
		}
		*dst = entries
		return nil
	}
}

// bindPhraseList returns a binder for a non-empty sequence of non-empty
// strings.
func bindPhraseList(dst *[]string) func(*yaml.Node) error {
	return func(n *yaml.Node) error {
		if len(n.Content) == 0 {
			return fmt.Errorf("must not be empty")
		}
		out := make([]string, 0, len(n.Content))
		for i, el := range n.Content {
			s, err := decodeTaggedString(el)
			if err != nil {
				return fmt.Errorf("element %d: %w", i, err)
			}
			if s == "" {
				return fmt.Errorf("element %d: must not be empty", i)
			}
			out = append(out, s)
		}
		*dst = out
		return nil
	}
}

// bindPhraseListMap returns a binder for an open map of lower_snake_case
// key to a non-empty phrase list — the shape the lexicon and the naming
// style's parts map share — a world document's two authored-key
// exceptions to an otherwise closed mapping.
func bindPhraseListMap(dst *map[string][]string) func(*yaml.Node) error {
	return func(n *yaml.Node) error {
		out := map[string][]string{}
		for i := 0; i+1 < len(n.Content); i += 2 {
			keyNode, valNode := n.Content[i], n.Content[i+1]
			key, err := decodeTaggedString(keyNode)
			if err != nil {
				return fmt.Errorf("key: %w", err)
			}
			if !lowerSnakeCaseToken.MatchString(key) {
				return fmt.Errorf("key %q must be a lower_snake_case token", key)
			}
			var phrases []string
			if err := bindPhraseList(&phrases)(valNode); err != nil {
				return fmt.Errorf("%s: %w", key, err)
			}
			out[key] = phrases
		}
		*dst = out
		return nil
	}
}

// bindBestiary returns a binder for a non-empty sequence of
// {id, name, role} mappings: id a lower_snake_case token unique across the
// bestiary, name non-empty, role one of RoleCommon/RoleDangerous/RoleBoss.
func bindBestiary(dst *[]BestiaryEntry) func(*yaml.Node) error {
	return func(n *yaml.Node) error {
		if len(n.Content) == 0 {
			return fmt.Errorf("must not be empty")
		}
		seen := map[string]bool{}
		entries := make([]BestiaryEntry, 0, len(n.Content))
		for i, el := range n.Content {
			fields, err := closedMappingFields(el, "id", "name", "role")
			if err != nil {
				return fmt.Errorf("element %d: %w", i, err)
			}
			idNode, err := requireField(fields, "id")
			if err != nil {
				return fmt.Errorf("element %d: %w", i, err)
			}
			id, err := decodeTaggedString(idNode)
			if err != nil {
				return fmt.Errorf("element %d: id: %w", i, err)
			}
			if !lowerSnakeCaseToken.MatchString(id) {
				return fmt.Errorf("element %d: id must be a lower_snake_case token, got %q", i, id)
			}
			nameNode, err := requireField(fields, "name")
			if err != nil {
				return fmt.Errorf("element %d: %w", i, err)
			}
			name, err := decodeTaggedString(nameNode)
			if err != nil {
				return fmt.Errorf("element %d: name: %w", i, err)
			}
			if name == "" {
				return fmt.Errorf("element %d: name must not be empty", i)
			}
			roleNode, err := requireField(fields, "role")
			if err != nil {
				return fmt.Errorf("element %d: %w", i, err)
			}
			role, err := decodeTaggedString(roleNode)
			if err != nil {
				return fmt.Errorf("element %d: role: %w", i, err)
			}
			if role != RoleCommon && role != RoleDangerous && role != RoleBoss {
				return fmt.Errorf("element %d: role must be one of %s, %s, %s, got %q", i, RoleCommon, RoleDangerous, RoleBoss, role)
			}
			if seen[id] {
				return fmt.Errorf("duplicate id %q", id)
			}
			seen[id] = true
			entries = append(entries, BestiaryEntry{ID: id, Name: name, Role: role})
		}
		*dst = entries
		return nil
	}
}

// worldContentSchema returns the content half of one world document's
// key-path schema — the resource profile, the naming style, the lexicon
// and the bestiary — writing into w. Called fresh on every decode: no
// package-level state.
func worldContentSchema(w *World) []schemaEntry {
	entry := func(path string, kind leafKind, bind func(n *yaml.Node) error) schemaEntry {
		return schemaEntry{path: strings.Split(path, "."), kind: kind, bind: bind}
	}
	return []schemaEntry{
		entry("resource_profile", leafSequence, bindResourceProfile(&w.ResourceProfile)),
		entry("naming_style.templates", leafSequence, bindPhraseList(&w.NamingStyle.Templates)),
		entry("naming_style.parts", leafMap, bindPhraseListMap(&w.NamingStyle.Parts)),
		entry("lexicon", leafMap, bindPhraseListMap(&w.Lexicon)),
		entry("bestiary", leafSequence, bindBestiary(&w.Bestiary)),
	}
}

// checkNamingStyleSlots cross-validates ns after a successful content
// schema walk: every template's "{slot}" placeholder must be defined in
// Parts, and every Parts slot must be referenced by at least one
// template. Every mismatch is returned as a *KeyError naming
// naming_style.templates or naming_style.parts, in a deterministic order.
func checkNamingStyleSlots(ns NamingStyle) []error {
	var errs []error
	referenced := map[string]bool{}
	for _, tmpl := range ns.Templates {
		for _, m := range slotPlaceholder.FindAllStringSubmatch(tmpl, -1) {
			slot := m[1]
			referenced[slot] = true
			if _, ok := ns.Parts[slot]; !ok {
				errs = append(errs, keyErrorf("naming_style.templates", ErrInvalidValue,
					"template %q names slot %q, which naming_style.parts does not define", tmpl, slot))
			}
		}
	}
	unreferenced := make([]string, 0, len(ns.Parts))
	for slot := range ns.Parts {
		if !referenced[slot] {
			unreferenced = append(unreferenced, slot)
		}
	}
	sort.Strings(unreferenced)
	for _, slot := range unreferenced {
		errs = append(errs, keyErrorf("naming_style.parts", ErrInvalidValue,
			"slot %q is not named by any naming_style.templates entry", slot))
	}
	return errs
}
