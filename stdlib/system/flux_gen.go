// DO NOT EDIT: This file is autogenerated via the builtin command.

package system

import (
	ast "github.com/influxdata/flux/ast"
	runtime "github.com/influxdata/flux/runtime"
)

func init() {
	runtime.RegisterPackage(pkgAST)
}

var pkgAST = &ast.Package{
	BaseNode: ast.BaseNode{
		Comments: nil,
		Errors:   nil,
		Loc:      nil,
	},
	Files: []*ast.File{&ast.File{
		BaseNode: ast.BaseNode{
			Comments: nil,
			Errors:   nil,
			Loc: &ast.SourceLocation{
				End: ast.Position{
					Column: 13,
					Line:   3,
				},
				File:   "system.flux",
				Source: "package system\n\nbuiltin time",
				Start: ast.Position{
					Column: 1,
					Line:   1,
				},
			},
		},
		Body: []ast.Statement{&ast.BuiltinStatement{
			BaseNode: ast.BaseNode{
				Comments: nil,
				Errors:   nil,
				Loc: &ast.SourceLocation{
					End: ast.Position{
						Column: 13,
						Line:   3,
					},
					File:   "system.flux",
					Source: "builtin time",
					Start: ast.Position{
						Column: 1,
						Line:   3,
					},
				},
			},
			ID: &ast.Identifier{
				BaseNode: ast.BaseNode{
					Comments: nil,
					Errors:   nil,
					Loc: &ast.SourceLocation{
						End: ast.Position{
							Column: 13,
							Line:   3,
						},
						File:   "system.flux",
						Source: "time",
						Start: ast.Position{
							Column: 9,
							Line:   3,
						},
					},
				},
				Name: "time",
			},
			Ty: ast.TypeExpression{
				BaseNode: ast.BaseNode{
					Comments: nil,
					Errors:   nil,
					Loc: &ast.SourceLocation{
						End: ast.Position{
							Column: 26,
							Line:   3,
						},
						File:   "system.flux",
						Source: "() => time",
						Start: ast.Position{
							Column: 16,
							Line:   3,
						},
					},
				},
				Constraints: []*ast.TypeConstraint{},
				Ty: &ast.FunctionType{
					BaseNode: ast.BaseNode{
						Comments: nil,
						Errors:   nil,
						Loc: &ast.SourceLocation{
							End: ast.Position{
								Column: 26,
								Line:   3,
							},
							File:   "system.flux",
							Source: "() => time",
							Start: ast.Position{
								Column: 16,
								Line:   3,
							},
						},
					},
					Parameters: []*ast.ParameterType{},
					Return: &ast.NamedType{
						BaseNode: ast.BaseNode{
							Comments: nil,
							Errors:   nil,
							Loc: &ast.SourceLocation{
								End: ast.Position{
									Column: 26,
									Line:   3,
								},
								File:   "system.flux",
								Source: "time",
								Start: ast.Position{
									Column: 22,
									Line:   3,
								},
							},
						},
						ID: &ast.Identifier{
							BaseNode: ast.BaseNode{
								Comments: nil,
								Errors:   nil,
								Loc: &ast.SourceLocation{
									End: ast.Position{
										Column: 26,
										Line:   3,
									},
									File:   "system.flux",
									Source: "time",
									Start: ast.Position{
										Column: 22,
										Line:   3,
									},
								},
							},
							Name: "time",
						},
					},
				},
			},
		}},
		Imports:  nil,
		Metadata: "parser-type=rust",
		Name:     "system.flux",
		Package: &ast.PackageClause{
			BaseNode: ast.BaseNode{
				Comments: nil,
				Errors:   nil,
				Loc: &ast.SourceLocation{
					End: ast.Position{
						Column: 15,
						Line:   1,
					},
					File:   "system.flux",
					Source: "package system",
					Start: ast.Position{
						Column: 1,
						Line:   1,
					},
				},
			},
			Name: &ast.Identifier{
				BaseNode: ast.BaseNode{
					Comments: nil,
					Errors:   nil,
					Loc: &ast.SourceLocation{
						End: ast.Position{
							Column: 15,
							Line:   1,
						},
						File:   "system.flux",
						Source: "system",
						Start: ast.Position{
							Column: 9,
							Line:   1,
						},
					},
				},
				Name: "system",
			},
		},
	}},
	Package: "system",
	Path:    "system",
}
