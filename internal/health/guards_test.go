package health

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"

	"github.com/maratik123/lab-game/internal/ingest"
	"github.com/maratik123/lab-game/internal/repotest"
	"github.com/maratik123/lab-game/internal/scheduler"
	"github.com/maratik123/lab-game/internal/tg"
	"github.com/maratik123/lab-game/internal/tgtest"
)

// --- shared walk plumbing -------------------------------------------------

// walkGoFilesUnder calls fn with the path of every Go source file under
// root, skipping version-control directories a real repository root
// never needs walked, and skipping the repository's own designated
// scratch directory (the one directory a live session is expected to
// write throwaway probes into) — such a probe is not part of the tree
// this guard polices.
func walkGoFilesUnder(t *testing.T, root string, fn func(path string)) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			if path == filepath.Join(root, "tmp") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".go") {
			fn(path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir(%s): %v", root, err)
	}
}

// parseGoFile parses path with comments retained.
func parseGoFile(t *testing.T, path string) *ast.File {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("ParseFile(%s): %v", path, err)
	}
	return f
}

// importPaths returns f's own imports, unquoted.
func importPaths(t *testing.T, f *ast.File) []string {
	t.Helper()
	paths := make([]string, 0, len(f.Imports))
	for _, imp := range f.Imports {
		v, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			t.Fatalf("Unquote(%s): %v", imp.Path.Value, err)
		}
		paths = append(paths, v)
	}
	return paths
}

// --- guard (a): no promauto import, no reference to the client library's
// own default registerer/gatherer or its package-level MustRegister ------

// promAutoOffenses walks every Go source file under the module's command
// and internal source trees and reports each file that imports the
// client library's promauto helper package or refers to its default
// registerer, default gatherer, or package-level MustRegister — every
// one of them a use of the client library's own global state rather
// than this package's own registries.
func promAutoOffenses(t *testing.T, root string) []string {
	t.Helper()
	var offenses []string
	walkGoFilesUnder(t, root, func(path string) {
		rel, err := filepath.Rel(root, path)
		if err != nil {
			t.Fatalf("Rel(%s, %s): %v", root, path, err)
		}
		top := strings.SplitN(filepath.ToSlash(rel), "/", 2)[0]
		if top != "cmd" && top != "internal" {
			return
		}
		f := parseGoFile(t, path)
		for _, p := range importPaths(t, f) {
			if strings.HasSuffix(p, "prometheus/promauto") {
				offenses = append(offenses, rel+": imports "+p)
			}
		}
		ast.Inspect(f, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			ident, ok := sel.X.(*ast.Ident)
			if !ok || ident.Name != "prometheus" {
				return true
			}
			switch sel.Sel.Name {
			case "DefaultRegisterer", "DefaultGatherer", "MustRegister":
				offenses = append(offenses, rel+": references prometheus."+sel.Sel.Name)
			}
			return true
		})
	})
	return offenses
}

func TestGuard_NoPromautoOrDefaultRegisterer(t *testing.T) {
	t.Parallel()
	if offenses := promAutoOffenses(t, repotest.Root(t)); len(offenses) != 0 {
		t.Errorf("promauto/default-registerer offenses:\n%s", strings.Join(offenses, "\n"))
	}
}

