package gateguard_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	yaml "go.yaml.in/yaml/v3"

	"github.com/maratik123/lab-game/internal/repotest"
	"github.com/maratik123/lab-game/internal/srcguard"
)

// -----------------------------------------------------------------------
// The launch guard: every bare go statement in the module's compiled
// non-test source is keyed by its enclosing function and checked
// against a reviewed allow list.
// -----------------------------------------------------------------------

// launchKey identifies one goroutine-launching function declaration: the
// package directory holding it (relative to the tree root, "/"-separated)
// and its own name — a receiver method spelled "(*Type).Method" or
// "(Type).Method", a plain function spelled by its bare name.
type launchKey struct {
	dir  string
	name string
}

// launchAnswer is one launch's reviewed accounting: how it stops, who
// waits for it, where its error goes, and where its panic goes. Every
// field is required — an empty one is itself a finding.
type launchAnswer struct {
	stops, waitedBy, errorTo, panicTo string
}

// missingField returns the name of the first empty field, or "" when
// every field is filled in.
func (a launchAnswer) missingField() string {
	switch {
	case a.stops == "":
		return "stops"
	case a.waitedBy == "":
		return "waitedBy"
	case a.errorTo == "":
		return "errorTo"
	case a.panicTo == "":
		return "panicTo"
	default:
		return ""
	}
}

// launchTable is the reviewed allow list for every bare go statement in
// the module's compiled non-test source today: one row per enclosing
// function, one answer per launch that function makes, in source order.
var launchTable = map[launchKey][]launchAnswer{
	{"cmd/bot", "(*app).drain"}: {
		{
			stops:    "the errgroup.Group it wraps finishes its own Wait once every runner has stopped and returned",
			waitedBy: "drain's own select, on the joined channel this goroutine closes",
			errorTo:  "discarded — this goroutine only signals completion, never the group's own error",
			panicTo:  "unrecovered — a panicking runner is expected to have already recovered at its own boundary",
		},
	},
	{"internal/health", "(*Canary).Start"}: {
		{
			stops:    "ctx.Done(), cancelled by Shutdown",
			waitedBy: "Shutdown, which cancels ctx then selects on c.done",
			errorTo:  "not applicable — the tick loop returns nothing",
			panicTo:  "unrecovered",
		},
	},
	{"internal/health", "(*Canary).tick"}: {
		{
			stops:    "probeLeg returns once its own bounded probe completes or the shared tickCtx is done",
			waitedBy: "tick's own wg.Wait()",
			errorTo:  "recorded by probeLeg as a canary outcome observation, never returned",
			panicTo:  "unrecovered",
		},
		{
			stops:    "probeLeg returns once its own bounded probe completes or the shared tickCtx is done",
			waitedBy: "tick's own wg.Wait()",
			errorTo:  "recorded by probeLeg as a canary outcome observation, never returned",
			panicTo:  "unrecovered",
		},
	},
	{"internal/health", "(*Server).Start"}: {
		{
			stops:    "httpSrv.Serve returns once Shutdown closes the listener",
			waitedBy: "Shutdown's own stop goroutine, which reads from serveErr",
			errorTo:  "serveErr, folded into shutdownErr by Shutdown's stop goroutine",
			panicTo:  "unrecovered",
		},
	},
	{"internal/health", "(*Server).Shutdown"}: {
		{
			stops:    "httpSrv.Shutdown(ctx) returns and the serve goroutine's own error is read",
			waitedBy: "every caller of Shutdown, via the shared done channel each caller's own select races against its ctx",
			errorTo:  "shutdownErr, returned by Shutdown once done is closed",
			panicTo:  "unrecovered",
		},
	},
	{"internal/ingest", "(*Loop).getUpdatesStoppable"}: {
		{
			stops:    "pollCtx.Done(), reached via the deferred cancel or the stop channel",
			waitedBy: "this same function's own receive on watchDone before it returns",
			errorTo:  "not applicable — the goroutine returns nothing, only closes watchDone",
			panicTo:  "unrecovered",
		},
	},
	{"internal/scheduler", "(*Worker).executeOne"}: {
		{
			stops:    "the savepoint helper it runs returns, bounded by the deadline context when the handler itself respects it",
			waitedBy: "the enclosing select's result case, or the deadline watchdog launch below",
			errorTo:  "a buffered result channel, read by the settlement path or by the watchdog once the deadline already fired",
			panicTo:  "unrecovered — the scheduler's own handler-panic recovery is tracked as separate follow-up work",
		},
		{
			stops:    "the bounded connection close returns, bounded by its own named-constant timeout",
			waitedBy: "nothing — this is the module's one deliberately detached launch",
			errorTo:  "discarded",
			panicTo:  "unrecovered",
		},
	},
	{"internal/tg", "(jsonConstructor).MultipartRequest"}: {
		{
			stops:    "the body writer and the multipart writer's own Close both return, then the pipe write end is closed with the folded error",
			waitedBy: "the pipe reader (the returned body stream) draining or erroring, never a channel or a cleanup",
			errorTo:  "folded into the reader's own next Read error via the pipe's CloseWithError",
			panicTo:  "unrecovered",
		},
	},
	{"internal/tgtest", "New"}: {
		{
			stops:    "the server and its listener are both closed, which the tb.Cleanup this function registers does",
			waitedBy: "that same tb.Cleanup",
			errorTo:  "tb.Errorf, failing the test — never swallowed",
			panicTo:  "unrecovered",
		},
	},
}

