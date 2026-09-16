package config

import (
	"errors"
	"strings"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/maratik123/lab-game/internal/maze"
)

// validWorldScalarYAML is the known-good baseline every negative case in
// this file mutates: one documented change per case. It carries only the
// scalar half this schema declares — the content section (the resource
// profile, the naming style, the lexicon, the bestiary) is not yet
// declared, so it is deliberately absent here.
const validWorldScalarYAML = `id: cotton_candy
seed: 42
k: 1
generation:
  radius: 9
  island_share: 0.2
  extra_passage_share: 0.1
  growing_tree_bias: 0.5
  portal_share_lower: 0.2
  portal_share_upper: 0.6
  weights:
    backtracker: 1
    kruskal: 1
    prim: 1
    growing_tree: 1
    wilson_walk: 1
`

// decodeWorldScalar walks contents against worldScalarSchema alone,
// returning the populated World and every error the walk collected.
func decodeWorldScalar(t *testing.T, contents string) (*World, error) {
	t.Helper()
	root := parseNode(t, contents)
	w := &World{}
	tree := buildSchemaTree(worldScalarSchema(w))
	errs := walkNode(root, tree, nil, "world")
	if len(errs) == 0 {
		return w, nil
	}
	return w, errors.Join(errs...)
}

func TestWorldScalarSchema_HappyPath(t *testing.T) {
	t.Parallel()
	w, err := decodeWorldScalar(t, validWorldScalarYAML)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if w.ID != "cotton_candy" {
		t.Errorf("ID = %q, want cotton_candy", w.ID)
	}
	if w.Seed != 42 {
		t.Errorf("Seed = %d, want 42", w.Seed)
	}
	if w.GateSpacing != 1 {
		t.Errorf("GateSpacing = %d, want 1", w.GateSpacing)
	}
	wantWeights := maze.AlgorithmWeights{1, 1, 1, 1, 1}
	if w.Generation.Radius != 9 {
		t.Errorf("Generation.Radius = %d, want 9", w.Generation.Radius)
	}
	if w.Generation.Weights != wantWeights {
		t.Errorf("Generation.Weights = %v, want %v", w.Generation.Weights, wantWeights)
	}
	for _, pair := range []struct {
		name string
		got  decimal.Decimal
		want string
	}{
		{"IslandShare", w.Generation.IslandShare, "0.2"},
		{"ExtraPassageShare", w.Generation.ExtraPassageShare, "0.1"},
		{"GrowingTreeBias", w.Generation.GrowingTreeBias, "0.5"},
		{"PortalShareLower", w.Generation.PortalShareLower, "0.2"},
		{"PortalShareUpper", w.Generation.PortalShareUpper, "0.6"},
	} {
		want, decErr := decimal.NewFromString(pair.want)
		if decErr != nil {
			t.Fatalf("fixture: %v", decErr)
		}
		if !pair.got.Equal(want) {
			t.Errorf("%s = %s, want %s", pair.name, pair.got, want)
		}
	}
}

func TestWorldScalarSchema_MissingKeys(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"id":                             "id: cotton_candy\n",
		"seed":                           "seed: 42\n",
		"k":                              "k: 1\n",
		"generation.radius":              "  radius: 9\n",
		"generation.weights.backtracker": "    backtracker: 1\n",
	}
	for key, old := range cases {
		t.Run(key, func(t *testing.T) {
			t.Parallel()
			yaml := strings.Replace(validWorldScalarYAML, old, "", 1)
			if yaml == validWorldScalarYAML {
				t.Fatalf("fixture line %q not found", old)
			}
			_, err := decodeWorldScalar(t, yaml)
			assertKeyError(t, err, ErrMissing, key)
		})
	}
}

func TestWorldScalarSchema_UnknownKey(t *testing.T) {
	t.Parallel()
	t.Run("top_level", func(t *testing.T) {
		t.Parallel()
		yaml := validWorldScalarYAML + "bogus: 1\n"
		_, err := decodeWorldScalar(t, yaml)
		assertKeyError(t, err, ErrUnknownKey, "bogus")
	})
	t.Run("unrecognised_algorithm_weight", func(t *testing.T) {
		t.Parallel()
		yaml := strings.Replace(validWorldScalarYAML, "    backtracker: 1\n", "    backtracker: 1\n    bogus_algo: 1\n", 1)
		_, err := decodeWorldScalar(t, yaml)
		assertKeyError(t, err, ErrUnknownKey, "generation.weights.bogus_algo")
	})
}

