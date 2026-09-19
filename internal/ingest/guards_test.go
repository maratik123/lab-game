package ingest

import (
	"go/ast"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/mymmrac/telego"

	"github.com/maratik123/lab-game/internal/repotest"
	"github.com/maratik123/lab-game/internal/srcguard"
)

// ingestNonTestFiles returns every non-test Go source file's path in
// this package.
func ingestNonTestFiles(t *testing.T) []string {
	t.Helper()
	return srcguard.PackageFiles(t, repotest.RootPath(t, filepath.Join("internal", "ingest")))
}

// walkIngestSource calls fn with each non-test file's path and raw
// content.
func walkIngestSource(t *testing.T, fn func(path string, content []byte)) {
	t.Helper()
	for _, path := range ingestNonTestFiles(t) {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", path, err)
		}
		fn(path, content)
	}
}

// parseIngestSource parses every non-test file into an *ast.File.
func parseIngestSource(t *testing.T) map[string]*ast.File {
	t.Helper()
	return srcguard.ParseFiles(t, ingestNonTestFiles(t))
}

// TestGuard_NoPanicLogFatalOrOsExit asserts that the package's non-test
// source contains no panic(, log.Fatal or os.Exit — recover
// raises none, so the panic index gains no row.
func TestGuard_NoPanicLogFatalOrOsExit(t *testing.T) {
	t.Parallel()
	forbidden := []string{"panic(", "log.Fatal", "os.Exit"}
	walkIngestSource(t, func(path string, content []byte) {
		for _, f := range forbidden {
			if strings.Contains(string(content), f) {
				t.Errorf("%s contains %q — production code must never panic/Fatal/Exit (design D7)", path, f)
			}
		}
	})
}

// TestGuard_NoMetricsLibraryImport asserts that this package imports no
// metrics library — it declares only the Observer interface; a future
// integration owns exposition.
func TestGuard_NoMetricsLibraryImport(t *testing.T) {
	t.Parallel()
	walkIngestSource(t, func(path string, content []byte) {
		if strings.Contains(string(content), "prometheus/client_golang") {
			t.Errorf("%s imports a metrics registry — internal/ingest must expose only the Observer interface", path)
		}
	})
}

// telegoUpdateKindExemptions are telego.Update's own exported fields
// that carry no kindTable row, named so a later reader meets the
// exemption rather than a silent hole: UpdateID is the update's own
// canonical id (int, not a payload field), and there is no unexported
// field reflect.Type.Field ever surfaces as exported (the guard below
// filters by PkgPath, but this comment states the intent for a reader
// who does not trace that filter).
var telegoUpdateKindExemptions = map[string]bool{
	"update_id": true,
}

// payloadDeclaresField reports whether payload (a *telego.X struct type,
// already unwrapped from the pointer) declares an exported field named
// fieldName. It does not check the field's own type — callers pass the
// exact name/kind combination the kind table's rule cares about (a Date
// int64 field,
// or a non-pointer Chat field), and a same-named field of a different
// type would itself be a signal worth a loud test failure, not a silent
// skip.
func payloadDeclaresField(payload reflect.Type, fieldName string) bool {
	_, ok := payload.FieldByName(fieldName)
	return ok
}

// TestGuard_TelegoUpdateFieldsMatchKindTable is the drift check:
// every exported pointer field of telego.Update has its json tag either
// as a kindTable row or in the named exemption set above, every
// kindTable row's token is a tag some field declares, and each row's
// date/chatID extractor presence matches whether its own payload TYPE
// declares a Date int64 field / a non-pointer Chat field — a kind whose
// payload declares neither carries nil. A telego bump that adds an
// update type, or a kindTable row whose extractors lag its payload's
// own fields, reds this test until the kind table learns it.
func TestGuard_TelegoUpdateFieldsMatchKindTable(t *testing.T) {
	t.Parallel()

	typ := reflect.TypeOf(telego.Update{})
	seenTags := make(map[string]bool)
	for i := range typ.NumField() {
		f := typ.Field(i)
		if f.PkgPath != "" {
			// Unexported (e.g. a riding context) — not a payload field.
			continue
		}
		tagName, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if tagName == "" || tagName == "-" {
			continue
		}
		if telegoUpdateKindExemptions[tagName] {
			continue
		}
		if f.Type.Kind() != reflect.Pointer {
			t.Errorf("telego.Update.%s (json tag %q) is not a pointer field and is not in telegoUpdateKindExemptions — kind.go's table assumes every payload field is a pointer", f.Name, tagName)
			continue
		}
		seenTags[tagName] = true
		if !knownKind(Kind(tagName)) {
			t.Errorf("telego.Update.%s (json tag %q) has no kindTable row — a Bot API update type kind.go has not learned (design D4)", f.Name, tagName)
			continue
		}

		row, ok := rowForKind(Kind(tagName))
		if !ok {
			// Unreachable: knownKind(Kind(tagName)) above already
			// confirmed rowForKind succeeds for this tag.
			continue
		}
		payload := f.Type.Elem()

		wantDate := payloadDeclaresField(payload, "Date")
		if dateField, ok := payload.FieldByName("Date"); ok && dateField.Type.Kind() != reflect.Int64 {
			wantDate = false
		}
		if wantDate && row.date == nil {
			t.Errorf("kindTable row %q: payload %s declares a Date field but the row's date extractor is nil (design D4)", tagName, payload)
		}
		if !wantDate && row.date != nil {
			t.Errorf("kindTable row %q: payload %s declares no Date field but the row supplies a date extractor (design D4)", tagName, payload)
		}

		wantChat := false
		if chatField, ok := payload.FieldByName("Chat"); ok && chatField.Type.Kind() != reflect.Pointer {
			wantChat = true
		}
		if wantChat && row.chatID == nil {
			t.Errorf("kindTable row %q: payload %s declares a non-pointer Chat field but the row's chatID extractor is nil (design D4)", tagName, payload)
		}
		if !wantChat && row.chatID != nil {
			t.Errorf("kindTable row %q: payload %s declares no non-pointer Chat field but the row supplies a chatID extractor (design D4)", tagName, payload)
		}
	}

	for _, row := range kindTable {
		if !seenTags[string(row.kind)] {
			t.Errorf("kindTable row %q names no exported pointer field of telego.Update", row.kind)
		}
	}
}

