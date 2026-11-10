package main

import (
	"go/ast"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// OSExitAnalyzer forbids a direct os.Exit call in the main function of package main.
var OSExitAnalyzer = &analysis.Analyzer{
	Name: "osexit",
	Doc: `forbid a direct os.Exit call in the main function of package main

A violation is an os.Exit call in the body of main.main, including inside closures
and through a renamed or dot import, and os.Exit taken as a value there (f := os.Exit),
which hides a later call. Calls in other functions and packages,
and in the generated go test main, do not count.
An indirect exit (log.Fatal, an slog logger with Fatal, a C._exit call via cgo)
is not caught: it is a deliberate exit, visible in the code by its name.
No flags.`,
	Run: runOSExit,
}

// runOSExit reports every os.Exit call and every use of os.Exit as a value in the body of the main function
// of package main. It skips files without the .go extension; the result is always nil, nil.
func runOSExit(pass *analysis.Pass) (any, error) {
	if pass.Pkg.Name() != "main" {
		return nil, nil
	}
	for _, file := range pass.Files {
		if !strings.HasSuffix(pass.Fset.File(file.Pos()).Name(), ".go") {
			continue
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || fn.Name.Name != "main" || fn.Body == nil {
				continue
			}
			// Inspect visits a call before its function name, so a name already reported as a call is skipped.
			called := map[*ast.Ident]bool{}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				switch n := n.(type) {
				case *ast.CallExpr:
					if id := osExitIdent(pass.TypesInfo, n.Fun); id != nil {
						called[id] = true
						pass.Reportf(n.Pos(), "direct os.Exit call in main")
					}
				case *ast.Ident:
					if !called[n] && osExitIdent(pass.TypesInfo, n) != nil {
						pass.Reportf(n.Pos(), "os.Exit used as a value in main")
					}
				}
				return true
			})
		}
	}
	return nil, nil
}

// osExitIdent returns the name in expr that refers to os.Exit, under any import name, or nil.
func osExitIdent(info *types.Info, expr ast.Expr) *ast.Ident {
	var id *ast.Ident
	switch e := expr.(type) {
	case *ast.SelectorExpr:
		id = e.Sel
	case *ast.Ident:
		id = e
	default:
		return nil
	}
	f, ok := info.Uses[id].(*types.Func)
	if !ok || f.Pkg() == nil || f.Pkg().Path() != "os" || f.Name() != "Exit" {
		return nil
	}
	return id
}
