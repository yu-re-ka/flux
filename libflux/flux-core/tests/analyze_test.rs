use fluxcore::ast;
use fluxcore::semantic::convert_source;
use fluxcore::semantic::nodes::*;
use fluxcore::semantic::types::{Function, MonoType, Parameter, SemanticMap, Tvar};
use fluxcore::semantic::walk::{walk_mut, NodeMut};

use pretty_assertions::assert_eq;

#[test]
fn analyze_end_to_end() {
    let mut got = convert_source(
        r#"
n = 1
s = "string"
f = (a) => a + a
f(a: n)
f(a: s)
        "#,
    )
    .unwrap();
    let f_type = Function {
        positional: vec![Parameter {
            name: Some("a".to_string()),
            typ: MonoType::Var(Tvar(4)),
            required: true,
        }],
        named: SemanticMap::new(),
        retn: MonoType::Var(Tvar(4)),
    };
    let f_call_int_type = Function {
        positional: vec![],
        named: fluxcore::semantic_map! {
            "a".to_string() => Parameter{
                name: Some("a".to_string()),
                typ: MonoType::Int,
                required: true,
            },
        },
        retn: MonoType::Int,
    };
    let f_call_string_type = Function {
        positional: vec![],
        named: fluxcore::semantic_map! {
            "a".to_string() => Parameter{
                name: Some("a".to_string()),
                typ: MonoType::String,
                required: true,
            },
        },
        retn: MonoType::String,
    };
    let want = Package {
        loc: ast::BaseNode::default().location,
        package: "main".to_string(),
        files: vec![File {
            loc: ast::BaseNode::default().location,
            package: None,
            imports: Vec::new(),
            body: vec![
                Statement::Variable(Box::new(VariableAssgn::new(
                    Identifier {
                        loc: ast::BaseNode::default().location,
                        name: "n".to_string(),
                    },
                    Expression::Integer(IntegerLit {
                        loc: ast::BaseNode::default().location,
                        value: 1,
                    }),
                    ast::BaseNode::default().location,
                ))),
                Statement::Variable(Box::new(VariableAssgn::new(
                    Identifier {
                        loc: ast::BaseNode::default().location,
                        name: "s".to_string(),
                    },
                    Expression::StringLit(StringLit {
                        loc: ast::BaseNode::default().location,
                        value: "string".to_string(),
                    }),
                    ast::BaseNode::default().location,
                ))),
                Statement::Variable(Box::new(VariableAssgn::new(
                    Identifier {
                        loc: ast::BaseNode::default().location,
                        name: "f".to_string(),
                    },
                    Expression::Function(Box::new(FunctionExpr {
                        loc: ast::BaseNode::default().location,
                        typ: MonoType::Fun(Box::new(f_type)),
                        params: vec![FunctionParameter {
                            loc: ast::BaseNode::default().location,
                            key: Identifier {
                                loc: ast::BaseNode::default().location,
                                name: "a".to_string(),
                            },
                            default: None,
                        }],
                        body: Block::Return(ReturnStmt {
                            loc: ast::BaseNode::default().location,
                            argument: Expression::Binary(Box::new(BinaryExpr {
                                loc: ast::BaseNode::default().location,
                                typ: MonoType::Var(Tvar(4)),
                                operator: ast::Operator::AdditionOperator,
                                left: Expression::Identifier(IdentifierExpr {
                                    loc: ast::BaseNode::default().location,
                                    typ: MonoType::Var(Tvar(4)),
                                    name: "a".to_string(),
                                }),
                                right: Expression::Identifier(IdentifierExpr {
                                    loc: ast::BaseNode::default().location,
                                    typ: MonoType::Var(Tvar(4)),
                                    name: "a".to_string(),
                                }),
                            })),
                        }),
                    })),
                    ast::BaseNode::default().location,
                ))),
                Statement::Expr(ExprStmt {
                    loc: ast::BaseNode::default().location,
                    expression: Expression::Call(Box::new(CallExpr {
                        loc: ast::BaseNode::default().location,
                        typ: MonoType::Int,
                        callee: Expression::Identifier(IdentifierExpr {
                            loc: ast::BaseNode::default().location,
                            typ: MonoType::Fun(Box::new(f_call_int_type)),
                            name: "f".to_string(),
                        }),
                        positional: vec![],
                        named: vec![Property {
                            loc: ast::BaseNode::default().location,
                            key: Identifier {
                                loc: ast::BaseNode::default().location,
                                name: "a".to_string(),
                            },
                            value: Expression::Identifier(IdentifierExpr {
                                loc: ast::BaseNode::default().location,
                                typ: MonoType::Int,
                                name: "n".to_string(),
                            }),
                        }],
                    })),
                }),
                Statement::Expr(ExprStmt {
                    loc: ast::BaseNode::default().location,
                    expression: Expression::Call(Box::new(CallExpr {
                        loc: ast::BaseNode::default().location,
                        typ: MonoType::String,
                        callee: Expression::Identifier(IdentifierExpr {
                            loc: ast::BaseNode::default().location,
                            typ: MonoType::Fun(Box::new(f_call_string_type)),
                            name: "f".to_string(),
                        }),
                        positional: vec![],
                        named: vec![Property {
                            loc: ast::BaseNode::default().location,
                            key: Identifier {
                                loc: ast::BaseNode::default().location,
                                name: "a".to_string(),
                            },
                            value: Expression::Identifier(IdentifierExpr {
                                loc: ast::BaseNode::default().location,
                                typ: MonoType::String,
                                name: "s".to_string(),
                            }),
                        }],
                    })),
                }),
            ],
        }],
    };
    // We don't want to test the locations, so we override those with the base one.
    walk_mut(
        &mut |n: &mut NodeMut| n.set_loc(ast::BaseNode::default().location),
        &mut NodeMut::Package(&mut got),
    );
    assert_eq!(want, got);
}
