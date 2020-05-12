use crate::ast;
use crate::semantic::fresh::{Fresh, Fresher};
use crate::semantic::sub::{Substitutable, Substitution};

use std::{
    cmp,
    collections::{BTreeMap, BTreeSet},
    fmt,
};

// For use in generics where the specific type of map is not not mentioned.
pub type SemanticMap<K, V> = BTreeMap<K, V>;
pub type SemanticMapIter<'a, K, V> = std::collections::btree_map::Iter<'a, K, V>;

#[derive(Debug, Clone)]
pub struct PolyType {
    pub vars: Vec<Tvar>,
    pub cons: TvarKinds,
    pub expr: MonoType,
}

pub type PolyTypeMap = SemanticMap<String, PolyType>;
pub type PolyTypeMapMap = SemanticMap<String, SemanticMap<String, PolyType>>;

#[macro_export]
/// Alias the maplit literal construction macro so we can specify the type here.
macro_rules! semantic_map {
    ( $($x:tt)* ) => ( maplit::btreemap!( $($x)* ) );
}

impl fmt::Display for PolyType {
    fn fmt(&self, f: &mut fmt::Formatter) -> fmt::Result {
        let vars = &self
            .vars
            .iter()
            .map(|x| x.to_string())
            .collect::<Vec<_>>()
            .join(", ");
        if self.cons.is_empty() {
            write!(f, "forall [{}] {}", vars, self.expr)
        } else {
            write!(
                f,
                "forall [{}] where {} {}",
                vars,
                PolyType::display_constraints(&self.cons),
                self.expr
            )
        }
    }
}

impl PartialEq for PolyType {
    fn eq(&self, poly: &Self) -> bool {
        let a: Tvar = self.max_tvar();
        let b: Tvar = poly.max_tvar();

        let max = if a > b { a.0 } else { b.0 };

        let mut f = Fresher::from(max + 1);
        let mut g = Fresher::from(max + 1);

        let mut a = self.clone().fresh(&mut f, &mut TvarMap::new());
        let mut b = poly.clone().fresh(&mut g, &mut TvarMap::new());

        a.vars.sort();
        b.vars.sort();

        for kinds in a.cons.values_mut() {
            kinds.sort();
        }
        for kinds in b.cons.values_mut() {
            kinds.sort();
        }

        a.vars == b.vars && a.cons == b.cons && a.expr == b.expr
    }
}

impl Substitutable for PolyType {
    fn apply(self, sub: &Substitution) -> Self {
        PolyType {
            vars: self.vars,
            cons: self.cons,
            expr: self.expr.apply(sub),
        }
    }
    fn free_vars(&self) -> Vec<Tvar> {
        minus(&self.vars, self.expr.free_vars())
    }
}

impl MaxTvar for Vec<Tvar> {
    fn max_tvar(&self) -> Tvar {
        self.iter()
            .fold(Tvar(0), |max, tv| if *tv > max { *tv } else { max })
    }
}

impl MaxTvar for PolyType {
    fn max_tvar(&self) -> Tvar {
        vec![self.vars.max_tvar(), self.expr.max_tvar()].max_tvar()
    }
}

impl PolyType {
    fn display_constraints(cons: &TvarKinds) -> String {
        cons.iter()
            // A BTree produces a sorted iterator for
            // deterministic display output
            .collect::<BTreeMap<_, _>>()
            .iter()
            .map(|(&&tv, &kinds)| format!("{}:{}", tv, PolyType::display_kinds(kinds)))
            .collect::<Vec<_>>()
            .join(", ")
    }
    fn display_kinds(kinds: &[Kind]) -> String {
        kinds
            .iter()
            // Sort kinds with BTree
            .collect::<BTreeSet<_>>()
            .iter()
            .map(|x| x.to_string())
            .collect::<Vec<_>>()
            .join(" + ")
    }
}

pub fn union<T: PartialEq>(mut vars: Vec<T>, mut with: Vec<T>) -> Vec<T> {
    with.retain(|tv| !vars.contains(tv));
    vars.append(&mut with);
    vars
}

pub fn minus<T: PartialEq>(vars: &[T], mut from: Vec<T>) -> Vec<T> {
    from.retain(|tv| !vars.contains(tv));
    from
}

#[derive(Debug, PartialEq)]
pub enum Error {
    Record { err: RecordError },
    Arg { err: ArgError },
    Pipe { err: PipeError },
    Constraint { err: ConstraintError },
    NotEqual { exp: MonoType, act: MonoType },
    OccursCheck { tv: Tvar, ty: MonoType },
}

impl fmt::Display for Error {
    fn fmt(&self, f: &mut fmt::Formatter) -> fmt::Result {
        match self {
            Error::Record { err } => write!(f, "{}", err),
            Error::Arg { err } => write!(f, "{}", err),
            Error::Pipe { err } => write!(f, "{}", err),
            Error::Constraint { err } => write!(f, "{}", err),
            Error::NotEqual { exp, act } => match act {
                MonoType::Bool { loc } => write!(
                    f,
                    "error {}: types do not match\nExpected: {}\nFound: {}",
                    loc, exp, act
                ),
                MonoType::Int { loc } => write!(
                    f,
                    "error {}: types do not match\nExpected: {}\nFound: {}",
                    loc, exp, act
                ),
                MonoType::Uint { loc } => write!(
                    f,
                    "error {}: types do not match\nExpected: {}\nFound: {}",
                    loc, exp, act
                ),
                MonoType::Float { loc } => write!(
                    f,
                    "error {}: types do not match\nExpected: {}\nFound: {}",
                    loc, exp, act
                ),
                MonoType::String { loc } => write!(
                    f,
                    "error {}: types do not match\nExpected: {}\nFound: {}",
                    loc, exp, act
                ),
                MonoType::Duration { loc } => write!(
                    f,
                    "error {}: types do not match\nExpected: {}\nFound: {}",
                    loc, exp, act
                ),
                MonoType::Time { loc } => write!(
                    f,
                    "error {}: types do not match\nExpected: {}\nFound: {}",
                    loc, exp, act
                ),
                MonoType::Regexp { loc } => write!(
                    f,
                    "error {}: types do not match\nExpected: {}\nFound: {}",
                    loc, exp, act
                ),
                MonoType::Bytes { loc } => write!(
                    f,
                    "error {}: types do not match\nExpected: {}\nFound: {}",
                    loc, exp, act
                ),
                MonoType::Arr(arr) => write!(
                    f,
                    "error {}: types do not match\nExpected: {}\nFound: {}",
                    arr.loc(),
                    exp,
                    act
                ),
                MonoType::Obj(obj) => write!(
                    f,
                    "error {}: types do not match\nExpected: {}\nFound: {}",
                    obj.loc(),
                    exp,
                    act
                ),
                MonoType::Par(obj) => write!(
                    f,
                    "error {}: types do not match\nExpected: {}\nFound: {}",
                    obj.loc(),
                    exp,
                    act
                ),
                MonoType::Fnc(fun) => write!(
                    f,
                    "error {}: types do not match\nExpected: {}\nFound: {}",
                    fun.loc(),
                    exp,
                    act
                ),
            },
            Error::OccursCheck { tv, ty } => write!(f, "error: recursive type {} != {}", tv, ty),
        }
    }
}

