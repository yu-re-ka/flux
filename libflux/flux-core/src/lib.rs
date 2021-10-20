#![cfg_attr(feature = "strict", deny(warnings, missing_docs))]

//! This crate performs parsing and semantic analysis of Flux source
//! code. It forms the core of the compiler for the [Flux language].
//! It is made up of five modules. Four of these handle the analysis
//! of Flux code during compilation:
//!
//! - [`scanner`] produces tokens from plain source code;
//! - [`parser`] forms the abstract syntax tree (AST);
//! - [`ast`] defines the AST data structures and provides functions for its analysis; and
//! - [`semantic`] performs semantic analysis, including type inference,
//!   producing a semantic graph.
//!
//! In addition, the [`formatter`] module provides functions for code formatting utilities.
//!
//! [Flux language]: https://github.com/influxdata/flux

extern crate chrono;
extern crate derive_more;
extern crate fnv;
#[macro_use]
extern crate serde_derive;
extern crate serde_aux;

pub mod ast;
pub mod formatter;
pub mod parser;
pub mod scanner;
pub mod semantic;

use anyhow::{bail, Result};

use fnv::FnvHasher;
use std::hash::BuildHasherDefault;

pub use ast::DEFAULT_PACKAGE_NAME;

type DefaultHasher = BuildHasherDefault<FnvHasher>;

/// merge_packages takes an input package and an output package, checks that the package
/// clauses match and merges the files from the input package into the output package. If
/// package clauses fail validation then an option with an Error is returned.
pub fn merge_packages(out_pkg: &mut ast::Package, in_pkg: &mut ast::Package) -> Result<()> {
    let out_pkg_name = if let Some(pc) = &out_pkg.files[0].package {
        &pc.name.name
    } else {
        DEFAULT_PACKAGE_NAME
    };

    // Check that all input files have a package clause that matches the output package.
    for file in &in_pkg.files {
        match file.package.as_ref() {
            Some(pc) => {
                let in_pkg_name = &pc.name.name;
                if in_pkg_name != out_pkg_name {
                    bail!(
                        r#"error at {}: file is in package "{}", but other files are in package "{}""#,
                        pc.base.location,
                        in_pkg_name,
                        out_pkg_name
                    );
                }
            }
            None => {
                if out_pkg_name != DEFAULT_PACKAGE_NAME {
                    bail!(
                        r#"error at {}: file is in default package "{}", but other files are in package "{}""#,
                        file.base.location,
                        DEFAULT_PACKAGE_NAME,
                        out_pkg_name
                    );
                }
            }
        };
    }
    out_pkg.files.append(&mut in_pkg.files);
    Ok(())
}

use std::{
    collections::HashMap,
    ops::{Add, Sub},
};

enum Expr {
    Literal(Value),
    Identifier(String),
    Op(Box<Expr>, Op, Box<Expr>),
}

enum Op {
    Add,
    Subtract,
}

#[derive(Clone, Debug, PartialEq)]
enum Value {
    Int(i64),
    Float(f64),
}

impl Add for Value {
    type Output = anyhow::Result<Self>;
    fn add(self, other: Self) -> Self::Output {
        Ok(match (self, other) {
            (Value::Int(l), Value::Int(r)) => Value::Int(l + r),
            (Value::Float(l), Value::Float(r)) => Value::Float(l + r),
            _ => return Err(anyhow::anyhow!("Typemismatch")),
        })
    }
}

impl Sub for Value {
    type Output = anyhow::Result<Self>;
    fn sub(self, other: Self) -> Self::Output {
        Ok(match (self, other) {
            (Value::Int(l), Value::Int(r)) => Value::Int(l - r),
            (Value::Float(l), Value::Float(r)) => Value::Float(l - r),
            _ => return Err(anyhow::anyhow!("Typemismatch")),
        })
    }
}

trait VectorValue:
    Add<Output = anyhow::Result<Self>> + Sub<Output = anyhow::Result<Self>> + Sized + Clone
{
    type Context;
    fn from_value(v: Value, context: &Self::Context) -> Self;
}

impl VectorValue for Value {
    type Context = ();
    fn from_value(v: Value, _: &Self::Context) -> Self {
        v
    }
}

#[derive(Clone, Debug, PartialEq)]
enum Vector {
    Int(Vec<i64>),
    Float(Vec<f64>),
}

impl VectorValue for Vector {
    // Need to know the length to construct a vector (alternative we could have singleton variants
    // in `Vector` which act as a vector of whatever the input length is)
    type Context = usize;
    fn from_value(v: Value, len: &Self::Context) -> Self {
        match v {
            Value::Int(i) => Self::Int(vec![i; *len]),
            Value::Float(i) => Self::Float(vec![i; *len]),
        }
    }
}

