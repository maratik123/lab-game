package config

import (
	"errors"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
)

// validWorldContentYAML is the known-good baseline every negative case in
// this file mutates: one documented change per case.
const validWorldContentYAML = `resource_profile:
  - kind: spun_sugar
    weight: 1
  - kind: pastel_fleece
    weight: 2
naming_style:
  templates:
    - "{adjective} {noun}"
  parts:
    adjective:
      - fluffy
      - pastel
    noun:
      - meadow
      - cloud
lexicon:
  ambient:
    - the wind smells of sugar
bestiary:
  - id: cotton_wolf
    name: Cotton Wolf
    role: common
  - id: pastel_unicorn
    name: Pastel Unicorn
    role: dangerous
`

// decodeWorldContent walks contents against worldContentSchema alone,
// returning the populated World and every error the walk collected.
func decodeWorldContent(t *testing.T, contents string) (*World, error) {
	t.Helper()
	root := parseNode(t, contents)
	w := &World{}
	tree := buildSchemaTree(worldContentSchema(w))
	errs := walkNode(root, tree, nil, "world")
	if len(errs) == 0 {
		return w, nil
	}
	return w, errors.Join(errs...)
}

func TestWorldContentSchema_HappyPath(t *testing.T) {
	t.Parallel()
	w, err := decodeWorldContent(t, validWorldContentYAML)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(w.ResourceProfile) != 2 {
		t.Fatalf("ResourceProfile = %+v, want 2 entries", w.ResourceProfile)
	}
	if w.ResourceProfile[0].Kind != "spun_sugar" || !w.ResourceProfile[0].Weight.Equal(decimal.NewFromInt(1)) {
		t.Errorf("ResourceProfile[0] = %+v", w.ResourceProfile[0])
	}
	if len(w.NamingStyle.Templates) != 1 || w.NamingStyle.Templates[0] != "{adjective} {noun}" {
		t.Errorf("NamingStyle.Templates = %v", w.NamingStyle.Templates)
	}
	if len(w.NamingStyle.Parts["adjective"]) != 2 {
		t.Errorf("NamingStyle.Parts[adjective] = %v", w.NamingStyle.Parts["adjective"])
	}
	if len(w.Lexicon["ambient"]) != 1 {
		t.Errorf("Lexicon[ambient] = %v", w.Lexicon["ambient"])
	}
	if len(w.Bestiary) != 2 || w.Bestiary[0].ID != "cotton_wolf" || w.Bestiary[0].Role != RoleCommon {
		t.Errorf("Bestiary = %+v", w.Bestiary)
	}
	if errs := checkNamingStyleSlots(w.NamingStyle); len(errs) != 0 {
		t.Errorf("checkNamingStyleSlots: unexpected errors: %v", errs)
	}
}

func TestResourceProfile_Empty(t *testing.T) {
	t.Parallel()
	yaml := strings.Replace(validWorldContentYAML,
		"resource_profile:\n  - kind: spun_sugar\n    weight: 1\n  - kind: pastel_fleece\n    weight: 2\n",
		"resource_profile: []\n", 1)
	_, err := decodeWorldContent(t, yaml)
	assertKeyError(t, err, ErrInvalidValue, "resource_profile")
}

func TestResourceProfile_NegativeWeight(t *testing.T) {
	t.Parallel()
	yaml := strings.Replace(validWorldContentYAML, "weight: 1\n", "weight: -1\n", 1)
	_, err := decodeWorldContent(t, yaml)
	assertKeyError(t, err, ErrInvalidValue, "resource_profile")
}

func TestResourceProfile_DuplicatedKind(t *testing.T) {
	t.Parallel()
	yaml := strings.Replace(validWorldContentYAML, "kind: pastel_fleece\n", "kind: spun_sugar\n", 1)
	_, err := decodeWorldContent(t, yaml)
	assertKeyError(t, err, ErrInvalidValue, "resource_profile")
}

func TestNamingStyle_TemplateNamesUndefinedSlot(t *testing.T) {
	t.Parallel()
	w, err := decodeWorldContent(t, strings.Replace(validWorldContentYAML, "\"{adjective} {noun}\"", "\"{adjective} {undefined_slot}\"", 1))
	if err != nil {
		t.Fatalf("schema walk: unexpected error: %v", err)
	}
	errs := checkNamingStyleSlots(w.NamingStyle)
	joined := errors.Join(errs...)
	assertKeyError(t, joined, ErrInvalidValue, "naming_style.templates")
}

func TestNamingStyle_PartsSlotNotReferenced(t *testing.T) {
	t.Parallel()
	yaml := strings.Replace(validWorldContentYAML, "    noun:\n      - meadow\n      - cloud\n",
		"    noun:\n      - meadow\n      - cloud\n    unused_slot:\n      - x\n", 1)
	w, err := decodeWorldContent(t, yaml)
	if err != nil {
		t.Fatalf("schema walk: unexpected error: %v", err)
	}
	errs := checkNamingStyleSlots(w.NamingStyle)
	joined := errors.Join(errs...)
	assertKeyError(t, joined, ErrInvalidValue, "naming_style.parts")
}

func TestNamingStyle_CrossCheck_ValidPairingHasNoErrors(t *testing.T) {
	t.Parallel()
	w, err := decodeWorldContent(t, validWorldContentYAML)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if errs := checkNamingStyleSlots(w.NamingStyle); len(errs) != 0 {
		t.Errorf("checkNamingStyleSlots: unexpected errors on a matched template/parts pair: %v", errs)
	}
}

func TestLexicon_EmptyPhraseList(t *testing.T) {
	t.Parallel()
	yaml := strings.Replace(validWorldContentYAML, "  ambient:\n    - the wind smells of sugar\n", "  ambient: []\n", 1)
	_, err := decodeWorldContent(t, yaml)
	assertKeyError(t, err, ErrInvalidValue, "lexicon")
}

func TestLexicon_KeyNotLowerSnakeCase(t *testing.T) {
	t.Parallel()
	yaml := strings.Replace(validWorldContentYAML, "  ambient:\n", "  Ambient:\n", 1)
	_, err := decodeWorldContent(t, yaml)
	assertKeyError(t, err, ErrInvalidValue, "lexicon")
}

func TestBestiary_DuplicatedID(t *testing.T) {
	t.Parallel()
	yaml := strings.Replace(validWorldContentYAML, "id: pastel_unicorn\n", "id: cotton_wolf\n", 1)
	_, err := decodeWorldContent(t, yaml)
	assertKeyError(t, err, ErrInvalidValue, "bestiary")
}

func TestBestiary_UnknownRole(t *testing.T) {
	t.Parallel()
	yaml := strings.Replace(validWorldContentYAML, "role: common\n", "role: legendary\n", 1)
	_, err := decodeWorldContent(t, yaml)
	assertKeyError(t, err, ErrInvalidValue, "bestiary")
}

func TestBestiary_EmptyName(t *testing.T) {
	t.Parallel()
	yaml := strings.Replace(validWorldContentYAML, "name: Cotton Wolf\n", "name: \"\"\n", 1)
	_, err := decodeWorldContent(t, yaml)
	assertKeyError(t, err, ErrInvalidValue, "bestiary")
}