// Kind represents a class or family of types
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
pub enum Kind {
    Addable,
    Subtractable,
    Divisible,
    Numeric,
    Comparable,
    Equatable,
    Nullable,
    Row,
    Negatable,
}

impl fmt::Display for Kind {
    fn fmt(&self, f: &mut fmt::Formatter) -> fmt::Result {
        match self {
            Kind::Addable => f.write_str("Addable"),
            Kind::Subtractable => f.write_str("Subtractable"),
            Kind::Divisible => f.write_str("Divisible"),
            Kind::Numeric => f.write_str("Numeric"),
            Kind::Comparable => f.write_str("Comparable"),
            Kind::Equatable => f.write_str("Equatable"),
            Kind::Nullable => f.write_str("Nullable"),
            Kind::Row => f.write_str("Row"),
            Kind::Negatable => f.write_str("Negatable"),
        }
    }
}

// Kinds are ordered by name so that polytypes are displayed deterministically
impl cmp::Ord for Kind {
    fn cmp(&self, other: &Self) -> cmp::Ordering {
        self.to_string().cmp(&other.to_string())
    }
}

// Kinds are ordered by name so that polytypes are displayed deterministically
impl cmp::PartialOrd for Kind {
    fn partial_cmp(&self, other: &Self) -> Option<cmp::Ordering> {
        Some(self.cmp(other))
    }
}

// MonoType represents a specific named type
#[derive(Debug, Clone, PartialEq)]
pub enum MonoType {
    Bool { loc: ast::SourceLocation },
    Int { loc: ast::SourceLocation },
    Uint { loc: ast::SourceLocation },
    Float { loc: ast::SourceLocation },
    String { loc: ast::SourceLocation },
    Duration { loc: ast::SourceLocation },
    Time { loc: ast::SourceLocation },
    Regexp { loc: ast::SourceLocation },
    Bytes { loc: ast::SourceLocation },
    Var(Tvar),
    Arr(Box<Array>),
    Obj(Box<Record>),
    Par(Box<Parameter>),
    Fnc(Box<Fun>),
}

pub type MonoTypeMap = SemanticMap<String, MonoType>;
pub type MonoTypeVecMap = SemanticMap<String, Vec<MonoType>>;

impl fmt::Display for MonoType {
    fn fmt(&self, f: &mut fmt::Formatter) -> fmt::Result {
        match self {
            MonoType::Bool { .. } => f.write_str("bool"),
            MonoType::Int { .. } => f.write_str("int"),
            MonoType::Uint { .. } => f.write_str("uint"),
            MonoType::Float { .. } => f.write_str("float"),
            MonoType::String { .. } => f.write_str("string"),
            MonoType::Duration { .. } => f.write_str("duration"),
            MonoType::Time { .. } => f.write_str("time"),
            MonoType::Regexp { .. } => f.write_str("regexp"),
            MonoType::Bytes { .. } => f.write_str("bytes"),
            MonoType::Var(var) => var.fmt(f),
            MonoType::Arr(arr) => arr.fmt(f),
            MonoType::Obj(obj) => write!(f, "{}", obj),
            MonoType::Par(par) => write!(f, "{}", par),
            MonoType::Fnc(fun) => write!(f, "{}", fun),
        }
    }
}

impl Substitutable for MonoType {
    fn apply(self, sub: &Substitution) -> Self {
        match self {
            MonoType::Bool { .. }
            | MonoType::Int { .. }
            | MonoType::Uint { .. }
            | MonoType::Float { .. }
            | MonoType::String { .. }
            | MonoType::Duration { .. }
            | MonoType::Time { .. }
            | MonoType::Regexp { .. }
            | MonoType::Bytes { .. } => self,
            MonoType::Var(tvr) => sub.apply(tvr),
            MonoType::Arr(arr) => MonoType::Arr(Box::new(arr.apply(sub))),
            MonoType::Obj(obj) => MonoType::Obj(Box::new(obj.apply(sub))),
            MonoType::Par(par) => MonoType::Par(Box::new(par.apply(sub))),
            MonoType::Fnc(fun) => MonoType::Fnc(Box::new(fun.apply(sub))),
        }
    }
    fn free_vars(&self) -> Vec<Tvar> {
        match self {
            MonoType::Bool { .. }
            | MonoType::Int { .. }
            | MonoType::Uint { .. }
            | MonoType::Float { .. }
            | MonoType::String { .. }
            | MonoType::Duration { .. }
            | MonoType::Time { .. }
            | MonoType::Regexp { .. }
            | MonoType::Bytes { .. } => Vec::new(),
            MonoType::Var(tvr) => vec![*tvr],
            MonoType::Arr(arr) => arr.free_vars(),
            MonoType::Obj(obj) => obj.free_vars(),
            MonoType::Par(par) => par.free_vars(),
            MonoType::Fnc(fun) => fun.free_vars(),
        }
    }
}

impl MaxTvar for MonoType {
    fn max_tvar(&self) -> Tvar {
        match self {
            MonoType::Bool { .. }
            | MonoType::Int { .. }
            | MonoType::Uint { .. }
            | MonoType::Float { .. }
            | MonoType::String { .. }
            | MonoType::Duration { .. }
            | MonoType::Time { .. }
            | MonoType::Regexp { .. }
            | MonoType::Bytes { .. } => Tvar(0),
            MonoType::Var(tvr) => tvr.max_tvar(),
            MonoType::Arr(arr) => arr.max_tvar(),
            MonoType::Obj(obj) => obj.max_tvar(),
            MonoType::Par(par) => par.max_tvar(),
            MonoType::Fnc(fun) => fun.max_tvar(),
        }
    }
}

