package config

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

// validBalanceYAML is the known-good baseline every negative test case
// mutates: one documented change per case, rather than a hand-typed
// near-miss with no relation to the others.
const validBalanceYAML = `world:
  chunk:
    cols: 16
    rows: 16
raid:
  stamina:
    cap: 100
    step_cost: 1
  standing:
    calm_to_noises: 5m
    noises_to_wave: 2m
    wave_to_wave: 1m
  death:
    backpack_ttl: 30m
    own_chat_head_start: 2m
    respawn_debuff: 1m
  afk:
    cruelty: 0.5
  door:
    price_base: 10
    price_per_distance: 2
    price_distance_exponent: 1
    boss_reward_ttl: 30m
  monster_budget:
    base: 5
    per_distance: 1
    distance_exponent: 1
    scaling_per_level: 0.03
combat:
  hit_die_sides: 20
  base_defence: 10
  critical_natural: 20
  fumble_natural: 1
  weapon_dice_count: 2
  weapon_die_sides: 6
  initiative_die_sides: 4
  max_rounds: 20
  vulnerability_multiplier: 1.5
  resist_multiplier: 0.5
economy:
  shop:
    sell_rate: 0.25
    buy_markup: 0.5
`

// validBalance is validBalanceYAML's expected decoded value, used by the
// happy-path assertion below.
func validBalance() *Balance {
	dec := func(s string) decimal.Decimal {
		d, err := decimal.NewFromString(s)
		if err != nil {
			panic(err)
		}
		return d
	}
	return &Balance{
		World: WorldBalance{Chunk: ChunkBalance{Cols: 16, Rows: 16}},
		Raid: RaidBalance{
			Stamina: StaminaBalance{Cap: dec("100"), StepCost: dec("1")},
			Standing: StandingBalance{
				CalmToNoises: 5 * time.Minute,
				NoisesToWave: 2 * time.Minute,
				WaveToWave:   time.Minute,
			},
			Death: DeathBalance{
				BackpackTTL:      30 * time.Minute,
				OwnChatHeadStart: 2 * time.Minute,
				RespawnDebuff:    time.Minute,
			},
			AFK: AFKBalance{Cruelty: dec("0.5")},
			Door: DoorBalance{
				PriceBase:             dec("10"),
				PricePerDistance:      dec("2"),
				PriceDistanceExponent: dec("1"),
				BossRewardTTL:         30 * time.Minute,
			},
			MonsterBudget: MonsterBudgetBalance{
				Base:             dec("5"),
				PerDistance:      dec("1"),
				DistanceExponent: dec("1"),
				ScalingPerLevel:  dec("0.03"),
			},
		},
		Combat: CombatBalance{
			HitDieSides:             20,
			BaseDefence:             10,
			CriticalNatural:         20,
			FumbleNatural:           1,
			WeaponDiceCount:         2,
			WeaponDieSides:          6,
			InitiativeDieSides:      4,
			MaxRounds:               20,
			VulnerabilityMultiplier: dec("1.5"),
			ResistMultiplier:        dec("0.5"),
		},
		Economy: EconomyBalance{Shop: ShopBalance{SellRate: dec("0.25"), BuyMarkup: dec("0.5")}},
	}
}

// assertBalanceEqual walks got and want field-wise, comparing every
// decimal.Decimal with decimal.Decimal.Equal and everything else with ==.
// decimal.Decimal is a *big.Int plus an exponent, so neither == nor
// reflect.DeepEqual is a numeric comparison — this helper is written once
// here and reused elsewhere in this suite.
func assertBalanceEqual(t *testing.T, got, want *Balance) {
	t.Helper()
	compareBalanceValue(t, "Balance", reflect.ValueOf(*got), reflect.ValueOf(*want))
}

var decimalType = reflect.TypeOf(decimal.Decimal{})

func compareBalanceValue(t *testing.T, path string, got, want reflect.Value) {
	t.Helper()
	if got.Type() == decimalType {
		g := got.Interface().(decimal.Decimal)  //nolint:forcetypeassert // guarded by the got.Type() == decimalType check above
		w := want.Interface().(decimal.Decimal) //nolint:forcetypeassert // guarded by the got.Type() == decimalType check above
		if !g.Equal(w) {
			t.Errorf("%s: got %s, want %s", path, g, w)
		}
		return
	}
	switch got.Kind() {
	case reflect.Struct:
		for i := range got.NumField() {
			f := got.Type().Field(i)
			compareBalanceValue(t, path+"."+f.Name, got.Field(i), want.Field(i))
		}
	default:
		if got.Interface() != want.Interface() {
			t.Errorf("%s: got %v, want %v", path, got.Interface(), want.Interface())
		}
	}
}

// writeBalanceFile writes contents to a fresh temp file and returns its
// path.
func writeBalanceFile(t *testing.T, contents string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "balance.yaml")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

