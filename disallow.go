package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"sync/atomic"
)

const disallowPrefix = "disallow"

func (l *lint) checkDisallowed() {
	disallowedImports := make(map[string]bool)
	for _, one := range l.disallowedImports {
		disallowedImports[one] = true
	}
	disallowedFunctions := make(map[string]bool)
	for _, one := range l.disallowedFunctions {
		disallowedFunctions[one] = true
	}
	hadIssue := false
	for _, one := range l.files {
		fset := token.NewFileSet()
		if f, err := parser.ParseFile(fset, one, nil, 0); err == nil {
			if len(disallowedImports) > 0 {
				for _, imp := range f.Imports {
					if disallowedImports[strings.Trim(imp.Path.Value, `"`)] {
						hadIssue = true
						l.lineChan <- problem{prefix: disallowPrefix, output: fmt.Sprintf("%v: Import of %s not allowed", fset.Position(imp.Pos()), imp.Path.Value)}
					}
				}
			}
			if len(disallowedFunctions) > 0 {
				ast.Inspect(f, func(node ast.Node) bool {
					switch x := node.(type) {
					case *ast.CallExpr:
						switch c := x.Fun.(type) {
						case *ast.Ident:
							if disallowedFunctions[c.Name] {
								hadIssue = true
								l.lineChan <- problem{prefix: disallowPrefix, output: fmt.Sprintf(`%v: Use of "%s" not allowed`, fset.Position(c.Pos()), c.Name)}
							}
						}
					}
					return true
				})
			}
		}
	}
	if hadIssue {
		atomic.StoreInt32(&l.status, 1)
	}
}