pub fn unify_types(
    exp: MonoType,
    act: MonoType,
    cons: &mut TvarKinds,
    f: &mut Fresher,
) -> Result<Substitution, Error> {
    match (exp, act) {
        (MonoType::Bool { .. }, MonoType::Bool { .. })
        | (MonoType::Int { .. }, MonoType::Int { .. })
        | (MonoType::Uint { .. }, MonoType::Uint { .. })
        | (MonoType::Float { .. }, MonoType::Float { .. })
        | (MonoType::String { .. }, MonoType::String { .. })
        | (MonoType::Duration { .. }, MonoType::Duration { .. })
        | (MonoType::Time { .. }, MonoType::Time { .. })
        | (MonoType::Regexp { .. }, MonoType::Regexp { .. })
        | (MonoType::Bytes { .. }, MonoType::Bytes { .. }) => Ok(Substitution::empty()),
        (MonoType::Var(tv), t) => tv.unify(t, cons),
        (t, MonoType::Var(tv)) => tv.unify(t, cons),
        (MonoType::Arr(t), MonoType::Arr(s)) => unify_arrays(*t, *s, cons, f),
        (MonoType::Obj(t), MonoType::Obj(s)) => unify_record(*t, *s, cons, f),
        (MonoType::Par(t), MonoType::Par(s)) => unify_params(*t, *s, cons, f),
        (MonoType::Fnc(t), MonoType::Fnc(s)) => unify_function(*t, *s, cons, f),
        (t, with) => Err(Error::NotEqual { exp: exp, act: act }),
    }
}

#[derive(Debug, PartialEq)]
pub struct ConstraintError {
    exp: Kind,
    act: MonoType,
    loc: ast::SourceLocation,
}

impl fmt::Display for ConstraintError {
    fn fmt(&self, f: &mut fmt::Formatter) -> fmt::Result {
        match self.exp {
            Kind::Addable => write!(
                f,
                "error {}: {} is not {}\nExpected: {}",
                self.loc, self.act, self.exp, "int | uint | float | string"
            ),
            Kind::Subtractable => write!(
                f,
                "error {}: {} is not {}\nExpected: {}",
                self.loc, self.act, self.exp, "int | uint | float"
            ),
            Kind::Divisible => write!(
                f,
                "error {}: {} is not {}\nExpected: {}",
                self.loc, self.act, self.exp, "int | uint | float"
            ),
            Kind::Numeric => write!(
                f,
                "error {}: {} is not {}\nExpected: {}",
                self.loc, self.act, self.exp, "int | uint | float"
            ),
            Kind::Comparable => write!(
                f,
                "error {}: {} is not {}\nExpected: {}",
                self.loc, self.act, self.exp, "int | uint | float | string | duration | time"
            ),
            Kind::Equatable => write!(
                f,
                "error {}: {} is not {}\nExpected: {}",
                self.loc,
                self.act,
                self.exp,
                "bool | int | uint | float | string | duration | time | bytes | array | record"
            ),
            Kind::Nullable => write!(
                f,
                "error {}: {} is not {}\nExpected: {}",
                self.loc,
                self.act,
                self.exp,
                "bool | int | uint | float | string | duration | time"
            ),
            Kind::Negatable => write!(
                f,
                "error {}: {} is not {}\nExpected: {}",
                self.loc, self.act, self.exp, "int | uint | float | duration"
            ),
            Kind::Row => write!(f, "error {}: {} is not a record type", self.loc, self.act),
        }
    }
}

impl From<ConstraintError> for Error {
    fn from(err: ConstraintError) -> Self {
        Error::Constraint { err }
    }
}

fn constrain_type(ty: MonoType, with: Kind, cons: &mut TvarKinds) -> Result<Substitution, Error> {
    match with {
        Kind::Addable => match ty {
            MonoType::Int { .. }
            | MonoType::Uint { .. }
            | MonoType::Float { .. }
            | MonoType::String { .. } => Ok(Substitution::empty()),
            MonoType::Bool { loc }
            | MonoType::Duration { loc }
            | MonoType::Time { loc }
            | MonoType::Regexp { loc }
            | MonoType::Bytes { loc } => Err(ConstraintError {
                exp: with,
                act: ty,
                loc: loc,
            }
            .into()),
        },
        Kind::Subtractable => match ty {
            MonoType::Int { .. } | MonoType::Uint { .. } | MonoType::Float { .. } => {
                Ok(Substitution::empty())
            }
            MonoType::Bool { loc }
            | MonoType::Duration { loc }
            | MonoType::Time { loc }
            | MonoType::Regexp { loc }
            | MonoType::Bytes { loc } => Err(ConstraintError {
                exp: with,
                act: ty,
                loc: loc,
            }
            .into()),
        },
        Kind::Divisible => match ty {
            MonoType::Int { .. } | MonoType::Uint { .. } | MonoType::Float { .. } => {
                Ok(Substitution::empty())
            }
            MonoType::Bool { loc }
            | MonoType::Duration { loc }
            | MonoType::Time { loc }
            | MonoType::Regexp { loc }
            | MonoType::Bytes { loc } => Err(ConstraintError {
                exp: with,
                act: ty,
                loc: loc,
            }
            .into()),
        },
        Kind::Numeric => match ty {
            MonoType::Int { .. } | MonoType::Uint { .. } | MonoType::Float { .. } => {
                Ok(Substitution::empty())
            }
            MonoType::Bool { loc }
            | MonoType::Duration { loc }
            | MonoType::Time { loc }
            | MonoType::Regexp { loc }
            | MonoType::Bytes { loc } => Err(ConstraintError {
                exp: with,
                act: ty,
                loc: loc,
            }
            .into()),
        },
        Kind::Comparable => match ty {
            MonoType::Int { .. }
            | MonoType::Uint { .. }
            | MonoType::Float { .. }
            | MonoType::String { .. }
            | MonoType::Duration { .. }
            | MonoType::Time { .. } => Ok(Substitution::empty()),
            MonoType::Bool { loc }
            | MonoType::Duration { loc }
            | MonoType::Time { loc }
            | MonoType::Regexp { loc }
            | MonoType::Bytes { loc } => Err(ConstraintError {
                exp: with,
                act: ty,
                loc: loc,
            }
            .into()),
        },
        Kind::Equatable => match ty {
            MonoType::Bool { .. }
            | MonoType::Int { .. }
            | MonoType::Uint { .. }
            | MonoType::Float { .. }
            | MonoType::String { .. }
            | MonoType::Duration { .. }
            | MonoType::Time { .. }
            | MonoType::Bytes { .. }
            | MonoType::Arr { .. }
            | MonoType::Obj { .. } => Ok(Substitution::empty()),
            MonoType::Bool { loc }
            | MonoType::Duration { loc }
            | MonoType::Time { loc }
            | MonoType::Regexp { loc }
            | MonoType::Bytes { loc } => Err(ConstraintError {
                exp: with,
                act: ty,
                loc: loc,
            }
            .into()),
        },
        Kind::Nullable => match ty {
            MonoType::Bool { .. }
            | MonoType::Int { .. }
            | MonoType::Uint { .. }
            | MonoType::Float { .. }
            | MonoType::String { .. }
            | MonoType::Duration { .. }
            | MonoType::Time { .. } => Ok(Substitution::empty()),
            MonoType::Bool { loc }
            | MonoType::Duration { loc }
            | MonoType::Time { loc }
            | MonoType::Regexp { loc }
            | MonoType::Bytes { loc } => Err(ConstraintError {
                exp: with,
                act: ty,
                loc: loc,
            }
            .into()),
        },
        Kind::Negatable => match ty {
            MonoType::Int { .. }
            | MonoType::Uint { .. }
            | MonoType::Float { .. }
            | MonoType::Duration { .. } => Ok(Substitution::empty()),
            MonoType::Bool { loc }
            | MonoType::Duration { loc }
            | MonoType::Time { loc }
            | MonoType::Regexp { loc }
            | MonoType::Bytes { loc } => Err(ConstraintError {
                exp: with,
                act: ty,
                loc: loc,
            }
            .into()),
        },
        Kind::Row => match ty {
            MonoType::Obj { .. } => Ok(Substitution::empty()),
            MonoType::Bool { loc }
            | MonoType::Duration { loc }
            | MonoType::Time { loc }
            | MonoType::Regexp { loc }
            | MonoType::Bytes { loc } => Err(ConstraintError {
                exp: with,
                act: ty,
                loc: loc,
            }
            .into()),
        },
    }
}