// funcDeclName spells fd the way this guard keys it: a receiver method
// as "(*Type).Method" or "(Type).Method", a plain function by its bare
// name.
func funcDeclName(fd *ast.FuncDecl) string {
	if fd.Recv == nil || len(fd.Recv.List) == 0 {
		return fd.Name.Name
	}
	recvType := fd.Recv.List[0].Type
	if star, ok := recvType.(*ast.StarExpr); ok {
		return fmt.Sprintf("(*%s).%s", identName(star.X), fd.Name.Name)
	}
	return fmt.Sprintf("(%s).%s", identName(recvType), fd.Name.Name)
}

// identName returns e's identifier name, for the receiver-type shapes
// this module actually writes (a bare identifier, pointer already
// unwrapped by the caller).
func identName(e ast.Expr) string {
	if ident, ok := e.(*ast.Ident); ok {
		return ident.Name
	}
	return fmt.Sprintf("%v", e)
}

// enclosingFuncDecl returns the top-level function declaration among
// decls whose span contains pos, or nil when none does — the only
// shape a package-level function literal's go statement can produce,
// since Go func literals are expressions, never declarations, and so
// never themselves bound a "go statement's own function."
func enclosingFuncDecl(decls []*ast.FuncDecl, pos token.Pos) *ast.FuncDecl {
	for _, fd := range decls {
		if fd.Pos() <= pos && pos <= fd.End() {
			return fd
		}
	}
	return nil
}

// launchSite is one go statement found while walking a tree: the key
// its enclosing function resolves to, and a "file:line" location for a
// failure message.
type launchSite struct {
	key launchKey
	loc string
}

// parseGoFile parses path with its own token.FileSet, so callers can
// resolve a token.Pos back to a line number.
func parseGoFile(t testing.TB, path string) (*token.FileSet, *ast.File) {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return fset, f
}