// TestGuard_NoPromautoOrDefaultRegisterer_ProvenDiscriminating runs the
// walk against a scratch tree carrying the banned import, over its own
// t.TempDir() copy — never the worktree — and requires it to fail.
func TestGuard_NoPromautoOrDefaultRegisterer_ProvenDiscriminating(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	dir := filepath.Join(tmp, "internal", "scratch")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	src := "package scratch\n\nimport _ \"github.com/prometheus/client_golang/prometheus/promauto\"\n"
	if err := os.WriteFile(filepath.Join(dir, "scratch.go"), []byte(src), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if offenses := promAutoOffenses(t, tmp); len(offenses) == 0 {
		t.Fatal("expected the scratch promauto import to be flagged")
	}
}

// --- guard (b): the observation-field register binds the structs, both
// directions, and every exempt field is named in the package doc -------

// healthPackageDoc returns this package's own package doc comment text.
func healthPackageDoc(t *testing.T) string {
	t.Helper()
	path := repotest.RootPath(t, filepath.Join("internal", "health", "doc.go"))
	f := parseGoFile(t, path)
	if f.Doc == nil {
		t.Fatal("doc.go has no package doc comment")
	}
	return f.Doc.Text()
}

// registerOffenses checks entries — one struct name's field register —
// against typ's own field set in both directions, and (for exempt
// entries) against docText. It returns one message per mismatch, so a
// caller can assert either "none" (the real guard) or "at least one" (the
// discriminating proof below).
func registerOffenses(structName string, entries []fieldEntry, typ reflect.Type, docText string) []string {
	var offenses []string
	tableFields := map[string]bool{}
	for _, e := range entries {
		tableFields[e.Field] = true
	}
	structFields := map[string]bool{}
	for i := range typ.NumField() {
		structFields[typ.Field(i).Name] = true
	}
	for f := range structFields {
		if !tableFields[f] {
			offenses = append(offenses, structName+"."+f+": struct field has no register row")
		}
	}
	for f := range tableFields {
		if !structFields[f] {
			offenses = append(offenses, structName+"."+f+": register row names a field the struct does not have")
		}
	}
	for _, e := range entries {
		if e.Family == "" && !strings.Contains(docText, e.Field) {
			offenses = append(offenses, structName+"."+e.Field+": exempt but not named in the package doc comment")
		}
	}
	return offenses
}

func TestGuard_ObservationRegisterBindsStructsBothDirections(t *testing.T) {
	t.Parallel()
	docText := healthPackageDoc(t)
	types := map[string]reflect.Type{
		"tg.Observation":            reflect.TypeOf(tg.Observation{}),
		"scheduler.Observation":     reflect.TypeOf(scheduler.Observation{}),
		"scheduler.LoopObservation": reflect.TypeOf(scheduler.LoopObservation{}),
		"ingest.Observation":        reflect.TypeOf(ingest.Observation{}),
		"ingest.LoopObservation":    reflect.TypeOf(ingest.LoopObservation{}),
	}
	for name, typ := range types {
		entries, ok := observationRegister[name]
		if !ok {
			t.Fatalf("observationRegister has no entry for %q", name)
		}
		for _, offense := range registerOffenses(name, entries, typ, docText) {
			t.Error(offense)
		}
	}
}

// TestGuard_ObservationRegisterBindsStructsBothDirections_ProvenDiscriminating
// runs the same check function against a table with one row removed and
// against a doc comment with one exempt name removed, each required to
// fail.
func TestGuard_ObservationRegisterBindsStructsBothDirections_ProvenDiscriminating(t *testing.T) {
	t.Parallel()
	typ := reflect.TypeOf(scheduler.Observation{})
	full := observationRegister["scheduler.Observation"]
	docText := healthPackageDoc(t)

	missingRow := make([]fieldEntry, 0, len(full)-1)
	for _, e := range full {
		if e.Field == fieldBatchSize {
			continue
		}
		missingRow = append(missingRow, e)
	}
	if offenses := registerOffenses("scheduler.Observation", missingRow, typ, docText); len(offenses) == 0 {
		t.Error("expected a struct field with no register row to be flagged")
	}

	strippedDoc := strings.ReplaceAll(docText, "ConsecutiveFailures", "REDACTED")
	if offenses := registerOffenses("scheduler.Observation", full, typ, strippedDoc); len(offenses) == 0 {
		t.Error("expected an exempt field missing from the doc comment to be flagged")
	}
}

// --- guard (c): label-name allow-list and per-(family, label) observed
// value sets, over a registry every adapter has been driven through ----

// driveEveryAdapterOnce registers every adapter on reg and drives each
// one through its own value set once: every transport response class,
// every scheduler task outcome and failure classification, every update
// outcome plus the empty (unrouted) kind, a real pool snapshot, and both
// canary legs in both outcomes.
func driveEveryAdapterOnce(t *testing.T, reg prometheus.Registerer) {
	t.Helper()

	to, err := NewTransportObserver(reg)
	if err != nil {
		t.Fatalf("NewTransportObserver: %v", err)
	}
	to.ObserveCall(tg.Observation{Method: "getMe", StatusCode: 200})
	to.ObserveCall(tg.Observation{Method: "sendMessage", StatusCode: 429, RateLimited: true, Retries: 2})

	so, err := NewSchedulerObserver(reg)
	if err != nil {
		t.Fatalf("NewSchedulerObserver: %v", err)
	}
	for _, outcome := range []scheduler.Outcome{scheduler.OutcomeDone, scheduler.OutcomeNoop, scheduler.OutcomeFailed} {
		so.ObserveTask(scheduler.Observation{Type: "raid_extraction", Outcome: outcome})
	}
	for _, fk := range []scheduler.FailureKind{
		scheduler.FailureNone, scheduler.FailureHandler, scheduler.FailureUnregistered,
		scheduler.FailureDeadline, scheduler.FailureRolledBack,
	} {
		so.ObserveTask(scheduler.Observation{Type: "raid_extraction", Outcome: scheduler.OutcomeFailed, Failure: fk})
	}
	so.ObserveLoop(scheduler.LoopObservation{Duration: time.Millisecond, BatchSize: 3})

	io, err := NewIngestObserver(reg)
	if err != nil {
		t.Fatalf("NewIngestObserver: %v", err)
	}
	for _, outcome := range []ingest.Outcome{
		ingest.OutcomeHandled, ingest.OutcomeDuplicate, ingest.OutcomeUnrouted,
		ingest.OutcomeFailed, ingest.OutcomePanic, ingest.OutcomeGivenUp,
	} {
		io.ObserveUpdate(ingest.Observation{Kind: ingest.KindMessage, Outcome: outcome, LagKnown: true})
	}
	io.ObserveUpdate(ingest.Observation{Kind: "", Outcome: ingest.OutcomeUnrouted})
	io.ObserveLoop(ingest.LoopObservation{Duration: time.Millisecond, BatchSize: 1})

	stat := realStat(t, 4)
	if _, err := NewPoolCollector(reg, func() *pgxpool.Stat { return stat }); err != nil {
		t.Fatalf("NewPoolCollector: %v", err)
	}

	own := &fakeProber{}
	cloud := &fakeProber{}
	failingOwn := &fakeProber{err: errors.New("own failed")}
	failingCloud := &fakeProber{err: errors.New("cloud failed")}
	canary, err := NewCanary(reg, CanaryOptions{Legs: &Legs{Own: own, Cloud: cloud}, Interval: time.Hour})
	if err != nil {
		t.Fatalf("NewCanary: %v", err)
	}
	ctx := context.Background()
	canary.probeLeg(ctx, legOwn, own)
	canary.probeLeg(ctx, legCloud, cloud)
	canary.probeLeg(ctx, legOwn, failingOwn)
	canary.probeLeg(ctx, legCloud, failingCloud)
}

// labelValuesFor returns the observed set of label label's values, across
// every metric whose family name is in families.
func labelValuesFor(mfs []*dto.MetricFamily, families []string, label string) map[string]bool {
	want := map[string]bool{}
	for _, f := range families {
		want[f] = true
	}
	values := map[string]bool{}
	for _, mf := range mfs {
		if !want[mf.GetName()] {
			continue
		}
		for _, m := range mf.GetMetric() {
			for _, lp := range m.GetLabel() {
				if lp.GetName() == label {
					values[lp.GetValue()] = true
				}
			}
		}
	}
	return values
}

func assertExactSet(t *testing.T, got map[string]bool, want []string, what string) {
	t.Helper()
	wantSet := map[string]bool{}
	for _, w := range want {
		wantSet[w] = true
	}
	if len(got) != len(wantSet) {
		t.Errorf("%s: observed %v, want exactly %v", what, got, want)
		return
	}
	for w := range wantSet {
		if !got[w] {
			t.Errorf("%s: observed %v, missing %q", what, got, w)
		}
	}
}

func TestGuard_LabelNamesAndClosedSetValues(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	driveEveryAdapterOnce(t, reg)

	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}

	allowed := map[string]bool{}
	for _, n := range allowedLabelNames {
		allowed[n] = true
	}
	for _, mf := range mfs {
		for _, m := range mf.GetMetric() {
			for _, lp := range m.GetLabel() {
				if !allowed[lp.GetName()] {
					t.Errorf("family %s carries label %q outside the allow-list", mf.GetName(), lp.GetName())
				}
			}
		}
	}

	assertExactSet(t, labelValuesFor(mfs, []string{familySchedulerTasks}, labelOutcome),
		[]string{"done", "noop", "failed"}, "labgame_scheduler_tasks_total.outcome")
	assertExactSet(t, labelValuesFor(mfs, []string{familySchedulerTasks}, labelFailure),
		[]string{"none", "handler", "unregistered", "deadline", "rolled_back"}, "labgame_scheduler_tasks_total.failure")
	assertExactSet(t, labelValuesFor(mfs, []string{familyIngestUpdateOutcomes, familyIngestHandlerDuration}, labelOutcome),
		[]string{"handled", "duplicate", "unrouted", "failed", "panic", "given_up"}, "ingest outcome families.outcome")
	assertExactSet(t, labelValuesFor(mfs, []string{familyIngestUpdateLag, familyIngestHandlerDuration, familyIngestUpdateOutcomes}, labelKind),
		[]string{"message", unknownLabelValue}, "ingest kind-carrying families.kind")
	assertExactSet(t, labelValuesFor(mfs, []string{familyCanaryProbes, familyCanaryProbeDuration}, labelOutcome),
		[]string{canaryOutcomeSuccess, canaryOutcomeFailure}, "canary outcome families.outcome")
	assertExactSet(t, labelValuesFor(mfs, []string{familyCanaryProbes, familyCanaryProbeDuration, familyCanaryProbeFailures}, labelLeg),
		[]string{legOwn, legCloud}, "canary leg-carrying families.leg")
	assertExactSet(t, labelValuesFor(mfs, []string{familyPoolConns}, labelState),
		[]string{poolStateIdle, poolStateAcquired, poolStateConstructing}, "labgame_pgxpool_conns.state")

	// method, code and reason are not closed-set: a shape assertion only.
	for v := range labelValuesFor(mfs, []string{familyBotAPICallDuration, familyBotAPIResponses, familyBotAPIRateLimited, familyBotAPIRetries}, labelMethod) {
		if v == "" {
			t.Error("method label carries an empty value")
		}
	}
	for v := range labelValuesFor(mfs, []string{familyBotAPIResponses}, labelCode) {
		if _, err := strconv.Atoi(v); err != nil {
			t.Errorf("code label value %q is not decimal", v)
		}
	}
	for v := range labelValuesFor(mfs, []string{familyCanaryProbeFailures}, labelReason) {
		if _, err := strconv.Atoi(v); err != nil {
			switch v {
			case "timeout", "canceled", "network":
			default:
				t.Errorf("reason label value %q is neither decimal nor an enumerated transport class", v)
			}
		}
	}
}