func TestLoadBalance_HappyPath(t *testing.T) {
	t.Parallel()
	path := writeBalanceFile(t, validBalanceYAML)
	got, err := loadBalance(path)
	if err != nil {
		t.Fatalf("loadBalance: unexpected error: %v", err)
	}
	assertBalanceEqual(t, got, validBalance())
}

func TestLoadBalance_MissingKey(t *testing.T) {
	t.Parallel()
	yaml := strings.Replace(validBalanceYAML, "    cap: 100\n", "", 1)
	path := writeBalanceFile(t, yaml)
	_, err := loadBalance(path)
	assertKeyError(t, err, ErrMissing, "raid.stamina.cap")
}

func TestLoadBalance_NullValue(t *testing.T) {
	t.Parallel()
	yaml := strings.Replace(validBalanceYAML, "    cap: 100\n", "    cap: null\n", 1)
	path := writeBalanceFile(t, yaml)
	_, err := loadBalance(path)
	assertKeyError(t, err, ErrInvalidValue, "raid.stamina.cap")
}

// TestLoadBalance_NullValue_ZeroAdmittingKeys covers the two keys whose
// predicate accepts the zero value a !!null node decodes into, so the tag
// check is the only thing rejecting them. TestLoadBalance_NullValue above
// uses raid.stamina.cap, whose "> 0" predicate would reject the decoded zero
// on its own — deleting the tag guard leaves that test green while a real
// balance key silently defaults.
func TestLoadBalance_NullValue_ZeroAdmittingKeys(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name        string
		old         string
		replacement string
		wantKey     string
	}{
		{"duration_non_negative", "    respawn_debuff: 1m\n", "    respawn_debuff: null\n", "raid.death.respawn_debuff"},
		{"decimal_unit_fraction", "    cruelty: 0.5\n", "    cruelty: null\n", "raid.afk.cruelty"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			yaml := strings.Replace(validBalanceYAML, tc.old, tc.replacement, 1)
			if yaml == validBalanceYAML {
				t.Fatalf("fixture line %q not found — the test would assert nothing", tc.old)
			}
			path := writeBalanceFile(t, yaml)
			_, err := loadBalance(path)
			assertKeyError(t, err, ErrInvalidValue, tc.wantKey)
		})
	}
}

func TestLoadBalance_IntGivenFloat(t *testing.T) {
	t.Parallel()
	yaml := strings.Replace(validBalanceYAML, "    cols: 16\n", "    cols: 16.5\n", 1)
	path := writeBalanceFile(t, yaml)
	_, err := loadBalance(path)
	assertKeyError(t, err, ErrInvalidValue, "world.chunk.cols")
}

func TestLoadBalance_DecimalGivenQuotedNumber(t *testing.T) {
	t.Parallel()
	yaml := strings.Replace(validBalanceYAML, "    cap: 100\n", "    cap: \"100\"\n", 1)
	path := writeBalanceFile(t, yaml)
	_, err := loadBalance(path)
	assertKeyError(t, err, ErrInvalidValue, "raid.stamina.cap")
}

func TestLoadBalance_DurationGivenBareInt(t *testing.T) {
	t.Parallel()
	yaml := strings.Replace(validBalanceYAML, "    calm_to_noises: 5m\n", "    calm_to_noises: 300\n", 1)
	path := writeBalanceFile(t, yaml)
	_, err := loadBalance(path)
	assertKeyError(t, err, ErrInvalidValue, "raid.standing.calm_to_noises")
}

func TestLoadBalance_PredicateFailures(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		old, new string
		key      string
	}{
		{"non_positive", "    cols: 16\n", "    cols: 0\n", "world.chunk.cols"},
		{"negative", "    cols: 16\n", "    cols: -1\n", "world.chunk.cols"},
		{"above_upper_bound", "    cruelty: 0.5\n", "    cruelty: 1.5\n", "raid.afk.cruelty"},
		{"at_excluded_bound", "    sell_rate: 0.25\n", "    sell_rate: 1\n", "economy.shop.sell_rate"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			yaml := strings.Replace(validBalanceYAML, tc.old, tc.new, 1)
			path := writeBalanceFile(t, yaml)
			_, err := loadBalance(path)
			assertKeyError(t, err, ErrInvalidValue, tc.key)
		})
	}
}

func TestLoadBalance_UnknownKey_TopLevel(t *testing.T) {
	t.Parallel()
	yaml := validBalanceYAML + "bogus_key: 1\n"
	path := writeBalanceFile(t, yaml)
	_, err := loadBalance(path)
	assertKeyError(t, err, ErrUnknownKey, "bogus_key")
}

func TestLoadBalance_UnknownKey_Nested(t *testing.T) {
	t.Parallel()
	yaml := strings.Replace(validBalanceYAML, "raid:\n", "raid:\n  bogus_nested: 1\n", 1)
	path := writeBalanceFile(t, yaml)
	_, err := loadBalance(path)
	assertKeyError(t, err, ErrUnknownKey, "raid.bogus_nested")
}

