package mail

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestCheckSendAllowed_AllowsUnderTheCap(t *testing.T) {
	if refusal := checkSendAllowed(nil, "u1", "mb1", maxRecipientsPerMessage-1); refusal != nil {
		t.Fatalf("under the cap must be allowed, got %v", refusal)
	}
}

// The boundary is `>`, so exactly maxRecipientsPerMessage is allowed. This
// matches the Rcpt handler, which refuses the recipient that would make the
// list exceed 100 rather than the hundredth itself.
func TestCheckSendAllowed_AllowsExactlyTheCap(t *testing.T) {
	if refusal := checkSendAllowed(nil, "u1", "mb1", maxRecipientsPerMessage); refusal != nil {
		t.Fatalf("exactly the cap must be allowed, got %v", refusal)
	}
}

func TestCheckSendAllowed_RefusesOverTheCap(t *testing.T) {
	refusal := checkSendAllowed(nil, "u1", "mb1", maxRecipientsPerMessage+1)
	if refusal == nil {
		t.Fatal("over the cap must be refused")
	}
	if refusal.kind != sendErrForbidden {
		t.Errorf("kind = %v, want sendErrForbidden", refusal.kind)
	}
	// The refusal text is the whole UX of this feature — a user who cannot
	// tell why the send failed will retry it unchanged.
	if !strings.Contains(refusal.msg, "100") {
		t.Errorf("refusal should name the limit, got %q", refusal.msg)
	}
}

// A refusal must reach the client as a transient 4xx. A 5xx would make a mail
// client hard-bounce a message the user can send once the list is split,
// generating a bounce to their own correspondent.
func TestSMTPErrorForRefusal_IsTransient(t *testing.T) {
	err := smtpErrorForRefusal(&sendError{kind: sendErrForbidden, msg: "too many recipients"})
	if err.Code < 400 || err.Code >= 500 {
		t.Fatalf("Code = %d, want a 4xx transient refusal", err.Code)
	}
	if err.Code != 452 {
		t.Errorf("Code = %d, want 452", err.Code)
	}
	if err.EnhancedCode != [3]int{4, 5, 3} {
		t.Errorf("EnhancedCode = %v, want {4,5,3}", err.EnhancedCode)
	}
	if err.Message != "too many recipients" {
		t.Errorf("Message = %q, want the refusal's own message", err.Message)
	}
}

// The HTTP path had no recipient cap at all, and the hole was specifically
// that Cc and Bcc went uncounted — three lists of 40 are 120 recipients, and
// each is under any per-list limit. This pins that the call site sums all
// three, which a test needing a live provider could not reach.
func TestSendMessage_CountsToCcAndBcc(t *testing.T) {
	args := gateCallArgs(t, "endpoints_send.go", "sendMessage")
	for _, field := range []string{"p.To", "p.Cc", "p.Bcc"} {
		if !strings.Contains(args, field) {
			t.Errorf("the HTTP path must count %s toward the cap; got %q", field, args)
		}
	}
}

// The SMTP path must count the deduped bcc slice, not s.recipients. The
// envelope includes addresses that also appear in To/Cc, so using it would
// double-count them and refuse sends well under the real fan-out.
func TestSMTPData_CountsDedupedBccNotTheRawEnvelope(t *testing.T) {
	args := gateCallArgs(t, "smtp_session.go", "Data")
	if strings.Contains(args, "s.recipients") {
		t.Errorf("the SMTP path must not count s.recipients (double-counts To/Cc); got %q", args)
	}
	for _, field := range []string{"msg.To", "msg.Cc", "bcc"} {
		if !strings.Contains(args, field) {
			t.Errorf("the SMTP path must count %s toward the cap; got %q", field, args)
		}
	}
}

// gateCallArgs returns the source text of the recipient-count argument passed
// to checkSendAllowed inside the named function.
func gateCallArgs(t *testing.T, filename, funcName string) string {
	t.Helper()

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", filename, err)
	}

	src, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("read %s: %v", filename, err)
	}

	var found string
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil || fn.Name.Name != funcName {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			ident, ok := call.Fun.(*ast.Ident)
			if !ok || ident.Name != "checkSendAllowed" {
				return true
			}
			last := call.Args[len(call.Args)-1]
			found = string(src[fset.Position(last.Pos()).Offset:fset.Position(last.End()).Offset])
			return false
		})
	}

	if found == "" {
		t.Fatalf("no checkSendAllowed call found in %s:%s — if the gate moved, update this test rather than deleting it", filename, funcName)
	}
	return found
}