// --- guard (d): the scrape body carries none of the fixture's sentinel
// secrets --------------------------------------------------------------

// TestGuard_ScrapeCarriesNoSentinelSecret drives every genuinely
// confidential value this package's adapters ever see with a fixture and
// asserts none of them reach the scrape body. A chat id, an update id and
// a task/operation id are deliberately not among the sentinels: none of
// the observation types this package's adapters accept carries such a
// field, so no observation can carry one — the category is closed by
// those structs' own field sets, not by anything this scrape does, and a
// sentinel for it could never go red. The two caller-supplied strings
// that DO pass through to a label verbatim — a scheduled task's own type
// and an update's own kind — are excluded for the opposite reason: they
// are the task-type and update-kind dimensions the metrics exist to
// break down by, not secrets, so asserting their absence would assert
// against the adapters' own documented behaviour.
func TestGuard_ScrapeCarriesNoSentinelSecret(t *testing.T) {
	t.Parallel()
	const (
		// telego's own token format (a digit run, a colon, then exactly
		// 35 word/hyphen characters) is validated at construction, so
		// each sentinel token is padded with hyphens to match it while
		// staying unmistakably a fixture rather than a credential.
		sentinelBotToken    = "1:SENTINEL-BOT-TOKEN-VALUE-----------"
		sentinelCloudToken  = "1:SENTINEL-CLOUD-TOKEN-VALUE---------"
		sentinelDSNPassword = "SENTINEL-DSN-PASSWORD"
	)

	reg := NewRegistry()

	srv := tgtest.New(t, tgtest.Success(nil))
	factory := func(o TelegramProberOptions) (Prober, error) {
		o.HTTPClient = srv.Client()
		return NewTelegramProber(o)
	}
	legs, err := NewLegs(LegsOptions{
		OwnToken:      sentinelBotToken,
		OwnBaseURL:    tgtest.BaseURL,
		CloudToken:    sentinelCloudToken,
		CloudBaseURL:  tgtest.BaseURL,
		Transport:     probeTransport(),
		ProberFactory: factory,
	})
	if err != nil {
		t.Fatalf("NewLegs: %v", err)
	}
	canary, err := NewCanary(reg, CanaryOptions{Legs: legs, Interval: time.Hour})
	if err != nil {
		t.Fatalf("NewCanary: %v", err)
	}
	ctx := context.Background()
	canary.probeLeg(ctx, legOwn, legs.Own)
	canary.probeLeg(ctx, legCloud, legs.Cloud)

	dsn := closedPortDSN(t)
	dsn = strings.Replace(dsn, "pass", sentinelDSNPassword, 1)
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("NewWithConfig: %v", err)
	}
	t.Cleanup(pool.Close)
	stat := pool.Stat()
	if _, err := NewPoolCollector(reg, func() *pgxpool.Stat { return stat }); err != nil {
		t.Fatalf("NewPoolCollector: %v", err)
	}

	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	text := gatherText(t, mfs)
	for _, secret := range []string{
		sentinelBotToken, sentinelCloudToken, sentinelDSNPassword,
	} {
		if strings.Contains(text, secret) {
			t.Errorf("scrape text contains sentinel secret %q", secret)
		}
	}
}