impl MonoType {
    fn contains(&self, tv: Tvar) -> bool {
        match self {
            MonoType::Bool { .. }
            | MonoType::Int { .. }
            | MonoType::Uint { .. }
            | MonoType::Float { .. }
            | MonoType::String { .. }
            | MonoType::Duration { .. }
            | MonoType::Time { .. }
            | MonoType::Regexp { .. }
            | MonoType::Bytes { .. } => false,
            MonoType::Var(tvr) => tv == *tvr,
            MonoType::Arr(arr) => arr.contains(tv),
            MonoType::Obj(obj) => obj.contains(tv),
            MonoType::Par(par) => par.contains(tv),
            MonoType::Fnc(fun) => fun.contains(tv),
        }
    }
}

// Tvar stands for type variable.
// A type variable holds an unknown type.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, PartialOrd, Ord)]
pub struct Tvar(pub u64);

// TvarKinds is a map from type variables to their constraining kinds.
pub type TvarKinds = SemanticMap<Tvar, Vec<Kind>>;
pub type TvarMap = SemanticMap<Tvar, Tvar>;
pub type SubstitutionMap = SemanticMap<Tvar, MonoType>;

impl fmt::Display for Tvar {
    fn fmt(&self, f: &mut fmt::Formatter) -> fmt::Result {
        write!(f, "t{}", self.0)
    }
}

impl MaxTvar for Tvar {
    fn max_tvar(&self) -> Tvar {
        *self
    }
}

impl Tvar {
    fn unify(self, with: MonoType, cons: &mut TvarKinds) -> Result<Substitution, Error> {
        match with {
            MonoType::Var(tv) => {
                if self == tv {
                    // The empty substitution will always
                    // unify a type variable with itself.
                    Ok(Substitution::empty())
                } else {
                    // Unify two distinct type variables.
                    // This will update the kind constraints
                    // associated with these type variables.
                    self.unify_with_tvar(tv, cons)
                }
            }
            _ => {
                if with.contains(self) {
                    // Invalid recursive type
                    Err(Error::OccursCheck { tv: self, ty: with })
                } else {
                    // Unify a type variable with a monotype.
                    // The monotype must satisify any
                    // constraints placed on the type variable.
                    self.unify_with_type(with, cons)
                }
            }
        }
    }

    fn unify_with_tvar(self, tv: Tvar, cons: &mut TvarKinds) -> Result<Substitution, Error> {
        // Kind constraints for both type variables
        let kinds = union(
            cons.remove(&self).unwrap_or_default(),
            cons.remove(&tv).unwrap_or_default(),
        );
        if !kinds.is_empty() {
            cons.insert(tv, kinds);
        }
        Ok(Substitution::from(
            semantic_map! {self => MonoType::Var(tv)},
        ))
    }

    fn unify_with_type(self, t: MonoType, cons: &mut TvarKinds) -> Result<Substitution, Error> {
        let sub = Substitution::from(semantic_map! {self => t.clone()});
        match cons.remove(&self) {
            None => Ok(sub),
            Some(kinds) => Ok(sub.merge(kinds.into_iter().try_fold(
                Substitution::empty(),
                |sub, kind| {
                    // The monotype that is being unified with the
                    // tvar must be constrained with the same kinds
                    // as that of the tvar.
                    Ok(sub.merge(constrain_type(t.clone(), kind, cons)?))
                },
            )?)),
        }
    }

    fn constrain(self, with: Kind, cons: &mut TvarKinds) {
        match cons.get_mut(&self) {
            Some(kinds) => {
                if !kinds.contains(&with) {
                    kinds.push(with);
                }
            }
            None => {
                cons.insert(self, vec![with]);
            }
        }
    }
}

// Array is a homogeneous list type
#[derive(Debug, Clone, PartialEq)]
pub struct Array {
    typ: MonoType,
    loc: ast::SourceLocation,
}

impl fmt::Display for Array {
    fn fmt(&self, f: &mut fmt::Formatter) -> fmt::Result {
        write!(f, "[{}]", self.typ)
    }
}

impl Substitutable for Array {
    fn apply(self, sub: &Substitution) -> Self {
        Array {
            typ: self.typ.apply(&sub),
            loc: self.loc,
        }
    }
    fn free_vars(&self) -> Vec<Tvar> {
        self.typ.free_vars()
    }
}

impl MaxTvar for Array {
    fn max_tvar(&self) -> Tvar {
        self.typ.max_tvar()
    }
}

impl Array {
    fn loc(&self) -> &ast::SourceLocation {
        &self.loc
    }
    fn contains(&self, tv: Tvar) -> bool {
        self.typ.contains(tv)
    }
}

fn unify_arrays(
    exp: Array,
    act: Array,
    cons: &mut TvarKinds,
    f: &mut Fresher,
) -> Result<Substitution, Error> {
    unify_types(exp.typ, act.typ, cons, f)
}

// A key value pair representing a property type in a record
#[derive(Debug, Clone, PartialEq)]
pub struct Property {
    pub k: String,
    pub v: MonoType,
}

impl fmt::Display for Property {
    fn fmt(&self, f: &mut fmt::Formatter) -> fmt::Result {
        write!(f, "{}:{}", self.k, self.v)
    }
}