impl Add for Vector {
    type Output = anyhow::Result<Self>;
    fn add(self, other: Self) -> Self::Output {
        Ok(match (self, other) {
            (Self::Int(l), Self::Int(r)) => {
                Self::Int(l.into_iter().zip(r).map(|(l, r)| l + r).collect())
            }
            (Self::Float(l), Self::Float(r)) => {
                Self::Float(l.into_iter().zip(r).map(|(l, r)| l + r).collect())
            }
            _ => return Err(anyhow::anyhow!("Typemismatch")),
        })
    }
}

impl Sub for Vector {
    type Output = anyhow::Result<Self>;
    fn sub(self, other: Self) -> Self::Output {
        Ok(match (self, other) {
            (Self::Int(l), Self::Int(r)) => {
                Self::Int(l.into_iter().zip(r).map(|(l, r)| l - r).collect())
            }
            (Self::Float(l), Self::Float(r)) => {
                Self::Float(l.into_iter().zip(r).map(|(l, r)| l - r).collect())
            }
            _ => return Err(anyhow::anyhow!("Typemismatch")),
        })
    }
}

impl Expr {
    fn int(i: i64) -> Expr {
        Expr::Literal(Value::Int(i))
    }

    fn float(f: f64) -> Expr {
        Expr::Literal(Value::Float(f))
    }

    fn identifier(i: &str) -> Expr {
        Expr::Identifier(i.to_owned())
    }

    fn op(l: impl Into<Box<Expr>>, op: Op, r: impl Into<Box<Expr>>) -> Expr {
        Expr::Op(l.into(), op, r.into())
    }

    fn eval<V>(&self, env: &mut HashMap<String, V>, context: &V::Context) -> anyhow::Result<V>
    where
        V: VectorValue,
    {
        Ok(match self {
            Expr::Literal(v) => V::from_value(v.clone(), context),
            Expr::Identifier(i) => env
                .get(i)
                .ok_or_else(|| anyhow::anyhow!("Missing `{}`", i))?
                .clone(),
            Expr::Op(l, op, r) => match op {
                Op::Add => (l.eval(env, context)? + r.eval(env, context)?)?,
                Op::Subtract => (l.eval(env, context)? - r.eval(env, context)?)?,
            },
        })
    }
}

/// Test
pub fn test() {
    let int_expr = Expr::op(Expr::int(1), Op::Add, Expr::identifier("x"));
    let float_expr = Expr::op(Expr::float(1.), Op::Subtract, Expr::identifier("x"));
    macro_rules! collect {
        ($($expr: expr),* $(,)?) => {
            std::array::IntoIter::new([$($expr),*]).collect()
        }
    }
    assert_eq!(
        int_expr
            .eval(&mut collect![("x".into(), Value::Int(2))], &())
            .unwrap(),
        Value::Int(3)
    );

    assert_eq!(
        float_expr
            .eval(
                &mut vec![("x".into(), Value::Float(3.))].into_iter().collect(),
                &()
            )
            .unwrap(),
        Value::Float(1. - 3.)
    );

    assert_eq!(
        int_expr
            .eval(
                &mut vec![("x".into(), Vector::Int(vec![2, 3]))]
                    .into_iter()
                    .collect(),
                &2
            )
            .unwrap(),
        Vector::Int(vec![3, 4])
    );

    assert_eq!(
        float_expr
            .eval(
                &mut vec![("x".into(), Vector::Float(vec![3., 4., 5.]))]
                    .into_iter()
                    .collect(),
                &3
            )
            .unwrap(),
        Vector::Float(vec![1. - 3., 1. - 4., 1. - 5.])
    );
}

#[cfg(test)]
mod tests {
    use super::merge_packages;
    use crate::ast;

    #[test]
    fn vectorize() {
        super::test();
    }

    #[test]
    fn ok_merge_multi_file() {
        let in_script = "package foo\na = 1\n";
        let out_script = "package foo\nb = 2\n";

        let in_file = crate::parser::parse_string("test".to_string(), in_script);
        let out_file = crate::parser::parse_string("test".to_string(), out_script);
        let mut in_pkg = ast::Package {
            base: Default::default(),
            path: "./test".to_string(),
            package: "foo".to_string(),
            files: vec![in_file.clone()],
        };
        let mut out_pkg = ast::Package {
            base: Default::default(),
            path: "./test".to_string(),
            package: "foo".to_string(),
            files: vec![out_file.clone()],
        };
        merge_packages(&mut out_pkg, &mut in_pkg).unwrap();
        let got = out_pkg.files;
        let want = vec![out_file, in_file];
        assert_eq!(want, got);
    }