// --- guard (e): promlint reports no problem -----------------------------

func TestGuard_PromlintReportsNoProblem(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	driveEveryAdapterOnce(t, reg)

	problems, err := testutil.GatherAndLint(reg)
	if err != nil {
		t.Fatalf("GatherAndLint: %v", err)
	}
	for _, p := range problems {
		t.Errorf("promlint: %s: %s", p.Metric, p.Text)
	}
}

// --- guard (f): this package's own non-test source carries no panic(,
// log.Fatal, os.Exit, MustRegister, or MustNew-prefixed library call ----

// panicOrMustCallOffenses scans every non-test Go source file directly
// under dir for the forbidden literals.
func panicOrMustCallOffenses(t *testing.T, dir string) []string {
	t.Helper()
	forbidden := []string{"panic(", "log.Fatal", "os.Exit", "MustRegister", "MustNew"}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir(%s): %v", dir, err)
	}
	var offenses []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		content, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", e.Name(), err)
		}
		for _, f := range forbidden {
			if strings.Contains(string(content), f) {
				offenses = append(offenses, e.Name()+": contains "+f)
			}
		}
	}
	return offenses
}

func TestGuard_NoPanicLogFatalOsExitOrLibraryMustCall(t *testing.T) {
	t.Parallel()
	dir := repotest.RootPath(t, filepath.Join("internal", "health"))
	if offenses := panicOrMustCallOffenses(t, dir); len(offenses) != 0 {
		t.Errorf("forbidden-call offenses:\n%s", strings.Join(offenses, "\n"))
	}
}