impl Substitutable for Property {
    fn apply(self, sub: &Substitution) -> Self {
        Property {
            k: self.k,
            v: self.v.apply(sub),
        }
    }
    fn free_vars(&self) -> Vec<Tvar> {
        self.v.free_vars()
    }
}

impl MaxTvar for Property {
    fn max_tvar(&self) -> Tvar {
        self.v.max_tvar()
    }
}

#[allow(clippy::implicit_hasher)]
impl<T: Substitutable> Substitutable for SemanticMap<String, T> {
    fn apply(self, sub: &Substitution) -> Self {
        self.into_iter().map(|(k, v)| (k, v.apply(sub))).collect()
    }
    fn free_vars(&self) -> Vec<Tvar> {
        self.values()
            .fold(Vec::new(), |vars, t| union(vars, t.free_vars()))
    }
}

impl<T: Substitutable> Substitutable for Option<T> {
    fn apply(self, sub: &Substitution) -> Self {
        match self {
            Some(t) => Some(t.apply(sub)),
            None => None,
        }
    }
    fn free_vars(&self) -> Vec<Tvar> {
        match self {
            Some(t) => t.free_vars(),
            None => Vec::new(),
        }
    }
}

impl<U, T: MaxTvar> MaxTvar for SemanticMap<U, T> {
    fn max_tvar(&self) -> Tvar {
        self.iter()
            .map(|(_, t)| t.max_tvar())
            .fold(Tvar(0), |max, tv| if tv > max { tv } else { max })
    }
}

impl<T: MaxTvar> MaxTvar for Option<T> {
    fn max_tvar(&self) -> Tvar {
        match self {
            None => Tvar(0),
            Some(t) => t.max_tvar(),
        }
    }
}

pub trait MaxTvar {
    fn max_tvar(&self) -> Tvar;
}

#[derive(Debug, Clone, PartialEq)]
pub enum Record {
    Empty {
        loc: ast::SourceLocation,
    },
    Extension {
        loc: ast::SourceLocation,
        lab: String,
        typ: MonoType,
        ext: MonoType,
    },
}

impl fmt::Display for Record {
    fn fmt(&self, f: &mut fmt::Formatter) -> fmt::Result {
        match self {
            Record::Empty { .. } => write!(f, "{}", "{}"),
            Record::Extension {
                loc: _,
                lab: a,
                typ: t,
                ext: r,
            } => write!(f, "{}:{} | {}", a, t, r),
        }
    }
}

impl Substitutable for Record {
    fn apply(self, sub: &Substitution) -> Self {
        match self {
            Record::Empty { .. } => self,
            Record::Extension {
                loc: l,
                lab: a,
                typ: t,
                ext: r,
            } => Record::Extension {
                loc: l,
                lab: a,
                typ: t.apply(sub),
                ext: r.apply(sub),
            },
        }
    }
    fn free_vars(&self) -> Vec<Tvar> {
        match self {
            Record::Empty { .. } => Vec::new(),
            Record::Extension {
                loc: _,
                lab: _,
                typ: t,
                ext: r,
            } => union(t.free_vars(), r.free_vars()),
        }
    }
}

impl MaxTvar for Record {
    fn max_tvar(&self) -> Tvar {
        match self {
            Record::Empty { .. } => Tvar(0),
            Record::Extension {
                loc: _,
                lab: _,
                typ: t,
                ext: r,
            } => vec![t.max_tvar(), r.max_tvar()].max_tvar(),
        }
    }
}

impl Record {
    fn loc(&self) -> &ast::SourceLocation {
        match self {
            Record::Empty { loc } => loc,
            Record::Extension { loc, .. } => loc,
        }
    }
    fn contains(&self, tv: Tvar) -> bool {
        match self {
            Record::Empty { .. } => false,
            Record::Extension {
                loc: _,
                lab: _,
                typ: t,
                ext: r,
            } => t.contains(tv) || r.contains(tv),
        }
    }
}

#[derive(Debug, PartialEq)]
pub enum RecordError {
    NotFound {
        lab: String,
        loc: ast::SourceLocation,
    },
    Unexpected {
        lab: String,
        loc: ast::SourceLocation,
    },
    LabelMismatch {
        exp: String,
        act: String,
        loc: ast::SourceLocation,
    },
}

impl fmt::Display for RecordError {
    fn fmt(&self, f: &mut fmt::Formatter) -> fmt::Result {
        match self {
            RecordError::NotFound { lab, loc } => {
                write!(f, "error {}: label {} not found", loc, lab)
            }
            RecordError::Unexpected { lab, loc } => {
                write!(f, "error {}: unexpected label {}", loc, lab)
            }
            RecordError::LabelMismatch { exp, act, loc } => write!(
                f,
                "error {}: unexpected record labels\nExpected: {}\nFound: {}",
                loc, exp, act
            ),
        }
    }
}

impl From<RecordError> for Error {
    fn from(err: RecordError) -> Self {
        Error::Record { err }
    }
}

// Rules for record unification. In what follows a != b and t is a type variable.
//
// 1. {} == {}
// 2. {a: _ | _} != {}
// 3. {a: _ | t} != {b: _ | t}
// 4. {a: u | t} == {a: v | t} => u == v
// 5. {a: u | r} == {a: v | s} => t = u, r = s
// 6. {a: u | r} == {b: v | s} => r = {b: v | t}, s = {a: u | t}
//
fn unify_record(
    exp: Record,
    act: Record,
    cons: &mut TvarKinds,
    f: &mut Fresher,
) -> Result<Substitution, Error> {
    match (exp.clone(), act.clone()) {
        // 1. {} == {}
        (Record::Empty { .. }, Record::Empty { .. }) => Ok(Substitution::empty()),
        // 2. {a: _ | _} != {}
        (Record::Extension { lab: a, .. }, Record::Empty { loc }) => {
            Err(RecordError::NotFound { lab: a, loc }.into())
        }
        (Record::Empty { .. }, Record::Extension { lab: a, loc, .. }) => {
            Err(RecordError::Unexpected { lab: a, loc }.into())
        }
        // 3. {a: _ | t} != {b: _ | t}
        // 4. {a: u | t} == {a: v | t} => u == v
        (
            Record::Extension {
                lab: a,
                typ: u,
                ext: MonoType::Var(r),
                ..
            },
            Record::Extension {
                loc: l,
                lab: b,
                typ: v,
                ext: MonoType::Var(s),
            },
        ) if r == s => match a == b {
            true => unify_types(u, v, cons, f),
            false => Err(RecordError::LabelMismatch {
                exp: a,
                act: b,
                loc: l,
            }
            .into()),
        },
        // 5. {a: u | r} == {a: v | s} => t = u, r = s
        // 6. {a: u | r} == {b: v | s} => r = {b: v | t}, s = {a: u | t}
        (
            Record::Extension {
                loc: aloc,
                lab: a,
                typ: u,
                ext: r,
            },
            Record::Extension {
                loc: bloc,
                lab: b,
                typ: v,
                ext: s,
            },
        ) => match a == b {
            true => {
                let sub = unify_types(u, v, cons, f)?;
                apply_then_unify(r, s, sub, cons, f)
            }
            false => {
                let i = f.fresh();
                let x = MonoType::Obj(Box::new(Record::Extension {
                    loc: bloc,
                    lab: b,
                    typ: v,
                    ext: MonoType::Var(i),
                }));
                let y = MonoType::Obj(Box::new(Record::Extension {
                    loc: aloc,
                    lab: a,
                    typ: u,
                    ext: MonoType::Var(i),
                }));
                let sub = unify_types(r, x, cons, f)?;
                apply_then_unify(s, y, sub, cons, f)
            }
        },
    }
}