// pgxTxOrConnType reports whether e denotes pgx.Tx or pgx.Conn (by
// selector name only — this walk does not resolve imports, since every
// production file in this package imports pgx under the same alias).
func pgxTxOrConnType(e ast.Expr) bool {
	star, ok := e.(*ast.StarExpr)
	if ok {
		e = star.X
	}
	sel, ok := e.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok || pkg.Name != "pgx" {
		return false
	}
	return sel.Sel.Name == "Tx" || sel.Sel.Name == "Conn"
}

// TestGuard_NoTransactionEscapesTheHandlerContract asserts a structural
// property: no exported function or method of this package RETURNS a
// pgx.Tx or pgx.Conn, and no exported type declares an exported FIELD of
// either type — so the only transaction a Handler can reach is the one
// the loop passes it as Handle's parameter. DeadUpdates, which TAKES a
// caller-owned pgx.Tx, and Options.Pool, a *pgxpool.Pool (a
// connection source, not a transaction), are both legal under this
// walk because it checks results and fields, never parameters.
func TestGuard_NoTransactionEscapesTheHandlerContract(t *testing.T) {
	t.Parallel()

	for path, file := range parseIngestSource(t) {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || !fn.Name.IsExported() || fn.Recv != nil && !exportedReceiver(fn.Recv) {
				continue
			}
			if fn.Type.Results == nil {
				continue
			}
			for _, res := range fn.Type.Results.List {
				if pgxTxOrConnType(res.Type) {
					t.Errorf("%s: exported func/method %s returns a pgx.Tx/pgx.Conn — a Handler must only ever receive the loop's own transaction as a parameter (AC8)", path, fn.Name.Name)
				}
			}
		}

		ast.Inspect(file, func(n ast.Node) bool {
			ts, ok := n.(*ast.TypeSpec)
			if !ok || !ts.Name.IsExported() {
				return true
			}
			st, ok := ts.Type.(*ast.StructType)
			if !ok || st.Fields == nil {
				return true
			}
			for _, field := range st.Fields.List {
				if !pgxTxOrConnType(field.Type) {
					continue
				}
				for _, name := range field.Names {
					if name.IsExported() {
						t.Errorf("%s: exported type %s has exported field %s of type pgx.Tx/pgx.Conn — a Handler must only ever receive the loop's own transaction as a parameter (AC8)", path, ts.Name.Name, name.Name)
					}
				}
			}
			return true
		})
	}
}

// exportedReceiver reports whether recv's type is an exported (pointer
// to) identifier — an unexported receiver type's methods are not part of
// this package's exported surface even if the method name itself is
// capitalised.
func exportedReceiver(recv *ast.FieldList) bool {
	if len(recv.List) == 0 {
		return false
	}
	e := recv.List[0].Type
	if star, ok := e.(*ast.StarExpr); ok {
		e = star.X
	}
	ident, ok := e.(*ast.Ident)
	return ok && ident.IsExported()
}

// TestGuard_NoOwnBotAPIPath asserts this package's own half of a
// composition property: its non-test source constructs no
// telego.Bot, no http.Client and no telegoapi caller of its own — it
// reaches the Bot API only through the *telego.Bot that the Telegram
// client's own API() method returns. The Telegram-client half is already
// shipped in a sibling test and is not re-proven here.
func TestGuard_NoOwnBotAPIPath(t *testing.T) {
	t.Parallel()
	botConstruction := regexp.MustCompile(`telego\.NewBot\(|&?telego\.Bot\{|&?http\.Client\{`)
	walkIngestSource(t, func(path string, content []byte) {
		if strings.Contains(string(content), "telegoapi") {
			t.Errorf("%s imports/references telegoapi directly — the Bot API is reached only through tg.Client.API() (AC38)", path)
		}
		if m := botConstruction.FindString(string(content)); m != "" {
			t.Errorf("%s references %s — this package must not construct its own *telego.Bot or *http.Client (AC38)", path, m)
		}
	})
}