// collectLaunches walks root's compiled non-test source (no Go test
// file, no path the go tool itself never compiles) and returns every
// go statement found, each attributed to its
// outermost enclosing function declaration. unowned holds the location
// of any go statement with no such declaration — a package-level
// function-literal initializer is the reachable shape.
func collectLaunches(t testing.TB, root string) (sites []launchSite, unowned []string) {
	t.Helper()
	srcguard.WalkSubtree(t, root, func(path string) {
		if !srcguard.NonTestFile(path) {
			return
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			t.Fatalf("Rel(%s): %v", path, err)
		}
		if srcguard.ExcludedByDirName(rel) {
			return
		}

		fset, f := parseGoFile(t, path)
		dir := filepath.ToSlash(filepath.Dir(rel))

		var goStmts []*ast.GoStmt
		ast.Inspect(f, func(n ast.Node) bool {
			if g, ok := n.(*ast.GoStmt); ok {
				goStmts = append(goStmts, g)
			}
			return true
		})
		if len(goStmts) == 0 {
			return
		}
		sort.Slice(goStmts, func(i, j int) bool { return goStmts[i].Pos() < goStmts[j].Pos() })

		var funcDecls []*ast.FuncDecl
		for _, decl := range f.Decls {
			if fd, ok := decl.(*ast.FuncDecl); ok {
				funcDecls = append(funcDecls, fd)
			}
		}

		for _, g := range goStmts {
			loc := fmt.Sprintf("%s:%d", filepath.ToSlash(rel), fset.Position(g.Pos()).Line)
			owner := enclosingFuncDecl(funcDecls, g.Pos())
			if owner == nil {
				unowned = append(unowned, loc)
				continue
			}
			sites = append(sites, launchSite{key: launchKey{dir: dir, name: funcDeclName(owner)}, loc: loc})
		}
	})
	return sites, unowned
}

// checkLaunches walks root and checks every go statement it finds
// against table, returning one problem description per finding — empty
// once every launch is accounted for.
func checkLaunches(t testing.TB, root string, table map[launchKey][]launchAnswer) []string {
	t.Helper()
	sites, unowned := collectLaunches(t, root)

	var problems []string
	for _, loc := range unowned {
		problems = append(problems, fmt.Sprintf("%s: go statement has no enclosing function declaration", loc))
	}

	byKey := map[launchKey][]string{}
	for _, s := range sites {
		byKey[s.key] = append(byKey[s.key], s.loc)
	}

	for key, locs := range byKey {
		answers, ok := table[key]
		if !ok {
			problems = append(problems, fmt.Sprintf("%s %s: makes %d launch(es) with no allow-list row (%s)", key.dir, key.name, len(locs), strings.Join(locs, ", ")))
			continue
		}
		if len(answers) != len(locs) {
			problems = append(problems, fmt.Sprintf("%s %s: makes %d launch(es), allow-list row answers %d", key.dir, key.name, len(locs), len(answers)))
			continue
		}
		for i, a := range answers {
			if missing := a.missingField(); missing != "" {
				problems = append(problems, fmt.Sprintf("%s %s: launch %d's %q answer is empty", key.dir, key.name, i+1, missing))
			}
		}
	}
	sort.Strings(problems)
	return problems
}

func TestLaunchGuard_RealTreePasses(t *testing.T) {
	t.Parallel()
	root := repotest.Root(t)
	if problems := checkLaunches(t, root, launchTable); len(problems) != 0 {
		t.Errorf("launch guard problems:\n%s", strings.Join(problems, "\n"))
	}
}

func TestLaunchGuard_UnknownKeyFails(t *testing.T) {
	t.Parallel()
	root, _ := srcguard.WriteScratchFile(t, "pkg/f.go", "package pkg\n\nfunc Launch() {\n\tgo func() {}()\n}\n")

	problems := checkLaunches(t, root, map[launchKey][]launchAnswer{})
	if len(problems) == 0 {
		t.Fatal("want a problem for a launch with no allow-list row, got none")
	}
	if !strings.Contains(problems[0], "pkg") || !strings.Contains(problems[0], "Launch") {
		t.Errorf("problem = %q, want it to name the package and the function", problems[0])
	}
}