// TestGuard_NoPanicLogFatalOsExitOrLibraryMustCall_ProvenDiscriminating
// proves the walk against both categories it covers: a bare panic( and a
// library Must… call round 3's version of this guard could not detect.
// Both scratch files live in their own t.TempDir(), never the worktree.
func TestGuard_NoPanicLogFatalOsExitOrLibraryMustCall_ProvenDiscriminating(t *testing.T) {
	t.Parallel()

	panicDir := t.TempDir()
	panicSrc := "package scratch\n\nfunc f() { panic(\"boom\") }\n"
	if err := os.WriteFile(filepath.Join(panicDir, "scratch.go"), []byte(panicSrc), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if offenses := panicOrMustCallOffenses(t, panicDir); len(offenses) == 0 {
		t.Error("expected a bare panic( to be flagged")
	}

	mustRegisterDir := t.TempDir()
	mustSrc := "package scratch\n\nimport \"github.com/prometheus/client_golang/prometheus\"\n\nfunc f(reg *prometheus.Registry, c prometheus.Collector) { reg.MustRegister(c) }\n"
	if err := os.WriteFile(filepath.Join(mustRegisterDir, "scratch.go"), []byte(mustSrc), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if offenses := panicOrMustCallOffenses(t, mustRegisterDir); len(offenses) == 0 {
		t.Error("expected a library MustRegister call to be flagged")
	}
}

// --- guard (g): exactly one file-location-ascent resolver module-wide,
// inside the shared root-resolving package, and no repoRootPath
// declaration anywhere ---------------------------------------------------

// isFileLocationAscent reports whether fn both calls runtime.Caller and
// ascends the result via a filepath.Dir chain or a filepath.Join call
// carrying a ".." component — the file-location-ascent kind, under
// whatever spelling.
func isFileLocationAscent(fn *ast.FuncDecl) bool {
	if fn.Body == nil {
		return false
	}
	var usesRuntimeCaller, usesAscent bool
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pkgIdent, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		switch {
		case pkgIdent.Name == "runtime" && sel.Sel.Name == "Caller":
			usesRuntimeCaller = true
		case pkgIdent.Name == "filepath" && sel.Sel.Name == "Dir":
			usesAscent = true
		case pkgIdent.Name == "filepath" && sel.Sel.Name == "Join":
			for _, arg := range call.Args {
				lit, ok := arg.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}
				if v, err := strconv.Unquote(lit.Value); err == nil && v == ".." {
					usesAscent = true
				}
			}
		}
		return true
	})
	return usesRuntimeCaller && usesAscent
}