#[derive(Debug, Clone, PartialEq)]
pub enum Parameter {
    None {
        loc: ast::SourceLocation,
    },
    Req {
        loc: ast::SourceLocation,
        lab: String,
        typ: MonoType,
        ext: MonoType,
    },
    Opt {
        loc: ast::SourceLocation,
        lab: String,
        typ: MonoType,
        ext: MonoType,
    },
    Pipe {
        loc: ast::SourceLocation,
        lab: String,
        typ: MonoType,
        ext: MonoType,
    },
}

impl fmt::Display for Parameter {
    fn fmt(&self, f: &mut fmt::Formatter) -> fmt::Result {
        match self.clone() {
            Parameter::None { .. } => Ok(()),
            Parameter::Req {
                lab: a,
                typ: t,
                ext: MonoType::Par(r),
                ..
            } => match *r {
                Parameter::None { .. } => write!(f, "{}:{}", a, t),
                _ => write!(f, "{}:{}, {}", a, t, r),
            },
            Parameter::Opt {
                lab: a,
                typ: t,
                ext: MonoType::Par(r),
                ..
            } => match *r {
                Parameter::None { .. } => write!(f, "?{}:{}", a, t),
                _ => write!(f, "?{}:{}, {}", a, t, r),
            },
            Parameter::Pipe {
                lab: a,
                typ: t,
                ext: MonoType::Par(r),
                ..
            } => match *r {
                Parameter::None { .. } => {
                    if a == "<-" {
                        write!(f, "{}:{}", a, t)
                    } else {
                        write!(f, "<-{}:{}", a, t)
                    }
                }
                _ => {
                    if a == "<-" {
                        write!(f, "{}:{}, {}", a, t, r)
                    } else {
                        write!(f, "<-{}:{}, {}", a, t, r)
                    }
                }
            },
        }
    }
}

impl Substitutable for Parameter {
    fn apply(self, sub: &Substitution) -> Self {
        match self {
            Parameter::None { loc } => self,
            Parameter::Req {
                loc: l,
                lab: a,
                typ: t,
                ext: r,
            } => Parameter::Req {
                loc: l,
                lab: a,
                typ: t.apply(sub),
                ext: r.apply(sub),
            },
            Parameter::Opt {
                loc: l,
                lab: a,
                typ: t,
                ext: r,
            } => Parameter::Opt {
                loc: l,
                lab: a,
                typ: t.apply(sub),
                ext: r.apply(sub),
            },
            Parameter::Pipe {
                loc: l,
                lab: a,
                typ: t,
                ext: r,
            } => Parameter::Pipe {
                loc: l,
                lab: a,
                typ: t.apply(sub),
                ext: r.apply(sub),
            },
        }
    }
    fn free_vars(&self) -> Vec<Tvar> {
        match self {
            Parameter::None { .. } => Vec::new(),
            Parameter::Req { typ: t, ext: r, .. } => union(t.free_vars(), r.free_vars()),
            Parameter::Opt { typ: t, ext: r, .. } => union(t.free_vars(), r.free_vars()),
            Parameter::Pipe { typ: t, ext: r, .. } => union(t.free_vars(), r.free_vars()),
        }
    }
}

impl MaxTvar for Parameter {
    fn max_tvar(&self) -> Tvar {
        match self {
            Parameter::None { .. } => Tvar(0),
            Parameter::Req { typ: t, ext: r, .. } => vec![t.max_tvar(), r.max_tvar()].max_tvar(),
            Parameter::Opt { typ: t, ext: r, .. } => vec![t.max_tvar(), r.max_tvar()].max_tvar(),
            Parameter::Pipe { typ: t, ext: r, .. } => vec![t.max_tvar(), r.max_tvar()].max_tvar(),
        }
    }
}

impl Parameter {
    fn loc(&self) -> &ast::SourceLocation {
        match self {
            Parameter::None { loc } => loc,
            Parameter::Req { loc, .. } => loc,
            Parameter::Opt { loc, .. } => loc,
            Parameter::Pipe { loc, .. } => loc,
        }
    }
    fn contains(&self, tv: Tvar) -> bool {
        match self {
            Parameter::None { .. } => false,
            Parameter::Req { typ: t, ext: r, .. } => t.contains(tv) || r.contains(tv),
            Parameter::Opt { typ: t, ext: r, .. } => t.contains(tv) || r.contains(tv),
            Parameter::Pipe { typ: t, ext: r, .. } => t.contains(tv) || r.contains(tv),
        }
    }
}

#[derive(Debug, PartialEq)]
pub enum ArgError {
    NotFound {
        arg: String,
        loc: ast::SourceLocation,
    },
    Unexpected {
        arg: String,
        loc: ast::SourceLocation,
    },
    LabelMismatch {
        exp: String,
        act: String,
        loc: ast::SourceLocation,
    },
}

impl fmt::Display for ArgError {
    fn fmt(&self, f: &mut fmt::Formatter) -> fmt::Result {
        match self {
            ArgError::NotFound { arg, loc } => {
                write!(f, "error {}: required argument {} not found", loc, arg)
            }
            ArgError::Unexpected { arg, loc } => write!(
                f,
                "error {}: function does not take an argument named {}",
                loc, arg
            ),
            ArgError::LabelMismatch { exp, act, loc } => write!(
                f,
                "error {}: unexpected function argument\nExpected: {}\nFound: {}",
                loc, exp, act
            ),
        }
    }
}

impl From<ArgError> for Error {
    fn from(err: ArgError) -> Self {
        Error::Arg { err }
    }
}