func TestLaunchGuard_EmptyAnswerFails(t *testing.T) {
	t.Parallel()
	root, _ := srcguard.WriteScratchFile(t, "pkg/f.go", "package pkg\n\nfunc Launch() {\n\tgo func() {}()\n}\n")
	table := map[launchKey][]launchAnswer{
		{dir: "pkg", name: "Launch"}: {{stops: "x", waitedBy: "", errorTo: "y", panicTo: "z"}},
	}

	problems := checkLaunches(t, root, table)
	if len(problems) == 0 {
		t.Fatal("want a problem for the empty waitedBy answer, got none")
	}
}

func TestLaunchGuard_TooFewAnswersFails(t *testing.T) {
	t.Parallel()
	root, _ := srcguard.WriteScratchFile(t, "pkg/f.go", "package pkg\n\nfunc Launch() {\n\tgo func() {}()\n\tgo func() {}()\n}\n")
	table := map[launchKey][]launchAnswer{
		{dir: "pkg", name: "Launch"}: {{stops: "x", waitedBy: "y", errorTo: "z", panicTo: "w"}},
	}

	problems := checkLaunches(t, root, table)
	if len(problems) == 0 {
		t.Fatal("want a problem for a function whose row under-answers its own launches, got none")
	}
}

func TestLaunchGuard_NestedFuncLiteralAttributedToOuterDecl(t *testing.T) {
	t.Parallel()
	src := "package pkg\n\nfunc Launch() {\n\tf := func() {\n\t\tgo func() {}()\n\t}\n\tf()\n}\n"
	root, _ := srcguard.WriteScratchFile(t, "pkg/f.go", src)
	table := map[launchKey][]launchAnswer{
		{dir: "pkg", name: "Launch"}: {{stops: "x", waitedBy: "y", errorTo: "z", panicTo: "w"}},
	}

	if problems := checkLaunches(t, root, table); len(problems) != 0 {
		t.Errorf("want the launch inside the nested function literal attributed to Launch with no problem, got %v", problems)
	}
}

func TestLaunchGuard_NoEnclosingFuncDeclFails(t *testing.T) {
	t.Parallel()
	src := "package pkg\n\nvar _ = func() int {\n\tgo func() {}()\n\treturn 0\n}()\n"
	root, _ := srcguard.WriteScratchFile(t, "pkg/f.go", src)

	problems := checkLaunches(t, root, map[launchKey][]launchAnswer{})
	if len(problems) == 0 {
		t.Fatal("want a problem for a go statement with no enclosing function declaration, got none")
	}
	if !strings.Contains(problems[0], "f.go") {
		t.Errorf("problem = %q, want it to name the file", problems[0])
	}
}

func TestLaunchGuard_ExcludedPathsAreNotGatedAndOthersAre(t *testing.T) {
	t.Parallel()
	root, _ := srcguard.WriteScratchFile(t, "pkg/f_test.go", "package pkg\n\nfunc TestX() {\n\tgo func() {}()\n}\n")
	_ = srcguard.WriteScratchFileIn(t, root, "pkg/testdata/g.go", "package testdata\n\nfunc Launch() {\n\tgo func() {}()\n}\n")
	_ = srcguard.WriteScratchFileIn(t, root, "pkg/_hidden/h.go", "package hidden\n\nfunc Launch() {\n\tgo func() {}()\n}\n")
	_ = srcguard.WriteScratchFileIn(t, root, "pkg/main.go", "package pkg\n\nfunc Launch() {\n\tgo func() {}()\n}\n")

	sites, unowned := collectLaunches(t, root)
	if len(unowned) != 0 {
		t.Fatalf("unowned = %v, want none", unowned)
	}
	if len(sites) != 1 {
		t.Fatalf("sites = %v, want exactly the one launch in pkg/main.go", sites)
	}
	if sites[0].key != (launchKey{dir: "pkg", name: "Launch"}) {
		t.Errorf("sites[0].key = %+v, want {pkg Launch}", sites[0].key)
	}
}