func TestWorldScalarSchema_RadiusBelowMinStatesTheBound(t *testing.T) {
	t.Parallel()
	yaml := strings.Replace(validWorldScalarYAML, "  radius: 9\n", "  radius: 5\n", 1)
	_, err := decodeWorldScalar(t, yaml)
	assertKeyError(t, err, ErrInvalidValue, "generation.radius")
	if strings.Contains(err.Error(), "positive") {
		t.Errorf("error %q says \"positive\", must state the actual bound", err)
	}
	if !strings.Contains(err.Error(), "6") {
		t.Errorf("error %q does not state the bound 6", err)
	}
}

func TestWorldScalarSchema_RadiusAboveInt32IsRefusedByTheDecoder(t *testing.T) {
	t.Parallel()
	yaml := strings.Replace(validWorldScalarYAML, "  radius: 9\n", "  radius: 2147483648\n", 1)
	_, err := decodeWorldScalar(t, yaml)
	assertKeyError(t, err, ErrInvalidValue, "generation.radius")
}

func TestWorldScalarSchema_SharesOutsideUnitInterval(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"generation.island_share":        "island_share: 0.2\n",
		"generation.extra_passage_share": "extra_passage_share: 0.1\n",
		"generation.growing_tree_bias":   "growing_tree_bias: 0.5\n",
		"generation.portal_share_lower":  "portal_share_lower: 0.2\n",
		"generation.portal_share_upper":  "portal_share_upper: 0.6\n",
	}
	for key, old := range cases {
		t.Run(key, func(t *testing.T) {
			t.Parallel()
			name := strings.SplitN(old, ":", 2)[0]
			newLine := name + ": 1.5\n"
			yaml := strings.Replace(validWorldScalarYAML, old, newLine, 1)
			if yaml == validWorldScalarYAML {
				t.Fatalf("fixture line %q not found", old)
			}
			_, err := decodeWorldScalar(t, yaml)
			assertKeyError(t, err, ErrInvalidValue, key)
		})
	}
}

func TestWorldScalarSchema_NegativeGateSpacing(t *testing.T) {
	t.Parallel()
	yaml := strings.Replace(validWorldScalarYAML, "k: 1\n", "k: -1\n", 1)
	_, err := decodeWorldScalar(t, yaml)
	assertKeyError(t, err, ErrInvalidValue, "k")
}

func TestWorldScalarSchema_NonIntegerSeed(t *testing.T) {
	t.Parallel()
	yaml := strings.Replace(validWorldScalarYAML, "seed: 42\n", "seed: 42.5\n", 1)
	_, err := decodeWorldScalar(t, yaml)
	assertKeyError(t, err, ErrInvalidValue, "seed")
}

func TestWorldScalarSchema_WeightBelowZero(t *testing.T) {
	t.Parallel()
	yaml := strings.Replace(validWorldScalarYAML, "    backtracker: 1\n", "    backtracker: -1\n", 1)
	_, err := decodeWorldScalar(t, yaml)
	assertKeyError(t, err, ErrInvalidValue, "generation.weights.backtracker")
}

func TestWorldScalarSchema_RadiusIsFloat(t *testing.T) {
	t.Parallel()
	yaml := strings.Replace(validWorldScalarYAML, "  radius: 9\n", "  radius: 9.5\n", 1)
	_, err := decodeWorldScalar(t, yaml)
	assertKeyError(t, err, ErrInvalidValue, "generation.radius")
}

func TestWorldScalarSchema_WeightIsFloat(t *testing.T) {
	t.Parallel()
	yaml := strings.Replace(validWorldScalarYAML, "    backtracker: 1\n", "    backtracker: 1.5\n", 1)
	_, err := decodeWorldScalar(t, yaml)
	assertKeyError(t, err, ErrInvalidValue, "generation.weights.backtracker")
}

// TestWorldScalarSchema_WeightKeyAgreement asserts that every
// generation.weights.* entry's last path segment equals that algorithm's
// own rendered name, over every exported Algorithm constant — so a weight
// key never drifts from the spelling the generator itself renders.
func TestWorldScalarSchema_WeightKeyAgreement(t *testing.T) {
	t.Parallel()
	w := &World{}
	entries := worldScalarSchema(w)
	got := map[string]bool{}
	for _, e := range entries {
		if len(e.path) == 3 && e.path[0] == "generation" && e.path[1] == "weights" {
			got[e.path[2]] = true
		}
	}
	for a := maze.AlgorithmBacktracker; a <= maze.AlgorithmWilson; a++ {
		if !got[a.String()] {
			t.Errorf("no generation.weights schema entry for algorithm %s", a.String())
		}
		delete(got, a.String())
	}
	if len(got) != 0 {
		t.Errorf("extra generation.weights schema entries: %v", got)
	}
}