#[derive(Debug, PartialEq)]
pub enum PipeError {
    NotFound {
        loc: ast::SourceLocation,
    },
    Unexpected {
        loc: ast::SourceLocation,
    },
    Optional {
        arg: String,
        loc: ast::SourceLocation,
    },
    LabelMismatch {
        exp: String,
        act: String,
        loc: ast::SourceLocation,
    },
}

impl fmt::Display for PipeError {
    fn fmt(&self, f: &mut fmt::Formatter) -> fmt::Result {
        match self {
            PipeError::NotFound { loc } => write!(f, "error {}: pipe argument not found", loc),
            PipeError::Unexpected { loc } => {
                write!(f, "error {}: function does not take a pipe argument", loc)
            }
            PipeError::Optional { arg, loc } => write!(
                f,
                "error {}: pipe argument {} is not allowed to be optional",
                loc, arg
            ),
            PipeError::LabelMismatch { exp, act, loc } => write!(
                f,
                "error {}: pipe arguments do not match\nExpected: {}\nFound: {}",
                loc, exp, act
            ),
        }
    }
}

impl From<PipeError> for Error {
    fn from(err: PipeError) -> Self {
        Error::Pipe { err }
    }
}

// Rules for parameter unification. In what follows a != b, t is a
// type variable, > represents a pipe parameter, and ? represents
// an optional parameter.
//
// 0. {} == {}
// 1. {a: _ | _} != {}
// 2. {>: _ | _} != {}
// 3. {?: _ | r} == {} => r == {}
// 4. {a: _ | t} != {b: _ | t}
// 5. {a: u | t} == {a: v | t} => u == v
// 6. {>: u | r} == {>: v | s} => t = u, r = s
// 7. {>: _ | _} != {?: _ | _}
// 8. {a: u | r} == {a: v | s} => t = u, r = s
// 9. {a: u | r} == {b: v | s} => r = {b: v | t}, s = {a: u | t}
//
fn unify_params(
    exp: Parameter,
    act: Parameter,
    cons: &mut TvarKinds,
    f: &mut Fresher,
) -> Result<Substitution, Error> {
    match (exp.clone(), act.clone()) {
        // 0. {} == {}
        (Parameter::None { .. }, Parameter::None { .. }) => Ok(Substitution::empty()),
        // 1. {a: _ | _} != {}
        // 2. {>: _ | _} != {}
        (Parameter::Req { lab: a, .. }, Parameter::None { loc }) => {
            Err(ArgError::NotFound { arg: a, loc }.into())
        }
        (Parameter::Pipe { .. }, Parameter::None { loc }) => {
            Err(PipeError::NotFound { loc }.into())
        }
        (Parameter::None { .. }, Parameter::Req { lab: a, loc, .. }) => {
            Err(ArgError::Unexpected { arg: a, loc }.into())
        }
        (Parameter::None { .. }, Parameter::Pipe { loc, .. }) => {
            Err(PipeError::Unexpected { loc }.into())
        }
        // 3. {?: _ | r} == {} => r == {}
        (Parameter::Opt { ext: r, .. }, Parameter::None { loc }) => {
            unify_types(r, MonoType::Par(Box::new(act)), cons, f)
        }
        (Parameter::None { loc }, Parameter::Opt { ext: r, .. }) => {
            unify_types(MonoType::Par(Box::new(exp)), r, cons, f)
        }
        // 4. {a: _ | t} != {b: _ | t}
        // 5. {a: u | t} == {a: v | t} => u == v
        (
            Parameter::Req {
                lab: a,
                typ: u,
                ext: MonoType::Var(r),
                ..
            },
            Parameter::Req {
                loc: l,
                lab: b,
                typ: v,
                ext: MonoType::Var(s),
            },
        )
        | (
            Parameter::Opt {
                lab: a,
                typ: u,
                ext: MonoType::Var(r),
                ..
            },
            Parameter::Opt {
                loc: l,
                lab: b,
                typ: v,
                ext: MonoType::Var(s),
            },
        ) if r == s => match a == b {
            true => unify_types(u, v, cons, f),
            false => Err(ArgError::LabelMismatch {
                exp: a,
                act: b,
                loc: l,
            }
            .into()),
        },
        // 4. {a: _ | t} != {b: _ | t}
        // 5. {a: u | t} == {a: v | t} => u == v
        (
            Parameter::Pipe {
                lab: a,
                typ: u,
                ext: MonoType::Var(r),
                ..
            },
            Parameter::Pipe {
                loc: l,
                lab: b,
                typ: v,
                ext: MonoType::Var(s),
            },
        ) if r == s => {
            if a == "<-" || b == "<-" || a == b {
                unify_types(u, v, cons, f)
            } else {
                Err(PipeError::LabelMismatch {
                    exp: a,
                    act: b,
                    loc: l,
                }
                .into())
            }
        }
        // 6. {>: u | r} == {>: v | s} => t = u, r = s
        (
            Parameter::Pipe {
                loc: _,
                lab: a,
                typ: u,
                ext: r,
            },
            Parameter::Pipe {
                loc: l,
                lab: b,
                typ: v,
                ext: s,
            },
        ) => {
            if a == "<-" || b == "<-" || a == b {
                let sub = unify_types(u, v, cons, f)?;
                apply_then_unify(r, s, sub, cons, f)
            } else {
                Err(PipeError::LabelMismatch {
                    exp: a,
                    act: b,
                    loc: l,
                }
                .into())
            }
        }
        // 8. {a: u | r} == {a: v | s} => u == v, r == s
        // 9. {a: u | r} == {b: v | s} => r == {b: v | t}, s == {a: u | t}
        (
            Parameter::Req {
                loc: aloc,
                lab: a,
                typ: u,
                ext: r,
            },
            Parameter::Req {
                loc: bloc,
                lab: b,
                typ: v,
                ext: s,
            },
        ) => match a == b {
            true => {
                let sub = unify_types(u, v, cons, f)?;
                apply_then_unify(r, s, sub, cons, f)
            }
            false => {
                let i = f.fresh();
                let x = MonoType::Par(Box::new(Parameter::Req {
                    loc: bloc,
                    lab: b,
                    typ: v,
                    ext: MonoType::Var(i),
                }));
                let y = MonoType::Par(Box::new(Parameter::Req {
                    loc: aloc,
                    lab: a,
                    typ: u,
                    ext: MonoType::Var(i),
                }));
                let sub = unify_types(r, x, cons, f)?;
                apply_then_unify(y, s, sub, cons, f)
            }
        },
        // 8. {a: u | r} == {a: v | s} => u == v, r == s
        // 9. {a: u | r} == {b: v | s} => r == {b: v | t}, s == {a: u | t}
        (
            Parameter::Opt {
                loc: aloc,
                lab: a,
                typ: u,
                ext: r,
            },
            Parameter::Opt {
                loc: bloc,
                lab: b,
                typ: v,
                ext: s,
            },
        ) => match a == b {
            true => {
                let sub = unify_types(u, v, cons, f)?;
                apply_then_unify(r, s, sub, cons, f)
            }
            false => {
                let i = f.fresh();
                let x = MonoType::Par(Box::new(Parameter::Opt {
                    loc: bloc,
                    lab: b,
                    typ: v,
                    ext: MonoType::Var(i),
                }));
                let y = MonoType::Par(Box::new(Parameter::Opt {
                    loc: aloc,
                    lab: a,
                    typ: u,
                    ext: MonoType::Var(i),
                }));
                let sub = unify_types(r, x, cons, f)?;
                apply_then_unify(y, s, sub, cons, f)
            }
        },
        // 8. {a: u | r} == {a: v | s} => u == v, r == s
        // 9. {a: u | r} == {b: v | s} => r == {b: v | t}, s == {a: u | t}
        (
            Parameter::Req {
                loc: aloc,
                lab: a,
                typ: u,
                ext: r,
            },
            Parameter::Opt {
                loc: bloc,
                lab: b,
                typ: v,
                ext: s,
            },
        )
        | (
            Parameter::Opt {
                loc: bloc,
                lab: b,
                typ: v,
                ext: s,
            },
            Parameter::Req {
                loc: aloc,
                lab: a,
                typ: u,
                ext: r,
            },
        ) => match a == b {
            true => {
                let sub = unify_types(u, v, cons, f)?;
                apply_then_unify(r, s, sub, cons, f)
            }
            false => {
                let i = f.fresh();
                let x = MonoType::Par(Box::new(Parameter::Opt {
                    loc: bloc,
                    lab: b,
                    typ: v,
                    ext: MonoType::Var(i),
                }));
                let y = MonoType::Par(Box::new(Parameter::Req {
                    loc: aloc,
                    lab: a,
                    typ: u,
                    ext: MonoType::Var(i),
                }));
                let sub = unify_types(r, x, cons, f)?;
                apply_then_unify(s, y, sub, cons, f)
            }
        },
        // 8. {a: u | r} == {a: v | s} => u == v, r == s
        // 9. {a: u | r} == {b: v | s} => r == {b: v | t}, s == {a: u | t}
        (
            Parameter::Req {
                loc: aloc,
                lab: a,
                typ: u,
                ext: r,
            },
            Parameter::Pipe {
                loc: bloc,
                lab: b,
                typ: v,
                ext: s,
            },
        )
        | (
            Parameter::Pipe {
                loc: bloc,
                lab: b,
                typ: v,
                ext: s,
            },
            Parameter::Req {
                loc: aloc,
                lab: a,
                typ: u,
                ext: r,
            },
        ) => match a == b {
            true => {
                let sub = unify_types(u, v, cons, f)?;
                apply_then_unify(r, s, sub, cons, f)
            }
            false => {
                let i = f.fresh();
                let x = MonoType::Par(Box::new(Parameter::Pipe {
                    loc: bloc,
                    lab: b,
                    typ: v,
                    ext: MonoType::Var(i),
                }));
                let y = MonoType::Par(Box::new(Parameter::Req {
                    loc: aloc,
                    lab: a,
                    typ: u,
                    ext: MonoType::Var(i),
                }));
                let sub = unify_types(r, x, cons, f)?;
                apply_then_unify(s, y, sub, cons, f)
            }
        },
        // 7. {>: _ | _} != {?: _ | _}
        // 9. {a: u | r} == {b: v | s} => r == {b: v | t}, s == {a: u | t}
        (
            Parameter::Opt {
                loc: aloc,
                lab: a,
                typ: u,
                ext: r,
            },
            Parameter::Pipe {
                loc: bloc,
                lab: b,
                typ: v,
                ext: s,
            },
        )
        | (
            Parameter::Pipe {
                loc: bloc,
                lab: b,
                typ: v,
                ext: s,
            },
            Parameter::Opt {
                loc: aloc,
                lab: a,
                typ: u,
                ext: r,
            },
        ) => match a == b {
            true => Err(PipeError::Optional { arg: a, loc: aloc }.into()),
            false => {
                let i = f.fresh();
                let x = MonoType::Par(Box::new(Parameter::Pipe {
                    loc: bloc,
                    lab: b,
                    typ: v,
                    ext: MonoType::Var(i),
                }));
                let y = MonoType::Par(Box::new(Parameter::Opt {
                    loc: aloc,
                    lab: a,
                    typ: u,
                    ext: MonoType::Var(i),
                }));
                let sub = unify_types(r, x, cons, f)?;
                apply_then_unify(s, y, sub, cons, f)
            }
        },
    }
}