func TestLoadBalance_ShapeMismatch_ScalarWhereMappingExpected(t *testing.T) {
	t.Parallel()
	block := "  stamina:\n    cap: 100\n    step_cost: 1\n"
	yaml := strings.Replace(validBalanceYAML, block, "  stamina: oops\n", 1)
	path := writeBalanceFile(t, yaml)
	_, err := loadBalance(path)
	assertKeyError(t, err, ErrInvalidValue, "raid.stamina")
}

func TestLoadBalance_ShapeMismatch_MappingWhereScalarExpected(t *testing.T) {
	t.Parallel()
	yaml := strings.Replace(validBalanceYAML, "    cap: 100\n", "    cap:\n      a: 1\n", 1)
	path := writeBalanceFile(t, yaml)
	_, err := loadBalance(path)
	assertKeyError(t, err, ErrInvalidValue, "raid.stamina.cap")
}

func TestLoadBalance_Alias(t *testing.T) {
	t.Parallel()
	yaml := strings.Replace(validBalanceYAML, "    cols: 16\n    rows: 16\n", "    cols: &n 16\n    rows: *n\n", 1)
	path := writeBalanceFile(t, yaml)
	_, err := loadBalance(path)
	assertKeyError(t, err, ErrInvalidValue, "world.chunk.rows")
}

func TestLoadBalance_MergeKey(t *testing.T) {
	t.Parallel()
	yaml := strings.Replace(validBalanceYAML, "  stamina:\n", "  stamina: &st\n", 1)
	yaml = strings.Replace(yaml, "  standing:\n", "  standing:\n    <<: *st\n", 1)
	path := writeBalanceFile(t, yaml)
	_, err := loadBalance(path)
	assertKeyError(t, err, ErrUnknownKey, "raid.standing.<<")
}

func TestLoadBalance_DuplicateKey(t *testing.T) {
	t.Parallel()
	yaml := strings.Replace(validBalanceYAML, "    cols: 16\n", "    cols: 16\n    cols: 17\n", 1)
	path := writeBalanceFile(t, yaml)
	_, err := loadBalance(path)
	if err == nil {
		t.Fatal("loadBalance: expected an error for a duplicated key")
	}
	if !strings.Contains(err.Error(), "already defined") {
		t.Errorf("loadBalance: error %q does not report the duplicate as the parser's own error", err)
	}
}

func TestLoadBalance_NonMappingRoot(t *testing.T) {
	t.Parallel()
	path := writeBalanceFile(t, "- 1\n- 2\n")
	_, err := loadBalance(path)
	assertKeyError(t, err, ErrInvalidValue, "<root>")
}

func TestLoadBalance_SyntaxError(t *testing.T) {
	t.Parallel()
	path := writeBalanceFile(t, "world: [unterminated\n")
	_, err := loadBalance(path)
	if err == nil {
		t.Fatal("loadBalance: expected a parse error")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("loadBalance: error %q does not name the file", err)
	}
}

// TestLoadBalance_EmptyDocumentReportsEveryPathMissing covers the "every
// path missing" report as three distinct inputs because they are not the
// same node shape: a walk normalising at the document node would accept
// "{}" as populated and never report the missing paths.
func TestLoadBalance_EmptyDocumentReportsEveryPathMissing(t *testing.T) {
	t.Parallel()
	schema := balanceSchema(&Balance{})
	cases := map[string]string{
		"empty_string":    "",
		"whitespace_only": " \n  \n",
		"braces":          "{}\n",
	}
	for name, contents := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			path := writeBalanceFile(t, contents)
			_, err := loadBalance(path)
			if err == nil {
				t.Fatal("loadBalance: expected every schema path to be reported missing")
			}
			for _, e := range schema {
				key := strings.Join(e.path, ".")
				if !containsKeyError(err, key) {
					t.Errorf("%s: expected %s to be reported missing", name, key)
				}
			}
		})
	}
}

// assertKeyError asserts that err wraps sentinel via errors.Is, that
// errors.As recovers a *KeyError whose Key equals wantKey, and that the
// rendered message contains wantKey.
func assertKeyError(t *testing.T, err error, sentinel error, wantKey string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected a non-nil error")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("errors.Is(err, %v) = false; err = %v", sentinel, err)
	}
	if !containsKeyError(err, wantKey) {
		t.Errorf("no *KeyError with Key %q found in %v", wantKey, err)
	}
	if !strings.Contains(err.Error(), wantKey) {
		t.Errorf("error message %q does not contain key %q", err.Error(), wantKey)
	}
}

// containsKeyError reports whether err — possibly an errors.Join tree —
// contains a *KeyError whose Key equals key.
func containsKeyError(err error, key string) bool {
	var kerr *KeyError
	if errors.As(err, &kerr) && kerr.Key == key {
		return true
	}
	if u, ok := err.(interface{ Unwrap() []error }); ok {
		for _, sub := range u.Unwrap() {
			if containsKeyError(sub, key) {
				return true
			}
		}
	}
	return false
}