func TestLaunchGuard_JoinedFormsAreNotGoStatements(t *testing.T) {
	t.Parallel()
	src := "package pkg\n\nimport \"sync\"\n\nfunc Launch() {\n\tvar wg sync.WaitGroup\n\twg.Add(1)\n\twg.Done()\n\twg.Wait()\n}\n"
	root, _ := srcguard.WriteScratchFile(t, "pkg/f.go", src)

	sites, unowned := collectLaunches(t, root)
	if len(sites) != 0 || len(unowned) != 0 {
		t.Errorf("sites=%v unowned=%v, want none — no go statement was written", sites, unowned)
	}
}

// -----------------------------------------------------------------------
// The lint-configuration guard: the exclusion reach, the exclusion
// scoping, the enabled linter set and the pinned settings this task
// added.
// -----------------------------------------------------------------------

// mapAt descends m through keys, returning nil the moment a key is
// absent or its value is not itself a map.
func mapAt(m map[string]any, keys ...string) map[string]any {
	cur := m
	for _, k := range keys {
		v, ok := cur[k]
		if !ok {
			return nil
		}
		next, ok := v.(map[string]any)
		if !ok {
			return nil
		}
		cur = next
	}
	return cur
}

// stringsAt reads a string-list value nested under keys, returning nil
// when the path is absent or not a list of strings.
func stringsAt(m map[string]any, keys ...string) []string {
	if len(keys) == 0 {
		return nil
	}
	parent := mapAt(m, keys[:len(keys)-1]...)
	if parent == nil {
		return nil
	}
	return stringsFromAny(parent[keys[len(keys)-1]])
}