// ctxFirstExemptions names every exported method of this package that
// legitimately does not take ctx context.Context first, because it
// reaches neither the network nor the database — that predicate is
// not computable from an AST (reachability is whole-program), so this
// walk demands ctx first from every exported method and the exemption
// set is the only escape.
var ctxFirstExemptions = map[string]string{
	"(*Router).Kinds":      "returns the immutable, already-registered kind set — no I/O",
	"(*Router).Lookup":     "an in-memory map lookup — no I/O",
	"Outcome.String":       "pure rendering, no I/O",
	"(*OptionError).Error": "pure rendering, no I/O",
	"New":                  "a constructor: its parameter is Options, not ctx — no I/O until PollOnce/Run",
	"NewGate":              "a constructor — no I/O",
	"NewRouter":            "a constructor — no I/O",
	"NewUpdate":            "pure in-memory derivation from a raw telego.Update — no I/O",
	"Derive":               "pure in-memory derivation — no I/O",
	"Date":                 "pure in-memory derivation — no I/O",
	"ChatID":               "pure in-memory derivation — no I/O",
	"(*Loop).Stop":         "closes the stop channel — no I/O, and takes no ctx by design (the drain lever, mirroring the scheduler package's Worker/Liveness Stop)",
}

// TestGuard_CtxFirstAndNoRidingContext is this package's ctx-discipline
// walk: no type declared in this package has a context.Context
// field; every exported method takes ctx context.Context as its first
// parameter unless named in ctxFirstExemptions; no type embeds
// telego.Update (only a NAMED field is legal); and the package's
// non-test source contains no selector named Context or WithContext
// (telego.Update's own riding-context accessors), since reading or
// setting the riding context would silently escape both the loop's ctx
// and the per-attempt transaction.
func TestGuard_CtxFirstAndNoRidingContext(t *testing.T) {
	t.Parallel()

	for path, file := range parseIngestSource(t) {
		ast.Inspect(file, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.TypeSpec:
				st, ok := node.Type.(*ast.StructType)
				if !ok || st.Fields == nil {
					return true
				}
				for _, field := range st.Fields.List {
					if isContextContextType(field.Type) {
						name := "(embedded)"
						if len(field.Names) > 0 {
							name = field.Names[0].Name
						}
						t.Errorf("%s: type %s has a context.Context field %s — a struct must never store a context (AGENTS.md § API Naming)", path, node.Name.Name, name)
					}
					if isTelegoUpdateType(field.Type) && len(field.Names) == 0 {
						t.Errorf("%s: type %s embeds telego.Update — it must be a NAMED field (design D9)", path, node.Name.Name)
					}
				}
			case *ast.SelectorExpr:
				// Exclude the "context" package qualifier itself
				// (context.Context, context.Background, ...) — only a
				// CALL of telego.Update's own Context()/WithContext()
				// riding-context accessors is the concern here.
				if pkg, ok := node.X.(*ast.Ident); ok && pkg.Name == "context" {
					return true
				}
				if node.Sel.Name == "Context" || node.Sel.Name == "WithContext" {
					t.Errorf("%s: selector .%s — the riding context on a polled update is always context.Background() and must never be read or set (design D9)", path, node.Sel.Name)
				}
			}
			return true
		})

		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || !fn.Name.IsExported() {
				continue
			}
			qualified := fn.Name.Name
			if fn.Recv != nil && len(fn.Recv.List) > 0 {
				qualified = receiverTypeName(fn.Recv.List[0].Type) + "." + fn.Name.Name
			}
			if ctxFirstExemptions[qualified] != "" {
				continue
			}
			params := fn.Type.Params.List
			if len(params) == 0 || !isContextContextType(params[0].Type) {
				t.Errorf("%s: exported method/func %s does not take ctx context.Context first, and is not in ctxFirstExemptions (design D9, AC2)", path, qualified)
			}
		}
	}
}

func isContextContextType(e ast.Expr) bool {
	sel, ok := e.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "context" && sel.Sel.Name == "Context"
}

func isTelegoUpdateType(e ast.Expr) bool {
	sel, ok := e.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "telego" && sel.Sel.Name == "Update"
}

func receiverTypeName(e ast.Expr) string {
	if star, ok := e.(*ast.StarExpr); ok {
		return "(*" + receiverTypeName(star.X) + ")"
	}
	if ident, ok := e.(*ast.Ident); ok {
		return ident.Name
	}
	return "?"
}