// resolverHit names one file-location-ascent resolver's location.
type resolverHit struct {
	relPath  string
	funcName string
}

// fileLocationAscentResolvers walks every Go source file under root and
// returns every function of the file-location-ascent kind it declares.
func fileLocationAscentResolvers(t *testing.T, root string) []resolverHit {
	t.Helper()
	var hits []resolverHit
	walkGoFilesUnder(t, root, func(path string) {
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("ParseFile(%s): %v", path, err)
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			t.Fatalf("Rel(%s, %s): %v", root, path, err)
		}
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			if isFileLocationAscent(fn) {
				hits = append(hits, resolverHit{relPath: filepath.ToSlash(rel), funcName: fn.Name.Name})
			}
		}
	})
	return hits
}

func TestGuard_SingleRepoRootResolver_RealTree(t *testing.T) {
	t.Parallel()
	root := repotest.Root(t)
	hits := fileLocationAscentResolvers(t, root)

	var inside, outside int
	for _, h := range hits {
		if strings.HasPrefix(h.relPath, "internal/repotest/") {
			inside++
		} else {
			outside++
			t.Errorf("file-location-ascent resolver outside internal/repotest: %s (%s)", h.relPath, h.funcName)
		}
	}
	if inside != 1 {
		t.Errorf("internal/repotest declares %d file-location-ascent resolvers, want exactly 1", inside)
	}
}