    #[test]
    fn ok_merge_one_default_pkg() {
        // Make sure we can merge one file with default "main"
        // and on explicit
        let has_clause_script = "package main\nx = 32";
        let no_clause_script = "y = 32";
        let has_clause_file =
            crate::parser::parse_string("has_clause.flux".to_string(), has_clause_script);
        let no_clause_file =
            crate::parser::parse_string("no_clause.flux".to_string(), no_clause_script);
        {
            let mut out_pkg: ast::Package = has_clause_file.clone().into();
            let mut in_pkg: ast::Package = no_clause_file.clone().into();
            merge_packages(&mut out_pkg, &mut in_pkg).unwrap();
            let got = out_pkg.files;
            let want = vec![has_clause_file.clone(), no_clause_file.clone()];
            assert_eq!(want, got);
        }
        {
            // Same as previous test, but reverse order
            let mut out_pkg: ast::Package = no_clause_file.clone().into();
            let mut in_pkg: ast::Package = has_clause_file.clone().into();
            merge_packages(&mut out_pkg, &mut in_pkg).unwrap();
            let got = out_pkg.files;
            let want = vec![no_clause_file.clone(), has_clause_file.clone()];
            assert_eq!(want, got);
        }
    }

    #[test]
    fn ok_no_in_pkg() {
        let out_script = "package foo\nb = 2\n";

        let out_file = crate::parser::parse_string("test".to_string(), out_script);
        let mut in_pkg = ast::Package {
            base: Default::default(),
            path: "./test".to_string(),
            package: "foo".to_string(),
            files: vec![],
        };
        let mut out_pkg = ast::Package {
            base: Default::default(),
            path: "./test".to_string(),
            package: "foo".to_string(),
            files: vec![out_file.clone()],
        };
        merge_packages(&mut out_pkg, &mut in_pkg).unwrap();
        let got = out_pkg.files;
        let want = vec![out_file];
        assert_eq!(want, got);
    }

    #[test]
    fn err_no_out_pkg_clause() {
        let in_script = "package foo\na = 1\n";
        let out_script = "";

        let in_file = crate::parser::parse_string("test_in.flux".to_string(), in_script);
        let out_file = crate::parser::parse_string("test_out.flux".to_string(), out_script);
        let mut in_pkg = ast::Package {
            base: Default::default(),
            path: "./test".to_string(),
            package: "foo".to_string(),
            files: vec![in_file.clone()],
        };
        let mut out_pkg = ast::Package {
            base: Default::default(),
            path: "./test".to_string(),
            package: "foo".to_string(),
            files: vec![out_file.clone()],
        };
        let got_err = merge_packages(&mut out_pkg, &mut in_pkg)
            .unwrap_err()
            .to_string();
        let want_err = r#"error at test_in.flux@1:1-1:12: file is in package "foo", but other files are in package "main""#;
        assert_eq!(got_err.to_string(), want_err);
    }

    #[test]
    fn err_no_in_pkg_clause() {
        let in_script = "a = 1000\n";
        let out_script = "package foo\nb = 100\n";

        let in_file = crate::parser::parse_string("test_in.flux".to_string(), in_script);
        let out_file = crate::parser::parse_string("test_out.flux".to_string(), out_script);
        let mut in_pkg = ast::Package {
            base: Default::default(),
            path: "./test".to_string(),
            package: "foo".to_string(),
            files: vec![in_file.clone()],
        };
        let mut out_pkg = ast::Package {
            base: Default::default(),
            path: "./test".to_string(),
            package: "foo".to_string(),
            files: vec![out_file.clone()],
        };
        let got_err = merge_packages(&mut out_pkg, &mut in_pkg)
            .unwrap_err()
            .to_string();
        let want_err = r#"error at test_in.flux@1:1-1:9: file is in default package "main", but other files are in package "foo""#;
        assert_eq!(got_err.to_string(), want_err);
    }

    #[test]
    fn ok_no_pkg_clauses() {
        let in_script = "a = 100\n";
        let out_script = "b = a * a\n";
        let in_file = crate::parser::parse_string("test".to_string(), in_script);
        let out_file = crate::parser::parse_string("test".to_string(), out_script);
        let mut in_pkg = ast::Package {
            base: Default::default(),
            path: "./test".to_string(),
            package: "foo".to_string(),
            files: vec![in_file.clone()],
        };
        let mut out_pkg = ast::Package {
            base: Default::default(),
            path: "./test".to_string(),
            package: "foo".to_string(),
            files: vec![out_file.clone()],
        };
        merge_packages(&mut out_pkg, &mut in_pkg).unwrap();
        assert_eq!(2, out_pkg.files.len());
    }
}
