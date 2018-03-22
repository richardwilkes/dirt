package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
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
		if f, err := parser.ParseFile(fset, one, nil, parser.ParseComments); err == nil {
			if len(disallowedImports) > 0 {
				for _, imp := range f.Imports {
					if disallowedImports[strings.Trim(imp.Path.Value, `"`)] {
						if imp.Comment == nil || !strings.Contains(imp.Comment.Text(), "@allow") {
							hadIssue = true
							l.lineChan <- problem{prefix: disallowPrefix, output: fmt.Sprintf("%v: Import of %s not allowed", fset.Position(imp.Pos()), imp.Path.Value)}
						}
					}
				}
			}
			if len(disallowedFunctions) > 0 {
				ast.Inspect(f, func(node ast.Node) bool {
					switch x := node.(type) {
					case *ast.CallExpr:
						var name string
						var pos token.Pos
						switch c := x.Fun.(type) {
						case *ast.Ident:
							name = c.Name
							pos = c.Pos()
						case *ast.SelectorExpr:
							if sx, ok := c.X.(*ast.Ident); ok {
								name = sx.Name + "."
							}
							name += c.Sel.Name
						}
						if name != "" {
							if disallowedFunctions[name] {
								line := fset.Position(pos).Line
								ignore := false
								for _, one := range f.Comments {
									if line == fset.Position(one.Pos()).Line && strings.Contains(one.Text(), "@allow") {
										ignore = true
										break
									}
								}
								if !ignore {
									hadIssue = true
									l.lineChan <- problem{prefix: disallowPrefix, output: fmt.Sprintf(`%v: Use of "%s" not allowed`, fset.Position(pos), name)}
								}
							}
						}
					}
					return true
				})
			}
		}
	}
	if hadIssue {
		l.markError()
	}
}