#[derive(Debug, Clone, PartialEq)]
pub struct Fun {
    pub x: MonoType,
    pub e: MonoType,
    pub loc: ast::SourceLocation,
}

impl fmt::Display for Fun {
    fn fmt(&self, f: &mut fmt::Formatter) -> fmt::Result {
        write!(f, "({}) -> {}", self.x, self.e)
    }
}

impl Substitutable for Fun {
    fn apply(self, sub: &Substitution) -> Self {
        Fun {
            x: self.x.apply(sub),
            e: self.e.apply(sub),
            loc: self.loc,
        }
    }
    fn free_vars(&self) -> Vec<Tvar> {
        union(self.x.free_vars(), self.e.free_vars())
    }
}

impl MaxTvar for Fun {
    fn max_tvar(&self) -> Tvar {
        vec![self.x.max_tvar(), self.e.max_tvar()].max_tvar()
    }
}

fn unify_function(
    exp: Fun,
    act: Fun,
    cons: &mut TvarKinds,
    f: &mut Fresher,
) -> Result<Substitution, Error> {
    let sub = unify_types(exp.x, act.x, cons, f)?;
    apply_then_unify(exp.e, act.e, sub, cons, f)
}

impl Fun {
    fn loc(&self) -> &ast::SourceLocation {
        &self.loc
    }
    fn contains(&self, tv: Tvar) -> bool {
        self.x.contains(tv) || self.e.contains(tv)
    }
}

// Unification requires that the current substitution be applied
// to both sides of a constraint before unifying.
//
// This helper function applies a substitution to a constraint
// before unifying the two types. Note the substitution produced
// from unification is merged with input substitution before it
// is returned.
//
fn apply_then_unify(
    exp: MonoType,
    act: MonoType,
    sub: Substitution,
    cons: &mut TvarKinds,
    f: &mut Fresher,
) -> Result<Substitution, Error> {
    Ok(sub.merge(unify_types(exp.apply(&sub), act.apply(&sub), cons, f)?))
}