func stringsFromAny(v any) []string {
	list, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(list))
	for _, item := range list {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func containsString(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

// exclusionRules returns linters.exclusions.rules as a list of maps,
// one per rule.
func exclusionRules(m map[string]any) []map[string]any {
	parent := mapAt(m, "linters", "exclusions")
	if parent == nil {
		return nil
	}
	list, ok := parent["rules"].([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(list))
	for _, item := range list {
		if rule, ok := item.(map[string]any); ok {
			out = append(out, rule)
		}
	}
	return out
}

// forbidPatterns returns every configured forbidigo pattern string.
func forbidPatterns(m map[string]any) []string {
	parent := mapAt(m, "linters", "settings", "forbidigo")
	if parent == nil {
		return nil
	}
	list, ok := parent["forbid"].([]any)
	if !ok {
		return nil
	}
	var out []string
	for _, item := range list {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if p, ok := entry["pattern"].(string); ok {
			out = append(out, p)
		}
	}
	return out
}

// anyPatternMatches reports whether any of patterns, compiled as a
// regexp, matches target — the robust way to check a forbidigo pattern
// covers an identifier regardless of exactly how it is anchored.
func anyPatternMatches(patterns []string, target string) bool {
	for _, p := range patterns {
		re, err := regexp.Compile(p)
		if err != nil {
			continue
		}
		if re.MatchString(target) {
			return true
		}
	}
	return false
}

// isZeroInt reports whether m[key] decodes to the integer 0.
func isZeroInt(m map[string]any, key string) bool {
	if m == nil {
		return false
	}
	switch v := m[key].(type) {
	case int:
		return v == 0
	case int64:
		return v == 0
	case float64:
		return v == 0
	default:
		return false
	}
}

// internalNonTestFiles returns every compiled non-test Go source file
// under root's internal package tree, root-relative and
// "/"-separated — the same rendering golangci-lint uses once the
// relative-path mode is pinned to the module root.
func internalNonTestFiles(t testing.TB, root string) []string {
	t.Helper()
	var out []string
	srcguard.WalkSubtree(t, root, func(path string) {
		rel, err := filepath.Rel(root, path)
		if err != nil {
			t.Fatalf("Rel(%s): %v", path, err)
		}
		relSlash := filepath.ToSlash(rel)
		if !strings.HasPrefix(relSlash, "internal/") {
			return
		}
		if !srcguard.NonTestFile(path) {
			return
		}
		if srcguard.ExcludedByDirName(rel) {
			return
		}
		out = append(out, relSlash)
	})
	return out
}

// checkLintConfig loads the YAML file at path and checks it against
// every assertion this guard owns, returning one problem description
// per finding.
func checkLintConfig(t testing.TB, root, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		t.Fatalf("yaml.Unmarshal(%s): %v", path, err)
	}

	var problems []string
	nonTestFiles := internalNonTestFiles(t, root)

	for i, rule := range exclusionRules(raw) {
		pathPattern, _ := rule["path"].(string)
		if pathPattern == "" {
			continue
		}
		linters := stringsFromAny(rule["linters"])
		if len(linters) == 0 {
			problems = append(problems, fmt.Sprintf("exclusion rule %d (path %q) has no linters list — this switches off every linter for that path", i, pathPattern))
			continue
		}
		re, err := regexp.Compile(pathPattern)
		if err != nil {
			problems = append(problems, fmt.Sprintf("exclusion rule %d: path %q does not compile: %v", i, pathPattern, err))
			continue
		}
		for _, f := range nonTestFiles {
			if re.MatchString(f) {
				problems = append(problems, fmt.Sprintf("exclusion rule %d (path %q) matches non-test file %s under internal/", i, pathPattern, f))
			}
		}
	}

	enable := stringsAt(raw, "linters", "enable")
	for _, want := range []string{"containedctx", "fatcontext", "forbidigo", "gocritic"} {
		if !containsString(enable, want) {
			problems = append(problems, fmt.Sprintf("linters.enable is missing %q", want))
		}
	}
	if gocriticChecks := stringsAt(raw, "linters", "settings", "gocritic", "enabled-checks"); !containsString(gocriticChecks, "deferInLoop") {
		problems = append(problems, `gocritic.enabled-checks is missing "deferInLoop"`)
	}
	if govetChecks := stringsAt(raw, "linters", "settings", "govet", "enable"); !containsString(govetChecks, "nilness") {
		problems = append(problems, `govet.enable is missing "nilness"`)
	}

	forbid := forbidPatterns(raw)
	for _, want := range []string{"context.Background", "context.TODO", "time.Tick", "time.After"} {
		if !anyPatternMatches(forbid, want) {
			problems = append(problems, fmt.Sprintf("forbidigo.forbid has no pattern matching %q", want))
		}
	}

	relPathMode, _ := mapAt(raw, "run")["relative-path-mode"].(string)
	if relPathMode != "gomod" {
		problems = append(problems, fmt.Sprintf("run.relative-path-mode = %q, want \"gomod\"", relPathMode))
	}
	issues := mapAt(raw, "issues")
	if !isZeroInt(issues, "max-issues-per-linter") {
		problems = append(problems, "issues.max-issues-per-linter is not pinned to 0")
	}
	if !isZeroInt(issues, "max-same-issues") {
		problems = append(problems, "issues.max-same-issues is not pinned to 0")
	}

	sort.Strings(problems)
	return problems
}

func TestLintConfigGuard_RealConfigPasses(t *testing.T) {
	t.Parallel()
	root := repotest.Root(t)
	path := filepath.Join(root, ".golangci.yml")
	if problems := checkLintConfig(t, root, path); len(problems) != 0 {
		t.Errorf("lint-config guard problems:\n%s", strings.Join(problems, "\n"))
	}
}

// configOpts builds a scratch lint configuration that passes every
// assertion by default — each discriminating test copies it and
// breaks exactly one dimension.
type configOpts struct {
	enable          []string
	gocriticChecks  []string
	govetChecks     []string
	forbidPatterns  []string
	omitRelPathMode bool
	relPathMode     string
	omitMaxIssues   bool
	maxIssues       int
	omitMaxSame     bool
	maxSame         int
	extraExclusion  string
}

func defaultConfigOpts() configOpts {
	return configOpts{
		enable:         []string{"containedctx", "fatcontext", "forbidigo", "gocritic"},
		gocriticChecks: []string{"deferInLoop"},
		govetChecks:    []string{"nilness"},
		forbidPatterns: []string{`^context\.Background$`, `^context\.TODO$`, `^time\.Tick$`, `^time\.After$`},
		relPathMode:    "gomod",
	}
}

func without(list []string, s string) []string {
	out := make([]string, 0, len(list))
	for _, v := range list {
		if v != s {
			out = append(out, v)
		}
	}
	return out
}

func renderConfig(o configOpts) string {
	var b strings.Builder
	b.WriteString("linters:\n  enable:\n")
	for _, e := range o.enable {
		fmt.Fprintf(&b, "    - %s\n", e)
	}
	b.WriteString("  settings:\n    gocritic:\n      enabled-checks:\n")
	for _, c := range o.gocriticChecks {
		fmt.Fprintf(&b, "        - %s\n", c)
	}
	b.WriteString("    govet:\n      enable:\n")
	for _, c := range o.govetChecks {
		fmt.Fprintf(&b, "        - %s\n", c)
	}
	b.WriteString("    forbidigo:\n      forbid:\n")
	for _, p := range o.forbidPatterns {
		fmt.Fprintf(&b, "        - pattern: '%s'\n", p)
	}
	b.WriteString("  exclusions:\n    rules:\n      - path: _test\\.go\n        linters:\n          - forbidigo\n      - path: ^cmd/\n        linters:\n          - forbidigo\n")
	b.WriteString(o.extraExclusion)
	if !o.omitRelPathMode {
		fmt.Fprintf(&b, "run:\n  relative-path-mode: %s\n", o.relPathMode)
	}
	b.WriteString("issues:\n")
	if !o.omitMaxIssues {
		fmt.Fprintf(&b, "  max-issues-per-linter: %d\n", o.maxIssues)
	}
	if !o.omitMaxSame {
		fmt.Fprintf(&b, "  max-same-issues: %d\n", o.maxSame)
	}
	return b.String()
}

func writeScratchConfig(t testing.TB, root string, o configOpts) string {
	t.Helper()
	return srcguard.WriteScratchFileIn(t, root, "golangci.yml", renderConfig(o))
}

func TestLintConfigGuard_BaselinePasses(t *testing.T) {
	t.Parallel()
	root, _ := srcguard.WriteScratchFile(t, "internal/pkg/f.go", "package pkg\n")
	path := writeScratchConfig(t, root, defaultConfigOpts())

	if problems := checkLintConfig(t, root, path); len(problems) != 0 {
		t.Errorf("baseline config problems:\n%s", strings.Join(problems, "\n"))
	}
}

func TestLintConfigGuard_ExclusionMatchingInternalNonTestFileFails(t *testing.T) {
	t.Parallel()
	root, _ := srcguard.WriteScratchFile(t, "internal/pkg/f.go", "package pkg\n")
	opts := defaultConfigOpts()
	opts.extraExclusion = "      - path: internal/pkg/f\\.go\n        linters:\n          - forbidigo\n"
	path := writeScratchConfig(t, root, opts)

	problems := checkLintConfig(t, root, path)
	if len(problems) == 0 {
		t.Fatal("want a problem for an exclusion matching a non-test file under internal/, got none")
	}
}

func TestLintConfigGuard_ExclusionWithNoLintersListFails(t *testing.T) {
	t.Parallel()
	root, _ := srcguard.WriteScratchFile(t, "internal/pkg/f.go", "package pkg\n")
	opts := defaultConfigOpts()
	opts.extraExclusion = "      - path: internal/pkg/other\\.go\n"
	path := writeScratchConfig(t, root, opts)

	problems := checkLintConfig(t, root, path)
	if len(problems) == 0 {
		t.Fatal("want a problem for an exclusion rule with a path and no linters list, got none")
	}
}

func TestLintConfigGuard_MissingEnabledLinterFails(t *testing.T) {
	t.Parallel()
	for _, missing := range []string{"containedctx", "fatcontext", "forbidigo", "gocritic"} {
		t.Run(missing, func(t *testing.T) {
			t.Parallel()
			root, _ := srcguard.WriteScratchFile(t, "internal/pkg/f.go", "package pkg\n")
			opts := defaultConfigOpts()
			opts.enable = without(opts.enable, missing)
			path := writeScratchConfig(t, root, opts)

			problems := checkLintConfig(t, root, path)
			if len(problems) == 0 {
				t.Fatalf("want a problem for a config missing %q from linters.enable, got none", missing)
			}
			if !strings.Contains(strings.Join(problems, "\n"), missing) {
				t.Errorf("problems = %v, want one naming %q", problems, missing)
			}
		})
	}
}

func TestLintConfigGuard_MissingGocriticCheckFails(t *testing.T) {
	t.Parallel()
	root, _ := srcguard.WriteScratchFile(t, "internal/pkg/f.go", "package pkg\n")
	opts := defaultConfigOpts()
	opts.gocriticChecks = nil
	path := writeScratchConfig(t, root, opts)

	if problems := checkLintConfig(t, root, path); len(problems) == 0 {
		t.Fatal("want a problem for a config missing gocritic's deferInLoop check, got none")
	}
}

func TestLintConfigGuard_MissingGovetAnalyzerFails(t *testing.T) {
	t.Parallel()
	root, _ := srcguard.WriteScratchFile(t, "internal/pkg/f.go", "package pkg\n")
	opts := defaultConfigOpts()
	opts.govetChecks = nil
	path := writeScratchConfig(t, root, opts)

	if problems := checkLintConfig(t, root, path); len(problems) == 0 {
		t.Fatal("want a problem for a config missing govet's nilness analyzer, got none")
	}
}

func TestLintConfigGuard_MissingForbidigoPatternFails(t *testing.T) {
	t.Parallel()
	root, _ := srcguard.WriteScratchFile(t, "internal/pkg/f.go", "package pkg\n")
	opts := defaultConfigOpts()
	opts.forbidPatterns = without(opts.forbidPatterns, `^time\.After$`)
	path := writeScratchConfig(t, root, opts)

	if problems := checkLintConfig(t, root, path); len(problems) == 0 {
		t.Fatal("want a problem for a config missing the time.After forbidigo pattern, got none")
	}
}

func TestLintConfigGuard_RelativePathModeFails(t *testing.T) {
	t.Parallel()
	for name, mutate := range map[string]func(*configOpts){
		"absent":      func(o *configOpts) { o.omitRelPathMode = true },
		"wrong value": func(o *configOpts) { o.relPathMode = "cfg" },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root, _ := srcguard.WriteScratchFile(t, "internal/pkg/f.go", "package pkg\n")
			opts := defaultConfigOpts()
			mutate(&opts)
			path := writeScratchConfig(t, root, opts)

			if problems := checkLintConfig(t, root, path); len(problems) == 0 {
				t.Fatalf("want a problem for run.relative-path-mode %s, got none", name)
			}
		})
	}
}

func TestLintConfigGuard_IssueCapsFail(t *testing.T) {
	t.Parallel()
	for name, mutate := range map[string]func(*configOpts){
		"max-issues-per-linter absent":   func(o *configOpts) { o.omitMaxIssues = true },
		"max-issues-per-linter non-zero": func(o *configOpts) { o.maxIssues = 5 },
		"max-same-issues absent":         func(o *configOpts) { o.omitMaxSame = true },
		"max-same-issues non-zero":       func(o *configOpts) { o.maxSame = 5 },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root, _ := srcguard.WriteScratchFile(t, "internal/pkg/f.go", "package pkg\n")
			opts := defaultConfigOpts()
			mutate(&opts)
			path := writeScratchConfig(t, root, opts)

			if problems := checkLintConfig(t, root, path); len(problems) == 0 {
				t.Fatalf("want a problem for %s, got none", name)
			}
		})
	}
}