// TestGuard_NoRepoRootPathIdentifierAnywhere parses (rather than greps)
// every Go source file in the module and looks for an actual function
// declaration named repoRootPath — the old unexported spelling every
// resolver this consolidation replaced once carried. Parsing rather than
// substring-matching is required here: this very file's own string
// literals name that spelling too, as the fixtures the guard's own
// discriminating proofs are built from, and a raw grep would flag them.
func TestGuard_NoRepoRootPathIdentifierAnywhere(t *testing.T) {
	t.Parallel()
	root := repotest.Root(t)
	var offenses []string
	walkGoFilesUnder(t, root, func(path string) {
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("ParseFile(%s): %v", path, err)
		}
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if ok && fn.Name.Name == "repoRootPath" {
				rel, relErr := filepath.Rel(root, path)
				if relErr != nil {
					t.Fatalf("Rel: %v", relErr)
				}
				offenses = append(offenses, rel)
			}
		}
	})
	if len(offenses) != 0 {
		t.Errorf("repoRootPath still declared at: %v", offenses)
	}
}

// preMoveFixture returns the frozen pre-consolidation content of rel
// (repository-root-relative), read from a committed testdata snapshot
// rather than via git — CI runs on a shallow clone that lacks the
// consolidation commit's parent, so reconstructing that history with
// "git show <rev>~1:<path>" fails there even though it succeeds in a
// full checkout. The stored copy carries a "fixture" suffix so the
// module-wide guards below, which walk every Go source file on disk,
// do not parse it as a second copy of a resolver they are checking
// for.
func preMoveFixture(t *testing.T, rel string) []byte {
	t.Helper()
	path := filepath.Join("testdata", "pre-move", filepath.FromSlash(rel)+".fixture")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	return content
}

// workingTreeFile returns rel's (repository-root-relative) current
// content under root. Used for the git-mechanism files, which must
// stay live: the guard's point is that a git-based resolver is not of
// the file-location-ascent class, so it is read as it stands today.
func workingTreeFile(t *testing.T, root, rel string) []byte {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	return content
}

