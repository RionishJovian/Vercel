package handler

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The runtime database role has a service policy (USING true) on every table, so row security
// protects a query only inside runInTenant / WithTenant (role "authenticated"). A handler that
// queries a tenant table straight on the *database.DB, without a business_id in the SQL, sees
// every business. This test makes each such query a conscious decision: it must appear in
// directTenantReads with its reason, or the test fails. Add the business_id filter or move the
// query into runInTenant instead of extending the list.
//
// shortcut: it reads the first SQL string literal and matches the receiver by the name "db";
// a query built from a variable, or on a differently named database field, is not seen.
var directTenantReads = map[string]string{
	"account_pass_renewals.go:PlatformHandler.AccountRenewalInbox":      "platform operator inbox of pending app pass requests across all businesses; DEVELOPER role checked first",
	"account_pass_renewals.go:PlatformHandler.decideAccountRenewal":     "checks that the deciding user is an active platform operator, in any business",
	"telegram_renewals.go:AccountRenewalDecider.ResolveRequestBusiness": "finds which business owns a request id from a platform Telegram tap; the approver is authorized by the platform binding",
	"telegram_renewals.go:PlatformHandler.sendAccountRenewal":           "platform Telegram alert for one pending request, system context",
	"telegram_renewals.go:PlatformHandler.RunRenewalNotifier":           "system retry loop for undelivered platform alerts, across businesses",
	"platform_handler.go:PlatformHandler.PutPricing":                    "writes one platform audit row for a DEVELOPER-only price publish, filed under the signed-in operator's own business",
	"guest_wifi_handler.go:GuestWifiHandler.RunOwnerAlertRetry":         "system retry loop for undelivered owner alerts; each alert goes only to that request's own business bot",
}

var (
	// A real filter on the caller's business: business_id = $n, or business_id IN / ANY (...).
	businessFilter = regexp.MustCompile(`business_id\s*(=\s*\$\d|=\s*any\s*\(|in\s*\()`)
	sqlTableRef    = regexp.MustCompile(`\b(?:from|join|into|update)\s+(?:only\s+)?(?:public\.)?([a-z_][a-z0-9_]*)`)
	createTableRe  = regexp.MustCompile(`(?is)create table (?:if not exists )?(?:public\.)?(\w+)\s*\((.*?)\n\);`)
)

// tenantTables are the tables that have a business_id column, read from the real schema files.
func tenantTables(t *testing.T) map[string]bool {
	t.Helper()
	files := []string{"../../db/baseline.sql"}
	more, err := filepath.Glob("../../db/migrations/*.sql")
	if err != nil {
		t.Fatal(err)
	}
	files = append(files, more...)
	out := map[string]bool{}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range createTableRe.FindAllStringSubmatch(string(raw), -1) {
			if strings.Contains(m[2], "business_id") {
				out[strings.ToLower(m[1])] = true
			}
		}
	}
	if len(out) < 40 {
		t.Fatalf("only %d tenant tables found in the schema files; the parser is broken", len(out))
	}
	return out
}

func stringLiterals(e ast.Expr) string {
	var b strings.Builder
	ast.Inspect(e, func(n ast.Node) bool {
		if l, ok := n.(*ast.BasicLit); ok && l.Kind == token.STRING {
			b.WriteString(strings.Trim(l.Value, "`\""))
			b.WriteString(" ")
		}
		return true
	})
	return strings.ToLower(b.String())
}

func lastName(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.SelectorExpr:
		return x.Sel.Name
	}
	return ""
}

func TestHandlersDoNotQueryTenantTablesWithoutATenantOrFilter(t *testing.T) {
	tables := tenantTables(t)
	methods := map[string]int{"Query": 0, "QueryRow": 0, "Exec": 0, "QueryContext": 1, "QueryRowContext": 1, "ExecContext": 1}
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]string{}
	fset := token.NewFileSet()
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") || name == "tenant.go" {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			key := name + ":" + fn.Name.Name
			if fn.Recv != nil && len(fn.Recv.List) == 1 {
				recv := fn.Recv.List[0].Type
				if s, ok := recv.(*ast.StarExpr); ok {
					recv = s.X
				}
				key = name + ":" + lastName(recv) + "." + fn.Name.Name
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				idx, isQuery := methods[sel.Sel.Name]
				if !isQuery || strings.ToLower(lastName(sel.X)) != "db" || len(call.Args) <= idx {
					return true
				}
				sql := stringLiterals(call.Args[idx])
				if sql == "" || businessFilter.MatchString(sql) {
					return true
				}
				for _, m := range sqlTableRef.FindAllStringSubmatch(sql, -1) {
					if tables[m[1]] {
						found[key] = m[1]
						break
					}
				}
				return true
			})
		}
	}
	var unlisted, stale []string
	for k, table := range found {
		if _, ok := directTenantReads[k]; !ok {
			unlisted = append(unlisted, k+" (table "+table+")")
		}
	}
	for k := range directTenantReads {
		if _, ok := found[k]; !ok {
			stale = append(stale, k)
		}
	}
	sort.Strings(unlisted)
	sort.Strings(stale)
	if len(unlisted) > 0 {
		t.Errorf("these handlers query a tenant table directly with no business_id in the SQL and no tenant transaction.\nAdd the filter, move the query into runInTenant, or (only if it is a deliberate platform or owner read) list it in directTenantReads with the reason:\n  %s", strings.Join(unlisted, "\n  "))
	}
	if len(stale) > 0 {
		t.Errorf("directTenantReads lists queries that no longer exist; remove them:\n  %s", strings.Join(stale, "\n  "))
	}
}