// TestEveryProviderSendIsGated is the structural half of the send gate.
//
// The gate binds no hook and sits on no chokepoint the runtime enforces — it
// is honoured only by its call sites. The failure mode is silent: a new
// outbound path that calls provider.Send without checkSendAllowed sends
// unmetered mail, and every test of the normal path still passes. This test
// reads the package's own source and asserts that each function calling
// provider.Send also calls the gate, so a fourth send path fails CI at the
// moment it is written rather than when it is abused.
//
// It matches any `.Send(` call outside the Provider implementations, so
// renaming the receiver, holding the Provider in a struct field, or taking a
// method value do not evade it, and it requires the gate call to appear
// before the send rather than merely somewhere in the same function.
//
// What it still does not prove: that the gate is unconditional. A gate inside
// `if false` above a send would pass. That needs real dominance analysis;
// this is the cheap approximation, and the exemption list is where an honest
// escape hatch lives.
//
// To confirm this test still watches anything, add a bare provider.Send call
// to a scratch file in this package and check that it goes red. A structural
// test that has never failed may be looking at nothing.
func TestEveryProviderSendIsGated(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}

	fset := token.NewFileSet()
	var ungated []string
	gatedFns := map[string]bool{}

	// The Provider implementations: their Send IS the wire call, so there is
	// nothing above them to gate. Named explicitly rather than inferred — an
	// exemption you can read is stronger than a heuristic that silently grows
	// to cover a file it should not.
	providerImpls := map[string]bool{
		"postmark.go":      true,
		"smtp_provider.go": true,
		"noop.go":          true,
	}

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		if providerImpls[name] {
			continue
		}

		file, err := parser.ParseFile(fset, filepath.Join(".", name), nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}

		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}

			var sendPos token.Pos
			gatePos := token.NoPos

			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				switch fun := call.Fun.(type) {
				case *ast.SelectorExpr:
					// Any x.Send(...), whatever the receiver is named and
					// wherever it is held. Keying on a receiver named
					// `provider` was the earlier mistake: renaming the
					// variable, holding the Provider in a struct field, or
					// taking a method value all slipped past silently, which
					// is exactly the failure this test exists to catch.
					if fun.Sel.Name == "Send" {
						sendPos = call.Pos()
					}
				case *ast.Ident:
					if fun.Name == "checkSendAllowed" && !gatePos.IsValid() {
						gatePos = call.Pos()
					}
				}
				return true
			})

			// A method value (send := x.Send) never appears as a CallExpr, so
			// catch the bare selector too.
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				sel, ok := n.(*ast.SelectorExpr)
				if ok && sel.Sel.Name == "Send" && !sendPos.IsValid() {
					sendPos = sel.Pos()
				}
				return true
			})

			if !sendPos.IsValid() {
				continue
			}

			where := fset.Position(sendPos)
			switch {
			case !gatePos.IsValid():
				ungated = append(ungated, where.String()+" in "+fn.Name.Name+" (no gate call)")
			case gatePos > sendPos:
				// Presence is not enough: a gate below the send decides
				// nothing. This is a cheap approximation of dominance — it
				// does not prove the gate is unconditional, but it does
				// catch the ordering mistake.
				ungated = append(ungated, where.String()+" in "+fn.Name.Name+" (gate runs after the send)")
			default:
				gatedFns[name+":"+fn.Name.Name] = true
			}
		}
	}

	if len(ungated) > 0 {
		sort.Strings(ungated)
		t.Fatalf("these call provider.Send without calling checkSendAllowed:\n  %s\n\n"+
			"Every outbound path must call the gate immediately before provider.Send — see send_gate.go. "+
			"If a path genuinely does not send user mail, exempt it here with a comment saying why.",
			strings.Join(ungated, "\n  "))
	}

	// Guard against the test silently passing because it found nothing to
	// check — a refactor that renames the provider variable would otherwise
	// make this test vacuous rather than red.
	if len(gatedFns) < 2 {
		t.Fatalf("expected at least the two known send paths (HTTP compose and SMTP submission), found %d: %v\n"+
			"If a send path was removed, lower this floor deliberately. If the provider variable was renamed, "+
			"this test stopped matching and must be updated.", len(gatedFns), gatedFns)
	}
}