// TestGuard_SingleRepoRootResolver_PreChangeTree validates guard (g)
// against the tree the consolidation replaced, not against scratch
// files written to match the rule's own shape: it names every in-class
// resolver the earlier consolidation's own scope derives and no other
// file, over a real pre-change tree, entirely inside a t.TempDir() copy
// the worktree never sees.
func TestGuard_SingleRepoRootResolver_PreChangeTree(t *testing.T) {
	t.Parallel()
	root := repotest.Root(t)

	preMoveFiles := []string{
		"cmd/bot/main_test.go",
		"internal/config/repo_root_test.go",
		"internal/ingest/guards_test.go",
		"internal/tg/guards_test.go",
		"internal/commentref/testhelpers_test.go",
		"internal/testdb/server_test.go",
	}
	gitBasedFiles := []string{
		"cmd/commentrefs/git.go",
		"cmd/commentrefs/git_test.go",
	}

	tmp := t.TempDir()
	writeAt := func(rel string, content []byte) {
		dst := filepath.Join(tmp, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.WriteFile(dst, content, 0o600); err != nil {
			t.Fatalf("WriteFile(%s): %v", dst, err)
		}
	}
	for _, rel := range preMoveFiles {
		writeAt(rel, preMoveFixture(t, rel))
	}
	for _, rel := range gitBasedFiles {
		writeAt(rel, workingTreeFile(t, root, rel))
	}

	hits := fileLocationAscentResolvers(t, tmp)
	got := map[string]bool{}
	for _, h := range hits {
		got[h.relPath] = true
	}
	if len(got) != len(preMoveFiles) {
		t.Errorf("pre-change tree: resolvers found in %v, want exactly %v", got, preMoveFiles)
	}
	for _, rel := range preMoveFiles {
		if !got[rel] {
			t.Errorf("pre-change tree: expected a resolver in %s, found none", rel)
		}
	}
	for _, rel := range gitBasedFiles {
		if got[rel] {
			t.Errorf("pre-change tree: git-based resolver %s wrongly flagged as file-location-ascent", rel)
		}
	}
}

// --- guard (h): no non-test file in the module imports the shared
// test-only root-resolving package -----------------------------------

func TestGuard_NoNonTestFileImportsRepotest(t *testing.T) {
	t.Parallel()
	root := repotest.Root(t)
	const bannedImport = "github.com/maratik123/lab-game/internal/repotest"
	var offenses []string
	walkGoFilesUnder(t, root, func(path string) {
		if strings.HasSuffix(path, "_test.go") {
			return
		}
		f := parseGoFile(t, path)
		for _, p := range importPaths(t, f) {
			if p == bannedImport {
				rel, err := filepath.Rel(root, path)
				if err != nil {
					t.Fatalf("Rel: %v", err)
				}
				offenses = append(offenses, rel)
			}
		}
	})
	if len(offenses) != 0 {
		t.Errorf("non-test file(s) import the shared root-resolving package: %v", offenses)
	}
}

// --- guard (i): the canary's own two files import no database or
// scheduler symbol — file-scoped, since two sibling files in this same
// package legitimately do --------------------------------------------

var bannedCanaryImports = []string{
	"github.com/jackc/pgx/v5",
	"github.com/jackc/pgx/v5/pgxpool",
	"github.com/maratik123/lab-game/internal/store",
	"github.com/maratik123/lab-game/internal/scheduler",
}

// canaryFileImportOffenses returns every banned import path present in
// path's own import block.
func canaryFileImportOffenses(t *testing.T, path string) []string {
	t.Helper()
	f := parseGoFile(t, path)
	var offenses []string
	for _, p := range importPaths(t, f) {
		for _, banned := range bannedCanaryImports {
			if p == banned {
				offenses = append(offenses, p)
			}
		}
	}
	return offenses
}

func TestGuard_CanaryFilesImportNoDatabaseOrSchedulerSymbol(t *testing.T) {
	t.Parallel()
	for _, rel := range []string{"canary.go", "probe.go"} {
		path := repotest.RootPath(t, filepath.Join("internal", "health", rel))
		if offenses := canaryFileImportOffenses(t, path); len(offenses) != 0 {
			t.Errorf("%s imports banned package(s): %v", rel, offenses)
		}
	}
}

// TestGuard_CanaryFilesImportNoDatabaseOrSchedulerSymbol_ProvenDiscriminating
// proves the walk against a scratch import added to a temp copy — the
// tracked file is never edited.
func TestGuard_CanaryFilesImportNoDatabaseOrSchedulerSymbol_ProvenDiscriminating(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	src := "package health\n\nimport (\n\t\"github.com/jackc/pgx/v5/pgxpool\"\n)\n\nvar _ = pgxpool.Stat{}\n"
	path := filepath.Join(tmp, "probe.go")
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if offenses := canaryFileImportOffenses(t, path); len(offenses) == 0 {
		t.Fatal("expected the scratch pgxpool import to be flagged")
	}
}
