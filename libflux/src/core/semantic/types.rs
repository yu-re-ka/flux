use crate::ast::SourceLocation;
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
    RecordType {
        err: RecordTypeError,
    },
    ParamType {
        err: ParamTypeError,
    },
    PipeArgument {
        err: PipeError,
    },
    CannotUnify {
        expected: MonoType,
        actual: MonoType,
    },
    CannotConstrain {
        expected: Kind,
        actual: MonoType,
    },
    OccursCheck {
        tv: Tvar,
        ty: MonoType,
    },
}

impl fmt::Display for Error {
    fn fmt(&self, f: &mut fmt::Formatter) -> fmt::Result {
        match self {
            Error::RecordType { err } => write!(f, "{}", err),
            Error::ParamType { err } => write!(f, "{}", err),
            Error::PipeArgument { err } => write!(f, "{}", err),
            Error::CannotUnify { expected, actual } => write!(
                f,
                "types do not match\nexpected: {}\nfound: {}",
                expected, actual
            ),
            Error::CannotConstrain { expected, actual } => {
                write!(f, "{} is not {}", expected, actual)
            }
            Error::OccursCheck { tv, ty } => write!(f, "recursive type {} != {}", tv, ty),
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
    Bool,
    Int,
    Uint,
    Float,
    String,
    Duration,
    Time,
    Regexp,
    Bytes,
    Var(Tvar),
    Arr(Box<Array>),
    Obj(Box<Record>),
    Par(Box<Parameter>),
    Fun(Box<Function>),
}

pub type MonoTypeMap = SemanticMap<String, MonoType>;
pub type MonoTypeVecMap = SemanticMap<String, Vec<MonoType>>;

impl fmt::Display for MonoType {
    fn fmt(&self, f: &mut fmt::Formatter) -> fmt::Result {
        match self {
            MonoType::Bool => f.write_str("bool"),
            MonoType::Int => f.write_str("int"),
            MonoType::Uint => f.write_str("uint"),
            MonoType::Float => f.write_str("float"),
            MonoType::String => f.write_str("string"),
            MonoType::Duration => f.write_str("duration"),
            MonoType::Time => f.write_str("time"),
            MonoType::Regexp => f.write_str("regexp"),
            MonoType::Bytes => f.write_str("bytes"),
            MonoType::Var(var) => var.fmt(f),
            MonoType::Arr(arr) => arr.fmt(f),
            MonoType::Obj(obj) => write!(f, "{}", obj),
            MonoType::Par(par) => write!(f, "{}", par),
            MonoType::Fun(fun) => write!(f, "{}", fun),
        }
    }
}

impl Substitutable for MonoType {
    fn apply(self, sub: &Substitution) -> Self {
        match self {
            MonoType::Bool
            | MonoType::Int
            | MonoType::Uint
            | MonoType::Float
            | MonoType::String
            | MonoType::Duration
            | MonoType::Time
            | MonoType::Regexp
            | MonoType::Bytes => self,
            MonoType::Var(tvr) => sub.apply(tvr),
            MonoType::Arr(arr) => MonoType::Arr(Box::new(arr.apply(sub))),
            MonoType::Obj(obj) => MonoType::Obj(Box::new(obj.apply(sub))),
            MonoType::Par(par) => MonoType::Par(Box::new(par.apply(sub))),
            MonoType::Fun(fun) => MonoType::Fun(Box::new(fun.apply(sub))),
        }
    }
    fn free_vars(&self) -> Vec<Tvar> {
        match self {
            MonoType::Bool
            | MonoType::Int
            | MonoType::Uint
            | MonoType::Float
            | MonoType::String
            | MonoType::Duration
            | MonoType::Time
            | MonoType::Regexp
            | MonoType::Bytes => Vec::new(),
            MonoType::Var(tvr) => vec![*tvr],
            MonoType::Arr(arr) => arr.free_vars(),
            MonoType::Obj(obj) => obj.free_vars(),
            MonoType::Par(par) => par.free_vars(),
            MonoType::Fun(fun) => fun.free_vars(),
        }
    }
}

impl MaxTvar for MonoType {
    fn max_tvar(&self) -> Tvar {
        match self {
            MonoType::Bool
            | MonoType::Int
            | MonoType::Uint
            | MonoType::Float
            | MonoType::String
            | MonoType::Duration
            | MonoType::Time
            | MonoType::Regexp
            | MonoType::Bytes => Tvar(0),
            MonoType::Var(tvr) => tvr.max_tvar(),
            MonoType::Arr(arr) => arr.max_tvar(),
            MonoType::Obj(obj) => obj.max_tvar(),
            MonoType::Par(par) => par.max_tvar(),
            MonoType::Fun(fun) => fun.max_tvar(),
        }
    }
}

impl MonoType {
    pub fn unify(
        self,
        with: Self,
        cons: &mut TvarKinds,
        f: &mut Fresher,
    ) -> Result<Substitution, Error> {
        match (self, with) {
            (MonoType::Bool, MonoType::Bool)
            | (MonoType::Int, MonoType::Int)
            | (MonoType::Uint, MonoType::Uint)
            | (MonoType::Float, MonoType::Float)
            | (MonoType::String, MonoType::String)
            | (MonoType::Duration, MonoType::Duration)
            | (MonoType::Time, MonoType::Time)
            | (MonoType::Regexp, MonoType::Regexp)
            | (MonoType::Bytes, MonoType::Bytes) => Ok(Substitution::empty()),
            (MonoType::Var(tv), t) => tv.unify(t, cons),
            (t, MonoType::Var(tv)) => tv.unify(t, cons),
            (MonoType::Arr(t), MonoType::Arr(s)) => t.unify(*s, cons, f),
            (MonoType::Obj(t), MonoType::Obj(s)) => unify_records(*t, *s, cons, f),
            (MonoType::Par(t), MonoType::Par(s)) => unify_params(*t, *s, cons, f),
            (MonoType::Fun(t), MonoType::Fun(s)) => unify_function(*t, *s, cons, f),
            (t, with) => Err(Error::CannotUnify {
                expected: t,
                actual: with,
            }),
        }
    }

    pub fn constrain(self, with: Kind, cons: &mut TvarKinds) -> Result<Substitution, Error> {
        match self {
            MonoType::Bool => match with {
                Kind::Equatable | Kind::Nullable => Ok(Substitution::empty()),
                _ => Err(Error::CannotConstrain {
                    actual: self,
                    expected: with,
                }),
            },
            MonoType::Int => match with {
                Kind::Addable
                | Kind::Subtractable
                | Kind::Divisible
                | Kind::Numeric
                | Kind::Comparable
                | Kind::Equatable
                | Kind::Nullable
                | Kind::Negatable => Ok(Substitution::empty()),
                _ => Err(Error::CannotConstrain {
                    actual: self,
                    expected: with,
                }),
            },
            MonoType::Uint => match with {
                Kind::Addable
                | Kind::Subtractable
                | Kind::Divisible
                | Kind::Numeric
                | Kind::Comparable
                | Kind::Equatable
                | Kind::Nullable
                | Kind::Negatable => Ok(Substitution::empty()),
                _ => Err(Error::CannotConstrain {
                    actual: self,
                    expected: with,
                }),
            },
            MonoType::Float => match with {
                Kind::Addable
                | Kind::Subtractable
                | Kind::Divisible
                | Kind::Numeric
                | Kind::Comparable
                | Kind::Equatable
                | Kind::Nullable
                | Kind::Negatable => Ok(Substitution::empty()),
                _ => Err(Error::CannotConstrain {
                    actual: self,
                    expected: with,
                }),
            },
            MonoType::String => match with {
                Kind::Addable | Kind::Comparable | Kind::Equatable | Kind::Nullable => {
                    Ok(Substitution::empty())
                }
                _ => Err(Error::CannotConstrain {
                    actual: self,
                    expected: with,
                }),
            },
            MonoType::Duration => match with {
                Kind::Comparable | Kind::Equatable | Kind::Nullable | Kind::Negatable => {
                    Ok(Substitution::empty())
                }
                _ => Err(Error::CannotConstrain {
                    actual: self,
                    expected: with,
                }),
            },
            MonoType::Time => match with {
                Kind::Comparable | Kind::Equatable | Kind::Nullable => Ok(Substitution::empty()),
                _ => Err(Error::CannotConstrain {
                    actual: self,
                    expected: with,
                }),
            },
            MonoType::Regexp => Err(Error::CannotConstrain {
                actual: self,
                expected: with,
            }),
            MonoType::Bytes => match with {
                Kind::Equatable => Ok(Substitution::empty()),
                _ => Err(Error::CannotConstrain {
                    actual: self,
                    expected: with,
                }),
            },
            MonoType::Var(tvr) => {
                tvr.constrain(with, cons);
                Ok(Substitution::empty())
            }
            MonoType::Arr(arr) => arr.constrain(with, cons),
            MonoType::Obj(obj) => obj.constrain(with, cons),
            MonoType::Par(par) => par.constrain(with, cons),
            MonoType::Fun(fun) => fun.constrain(with, cons),
        }
    }

    fn contains(&self, tv: Tvar) -> bool {
        match self {
            MonoType::Bool
            | MonoType::Int
            | MonoType::Uint
            | MonoType::Float
            | MonoType::String
            | MonoType::Duration
            | MonoType::Time
            | MonoType::Regexp
            | MonoType::Bytes => false,
            MonoType::Var(tvr) => tv == *tvr,
            MonoType::Arr(arr) => arr.contains(tv),
            MonoType::Obj(obj) => obj.contains(tv),
            MonoType::Par(par) => par.contains(tv),
            MonoType::Fun(fun) => fun.contains(tv),
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
            Some(kinds) => {
                let mut s = Substitution::empty();
                for kind in kinds {
                    // The monotype that is being unified with the
                    // tvar must be constrained with the same kinds
                    // as that of the tvar.
                    s = s.merge(t.clone().constrain(kind, cons)?);
                }
                Ok(sub.merge(s))
            }
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
pub struct Array(pub MonoType);

impl fmt::Display for Array {
    fn fmt(&self, f: &mut fmt::Formatter) -> fmt::Result {
        write!(f, "[{}]", self.0)
    }
}

impl Substitutable for Array {
    fn apply(self, sub: &Substitution) -> Self {
        Array(self.0.apply(sub))
    }
    fn free_vars(&self) -> Vec<Tvar> {
        self.0.free_vars()
    }
}

impl MaxTvar for Array {
    fn max_tvar(&self) -> Tvar {
        self.0.max_tvar()
    }
}

impl Array {
    fn unify(
        self,
        with: Self,
        cons: &mut TvarKinds,
        f: &mut Fresher,
    ) -> Result<Substitution, Error> {
        self.0.unify(with.0, cons, f)
    }

    fn constrain(self, with: Kind, cons: &mut TvarKinds) -> Result<Substitution, Error> {
        match with {
            Kind::Equatable => self.0.constrain(with, cons),
            _ => Err(Error::CannotConstrain {
                expected: with,
                actual: MonoType::Arr(Box::new(self)),
            }),
        }
    }

    fn contains(&self, tv: Tvar) -> bool {
        self.0.contains(tv)
    }
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

#[derive(Debug, Clone)]
pub enum Record {
    Empty {
        loc: Option<SourceLocation>,
    },
    Extension {
        loc: Option<SourceLocation>,
        lab: String,
        typ: MonoType,
        ext: MonoType,
    },
}

impl PartialEq for Record {
    fn eq(&self, other: &Self) -> bool {
        match (self, other) {
            (Record::Empty { .. }, Record::Empty { .. }) => true,
            (
                Record::Extension {
                    loc: _,
                    lab: a,
                    typ: u,
                    ext: r,
                },
                Record::Extension {
                    loc: _,
                    lab: b,
                    typ: v,
                    ext: s,
                },
            ) => a == b && u == v && r == s,
            (Record::Empty { .. }, Record::Extension { .. }) => false,
            (Record::Extension { .. }, Record::Empty { .. }) => false,
        }
    }
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
    fn constrain(self, with: Kind, cons: &mut TvarKinds) -> Result<Substitution, Error> {
        match with {
            Kind::Row => Ok(Substitution::empty()),
            Kind::Equatable => match self {
                Record::Empty { .. } => Ok(Substitution::empty()),
                Record::Extension { typ, ext, .. } => {
                    let sub = typ.constrain(with, cons)?;
                    Ok(sub.merge(ext.constrain(with, cons)?))
                }
            },
            _ => Err(Error::CannotConstrain {
                expected: with,
                actual: MonoType::Obj(Box::new(self)),
            }),
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
pub enum RecordTypeError {
    NotFound {
        lab: String,
        loc: Option<SourceLocation>,
    },
    Unexpected {
        lab: String,
        loc: Option<SourceLocation>,
    },
    Mismatch {
        exp: String,
        act: String,
        loc: Option<SourceLocation>,
    },
}

impl fmt::Display for RecordTypeError {
    fn fmt(&self, f: &mut fmt::Formatter) -> fmt::Result {
        match self {
            RecordTypeError::NotFound { lab, .. } => write!(f, "label {} not found", lab),
            RecordTypeError::Unexpected { lab, .. } => write!(f, "unexpected label {}", lab),
            RecordTypeError::Mismatch { exp, act, .. } => write!(
                f,
                "unexpected record labels\nExpected: {}\nFound: {}",
                exp, act
            ),
        }
    }
}

impl From<RecordTypeError> for Error {
    fn from(err: RecordTypeError) -> Self {
        Error::RecordType { err }
    }
}

// Rules for record unification. In what follows, a and b are labels
// where a != b, and t is a type variable.
//
// 1. {} == {}
// 2. {a: _ | _} != {}
// 3. {a: _ | t} != {b: _ | t}
// 4. {a: u | t} == {a: v | t} => u == v
// 5. {a: u | r} == {a: v | s} => u == v, r == s
// 6. {a: u | r} == {b: v | s} => r == {b: v | t}, s == {a: u | t}
//
fn unify_records(
    exp: Record,
    act: Record,
    cons: &mut TvarKinds,
    f: &mut Fresher,
) -> Result<Substitution, Error> {
    match (exp, act) {
        // 1. {} == {}
        (Record::Empty { .. }, Record::Empty { .. }) => Ok(Substitution::empty()),
        // 2. {a: _ | _} != {}
        (Record::Extension { lab: a, .. }, Record::Empty { loc }) => {
            Err(RecordTypeError::NotFound { lab: a, loc }.into())
        }
        (Record::Empty { .. }, Record::Extension { lab: a, loc, .. }) => {
            Err(RecordTypeError::Unexpected { lab: a, loc }.into())
        }
        // 3. {a: _ | t} != {b: _ | t}
        // 4. {a: u | t} == {a: v | t} => u == v
        (
            Record::Extension {
                loc: _,
                lab: a,
                typ: u,
                ext: MonoType::Var(r),
            },
            Record::Extension {
                loc: l,
                lab: b,
                typ: v,
                ext: MonoType::Var(s),
            },
        ) if r == s => match a == b {
            true => u.unify(v, cons, f),
            false => Err(RecordTypeError::Mismatch {
                exp: a,
                act: b,
                loc: l,
            }
            .into()),
        },
        // 5. {a: u | r} == {a: v | s} => u == v, r == s
        // 6. {a: u | r} == {b: v | s} => r == {b: v | t}, s == {a: u | t}
        (
            Record::Extension {
                lab: a,
                typ: u,
                ext: r,
                loc: k,
            },
            Record::Extension {
                lab: b,
                typ: v,
                ext: s,
                loc: l,
            },
        ) => match a == b {
            true => {
                let sub = u.unify(v, cons, f)?;
                apply_then_unify(r, s, sub, cons, f)
            }
            false => {
                let i = f.fresh();
                let x = MonoType::Obj(Box::new(Record::Extension {
                    loc: l,
                    lab: b,
                    typ: v,
                    ext: MonoType::Var(i),
                }));
                let y = MonoType::Obj(Box::new(Record::Extension {
                    loc: k,
                    lab: a,
                    typ: u,
                    ext: MonoType::Var(i),
                }));
                let sub = r.unify(x, cons, f)?;
                apply_then_unify(y, s, sub, cons, f)
            }
        },
    }
}

#[derive(Debug, Clone)]
pub enum Parameter {
    None {
        loc: Option<SourceLocation>,
    },
    Req {
        loc: Option<SourceLocation>,
        lab: String,
        typ: MonoType,
        ext: MonoType,
    },
    Opt {
        loc: Option<SourceLocation>,
        lab: String,
        typ: MonoType,
        ext: MonoType,
    },
    Pipe {
        loc: Option<SourceLocation>,
        lab: Option<String>,
        typ: MonoType,
        ext: MonoType,
    },
}

impl PartialEq for Parameter {
    fn eq(&self, other: &Self) -> bool {
        match (self, other) {
            (Parameter::None { .. }, Parameter::None { .. }) => true,
            (
                Parameter::Req {
                    loc: _,
                    lab: a,
                    typ: u,
                    ext: r,
                },
                Parameter::Req {
                    loc: _,
                    lab: b,
                    typ: v,
                    ext: s,
                },
            )
            | (
                Parameter::Opt {
                    loc: _,
                    lab: a,
                    typ: u,
                    ext: r,
                },
                Parameter::Opt {
                    loc: _,
                    lab: b,
                    typ: v,
                    ext: s,
                },
            ) => a == b && u == v && r == s,
            (
                Parameter::Pipe {
                    loc: _,
                    lab: a,
                    typ: u,
                    ext: r,
                },
                Parameter::Pipe {
                    loc: _,
                    lab: b,
                    typ: v,
                    ext: s,
                },
            ) => a == b && u == v && r == s,
            _ => false,
        }
    }
}

impl fmt::Display for Parameter {
    fn fmt(&self, f: &mut fmt::Formatter) -> fmt::Result {
        match self {
            Parameter::None { .. } => Ok(()),
            Parameter::Req {
                loc: _,
                lab: a,
                typ: t,
                ext: r,
            } => write!(f, "{}:{}, {}", a, t, r),
            Parameter::Opt {
                loc: _,
                lab: a,
                typ: t,
                ext: r,
            } => write!(f, "?{}:{}, {}", a, t, r),
            Parameter::Pipe {
                loc: _,
                lab: None,
                typ: t,
                ext: r,
            } => write!(f, "<-:{}, {}", t, r),
            Parameter::Pipe {
                loc: _,
                lab: Some(a),
                typ: t,
                ext: r,
            } => write!(f, "<-{}:{}, {}", a, t, r),
        }
    }
}

impl Substitutable for Parameter {
    fn apply(self, sub: &Substitution) -> Self {
        match self {
            Parameter::None { .. } => self,
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
            Parameter::Req {
                loc: _,
                lab: _,
                typ: t,
                ext: r,
            } => union(t.free_vars(), r.free_vars()),
            Parameter::Opt {
                loc: _,
                lab: _,
                typ: t,
                ext: r,
            } => union(t.free_vars(), r.free_vars()),
            Parameter::Pipe {
                loc: _,
                lab: _,
                typ: t,
                ext: r,
            } => union(t.free_vars(), r.free_vars()),
        }
    }
}

impl MaxTvar for Parameter {
    fn max_tvar(&self) -> Tvar {
        match self {
            Parameter::None { .. } => Tvar(0),
            Parameter::Req {
                loc: _,
                lab: _,
                typ: t,
                ext: r,
            } => vec![t.max_tvar(), r.max_tvar()].max_tvar(),
            Parameter::Opt {
                loc: _,
                lab: _,
                typ: t,
                ext: r,
            } => vec![t.max_tvar(), r.max_tvar()].max_tvar(),
            Parameter::Pipe {
                loc: _,
                lab: _,
                typ: t,
                ext: r,
            } => vec![t.max_tvar(), r.max_tvar()].max_tvar(),
        }
    }
}

impl Parameter {
    fn constrain(self, with: Kind, _: &mut TvarKinds) -> Result<Substitution, Error> {
        Err(Error::CannotConstrain {
            expected: with,
            actual: MonoType::Par(Box::new(self)),
        })
    }
    fn contains(&self, tv: Tvar) -> bool {
        match self {
            Parameter::None { .. } => false,
            Parameter::Req {
                loc: _,
                lab: _,
                typ: t,
                ext: r,
            } => t.contains(tv) || r.contains(tv),
            Parameter::Opt {
                loc: _,
                lab: _,
                typ: t,
                ext: r,
            } => t.contains(tv) || r.contains(tv),
            Parameter::Pipe {
                loc: _,
                lab: _,
                typ: t,
                ext: r,
            } => t.contains(tv) || r.contains(tv),
        }
    }
}

#[derive(Debug, PartialEq)]
pub enum ParamTypeError {
    NotFound {
        arg: String,
        loc: Option<SourceLocation>,
    },
    Unexpected {
        arg: String,
        loc: Option<SourceLocation>,
    },
    Mismatch {
        exp: String,
        act: String,
        loc: Option<SourceLocation>,
    },
}

impl fmt::Display for ParamTypeError {
    fn fmt(&self, f: &mut fmt::Formatter) -> fmt::Result {
        match self {
            ParamTypeError::NotFound { arg, .. } => {
                write!(f, "required argument {} not found", arg)
            }
            ParamTypeError::Unexpected { arg, .. } => {
                write!(f, "function does not take an argument named {}", arg)
            }
            ParamTypeError::Mismatch { exp, act, .. } => write!(
                f,
                "unexpected function argument\nExpected: {}\nFound: {}",
                exp, act
            ),
        }
    }
}

impl From<ParamTypeError> for Error {
    fn from(err: ParamTypeError) -> Self {
        Error::ParamType { err }
    }
}

#[derive(Debug, PartialEq)]
pub enum PipeError {
    NotFound {
        loc: Option<SourceLocation>,
    },
    Unexpected {
        loc: Option<SourceLocation>,
    },
    Optional {
        arg: String,
        loc: Option<SourceLocation>,
    },
    Mismatch {
        exp: String,
        act: String,
        loc: Option<SourceLocation>,
    },
}

impl fmt::Display for PipeError {
    fn fmt(&self, f: &mut fmt::Formatter) -> fmt::Result {
        match self {
            PipeError::NotFound { .. } => write!(f, "pipe argument not found"),
            PipeError::Unexpected { .. } => write!(f, "function does not take a pipe argument"),
            PipeError::Optional { arg, .. } => {
                write!(f, "pipe argument {} is not allowed to be optional", arg)
            }
            PipeError::Mismatch { exp, act, .. } => write!(
                f,
                "pipe arguments do not match\nExpected: {}\nFound: {}",
                exp, act
            ),
        }
    }
}

impl From<PipeError> for Error {
    fn from(err: PipeError) -> Self {
        Error::PipeArgument { err }
    }
}

// Rules for parameter unification. In what follows, a and b are
// parameters where a != b, t is a type variable, ? is an optional
// parameter, and > is a pipe parameter.
//
// 1. {} == {}
// 2. {?: _ | r} == {} => r == {}
// 3. {_: _ | _} != {}
// 4. {a: u | r} == {a: v | s} => u == v, r == s
// 5. {a: u | r} == {b: v | s} => r == {b: v | t}, s == {a: u | t}
// 6. {>: _ | _} != {?: _ | _}
//
fn unify_params(
    exp: Parameter,
    act: Parameter,
    cons: &mut TvarKinds,
    f: &mut Fresher,
) -> Result<Substitution, Error> {
    match (exp, act) {
        // 1. {} == {}
        (Parameter::None { .. }, Parameter::None { .. }) => Ok(Substitution::empty()),
        // 2. {?: _ | r} == {} => r == {}
        (Parameter::Opt { ext: r, .. }, Parameter::None { loc })
        | (Parameter::None { loc }, Parameter::Opt { ext: r, .. }) => {
            MonoType::Par(Box::new(Parameter::None { loc })).unify(r, cons, f)
        }
        // 2. {a: _ | _} != {}
        (Parameter::Req { lab: a, .. }, Parameter::None { loc })
        | (Parameter::None { .. }, Parameter::Req { lab: a, loc, .. }) => {
            Err(ParamTypeError::Unexpected { arg: a, loc }.into())
        }
        // 2. {>: _ | _} != {}
        (Parameter::Pipe { .. }, Parameter::None { loc })
        | (Parameter::None { .. }, Parameter::Pipe { loc, .. }) => {
            Err(PipeError::Unexpected { loc }.into())
        }
        // 4. {a: u | r} == {a: v | s} => u == v, r == s
        // 5. {a: u | r} == {b: v | s} => r == {b: v | t}, s == {a: u | t}
        (
            Parameter::Pipe {
                loc: _,
                lab: None,
                typ: u,
                ext: r,
            },
            Parameter::Pipe {
                loc: _,
                lab: _,
                typ: v,
                ext: s,
            },
        )
        | (
            Parameter::Pipe {
                loc: _,
                lab: _,
                typ: u,
                ext: r,
            },
            Parameter::Pipe {
                loc: _,
                lab: None,
                typ: v,
                ext: s,
            },
        ) => {
            let sub = u.unify(v, cons, f)?;
            apply_then_unify(r, s, sub, cons, f)
        }
        (
            Parameter::Pipe {
                loc: _,
                lab: Some(a),
                typ: u,
                ext: r,
            },
            Parameter::Pipe {
                loc: l,
                lab: Some(b),
                typ: v,
                ext: s,
            },
        ) => match a == b {
            true => {
                let sub = u.unify(v, cons, f)?;
                apply_then_unify(r, s, sub, cons, f)
            }
            false => Err(PipeError::Mismatch {
                exp: a,
                act: b,
                loc: l,
            }
            .into()),
        },
        // 4. {a: u | r} == {a: v | s} => u == v, r == s
        // 5. {a: u | r} == {b: v | s} => r == {b: v | t}, s == {a: u | t}
        (
            Parameter::Req {
                loc: k,
                lab: a,
                typ: u,
                ext: r,
            },
            Parameter::Req {
                loc: l,
                lab: b,
                typ: v,
                ext: s,
            },
        ) => match a == b {
            true => {
                let sub = u.unify(v, cons, f)?;
                apply_then_unify(r, s, sub, cons, f)
            }
            false => {
                let i = f.fresh();
                let x = MonoType::Par(Box::new(Parameter::Req {
                    loc: l,
                    lab: b,
                    typ: v,
                    ext: MonoType::Var(i),
                }));
                let y = MonoType::Par(Box::new(Parameter::Req {
                    loc: k,
                    lab: a,
                    typ: u,
                    ext: MonoType::Var(i),
                }));
                let sub = r.unify(x, cons, f)?;
                apply_then_unify(y, s, sub, cons, f)
            }
        },
        // 4. {a: u | r} == {a: v | s} => u == v, r == s
        // 5. {a: u | r} == {b: v | s} => r == {b: v | t}, s == {a: u | t}
        (
            Parameter::Opt {
                loc: k,
                lab: a,
                typ: u,
                ext: r,
            },
            Parameter::Opt {
                loc: l,
                lab: b,
                typ: v,
                ext: s,
            },
        ) => match a == b {
            true => {
                let sub = u.unify(v, cons, f)?;
                apply_then_unify(r, s, sub, cons, f)
            }
            false => {
                let i = f.fresh();
                let x = MonoType::Par(Box::new(Parameter::Opt {
                    loc: l,
                    lab: b,
                    typ: v,
                    ext: MonoType::Var(i),
                }));
                let y = MonoType::Par(Box::new(Parameter::Opt {
                    loc: k,
                    lab: a,
                    typ: u,
                    ext: MonoType::Var(i),
                }));
                let sub = r.unify(x, cons, f)?;
                apply_then_unify(y, s, sub, cons, f)
            }
        },
        // 4. {a: u | r} == {a: v | s} => u == v, r == s
        // 5. {a: u | r} == {b: v | s} => r == {b: v | t}, s == {a: u | t}
        (
            Parameter::Req {
                loc: k,
                lab: a,
                typ: u,
                ext: r,
            },
            Parameter::Opt {
                loc: l,
                lab: b,
                typ: v,
                ext: s,
            },
        ) => match a == b {
            true => {
                let sub = u.unify(v, cons, f)?;
                apply_then_unify(r, s, sub, cons, f)
            }
            false => {
                let i = f.fresh();
                let x = MonoType::Par(Box::new(Parameter::Opt {
                    loc: l,
                    lab: b,
                    typ: v,
                    ext: MonoType::Var(i),
                }));
                let y = MonoType::Par(Box::new(Parameter::Req {
                    loc: k,
                    lab: a,
                    typ: u,
                    ext: MonoType::Var(i),
                }));
                let sub = r.unify(x, cons, f)?;
                apply_then_unify(y, s, sub, cons, f)
            }
        },
        // 4. {a: u | r} == {a: v | s} => u == v, r == s
        // 5. {a: u | r} == {b: v | s} => r == {b: v | t}, s == {a: u | t}
        (
            Parameter::Opt {
                loc: k,
                lab: a,
                typ: u,
                ext: r,
            },
            Parameter::Req {
                loc: l,
                lab: b,
                typ: v,
                ext: s,
            },
        ) => match a == b {
            true => {
                let sub = u.unify(v, cons, f)?;
                apply_then_unify(r, s, sub, cons, f)
            }
            false => {
                let i = f.fresh();
                let x = MonoType::Par(Box::new(Parameter::Req {
                    loc: l,
                    lab: b,
                    typ: v,
                    ext: MonoType::Var(i),
                }));
                let y = MonoType::Par(Box::new(Parameter::Opt {
                    loc: k,
                    lab: a,
                    typ: u,
                    ext: MonoType::Var(i),
                }));
                let sub = r.unify(x, cons, f)?;
                apply_then_unify(y, s, sub, cons, f)
            }
        },
        // 3. {a: u | r} == {>: v | s} => u == v, r == s if a == >
        (
            Parameter::Req {
                loc: _,
                lab: a,
                typ: u,
                ext: r,
            },
            Parameter::Pipe {
                loc: _,
                lab: Some(b),
                typ: v,
                ext: s,
            },
        )
        | (
            Parameter::Pipe {
                loc: _,
                lab: Some(a),
                typ: u,
                ext: r,
            },
            Parameter::Req {
                loc: _,
                lab: b,
                typ: v,
                ext: s,
            },
        ) if a == b => {
            let sub = u.unify(v, cons, f)?;
            apply_then_unify(r, s, sub, cons, f)
        }
        // 5. {a: u | r} == {b: v | s} => r == {b: v | t}, s == {a: u | t}
        (
            Parameter::Req {
                loc: k,
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
            let i = f.fresh();
            let x = MonoType::Par(Box::new(Parameter::Pipe {
                loc: l,
                lab: b,
                typ: v,
                ext: MonoType::Var(i),
            }));
            let y = MonoType::Par(Box::new(Parameter::Req {
                loc: k,
                lab: a,
                typ: u,
                ext: MonoType::Var(i),
            }));
            let sub = r.unify(x, cons, f)?;
            apply_then_unify(y, s, sub, cons, f)
        }
        (
            Parameter::Pipe {
                loc: k,
                lab: a,
                typ: u,
                ext: r,
            },
            Parameter::Req {
                loc: l,
                lab: b,
                typ: v,
                ext: s,
            },
        ) => {
            let i = f.fresh();
            let x = MonoType::Par(Box::new(Parameter::Req {
                loc: l,
                lab: b,
                typ: v,
                ext: MonoType::Var(i),
            }));
            let y = MonoType::Par(Box::new(Parameter::Pipe {
                loc: k,
                lab: a,
                typ: u,
                ext: MonoType::Var(i),
            }));
            let sub = r.unify(x, cons, f)?;
            apply_then_unify(y, s, sub, cons, f)
        }
        // 6. {>: _ | _} != {?: _ | _}
        (
            Parameter::Opt { lab: a, .. },
            Parameter::Pipe {
                loc: l,
                lab: Some(b),
                typ: _,
                ext: _,
            },
        )
        | (
            Parameter::Pipe { lab: Some(a), .. },
            Parameter::Opt {
                loc: l,
                lab: b,
                typ: _,
                ext: _,
            },
        ) if a == b => Err(PipeError::Optional { arg: a, loc: l }.into()),
        // 5. {a: u | r} == {b: v | s} => r == {b: v | t}, s == {a: u | t}
        (
            Parameter::Opt {
                loc: k,
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
            let i = f.fresh();
            let x = MonoType::Par(Box::new(Parameter::Pipe {
                loc: l,
                lab: b,
                typ: v,
                ext: MonoType::Var(i),
            }));
            let y = MonoType::Par(Box::new(Parameter::Opt {
                loc: k,
                lab: a,
                typ: u,
                ext: MonoType::Var(i),
            }));
            let sub = r.unify(x, cons, f)?;
            apply_then_unify(y, s, sub, cons, f)
        }
        (
            Parameter::Pipe {
                loc: k,
                lab: a,
                typ: u,
                ext: r,
            },
            Parameter::Opt {
                loc: l,
                lab: b,
                typ: v,
                ext: s,
            },
        ) => {
            let i = f.fresh();
            let x = MonoType::Par(Box::new(Parameter::Opt {
                loc: l,
                lab: b,
                typ: v,
                ext: MonoType::Var(i),
            }));
            let y = MonoType::Par(Box::new(Parameter::Pipe {
                loc: k,
                lab: a,
                typ: u,
                ext: MonoType::Var(i),
            }));
            let sub = r.unify(x, cons, f)?;
            apply_then_unify(y, s, sub, cons, f)
        }
    }
}

#[derive(Debug, Clone, PartialEq)]
pub struct Function {
    pub x: MonoType,
    pub e: MonoType,
}

impl fmt::Display for Function {
    fn fmt(&self, f: &mut fmt::Formatter) -> fmt::Result {
        write!(f, "({}) -> {}", self.x, self.e)
    }
}

impl Substitutable for Function {
    fn apply(self, sub: &Substitution) -> Self {
        Function {
            x: self.x.apply(sub),
            e: self.e.apply(sub),
        }
    }
    fn free_vars(&self) -> Vec<Tvar> {
        union(self.x.free_vars(), self.e.free_vars())
    }
}

impl MaxTvar for Function {
    fn max_tvar(&self) -> Tvar {
        vec![self.x.max_tvar(), self.e.max_tvar()].max_tvar()
    }
}

fn unify_function(
    act: Function,
    exp: Function,
    cons: &mut TvarKinds,
    f: &mut Fresher,
) -> Result<Substitution, Error> {
    let sub = act.x.unify(exp.x, cons, f)?;
    apply_then_unify(act.e, exp.e, sub, cons, f)
}

impl Function {
    fn constrain(self, with: Kind, _: &mut TvarKinds) -> Result<Substitution, Error> {
        Err(Error::CannotConstrain {
            expected: with,
            actual: MonoType::Fun(Box::new(self)),
        })
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
    let exp = exp.apply(&sub);
    let act = act.apply(&sub);
    Ok(sub.merge(exp.unify(act, cons, f)?))
}
