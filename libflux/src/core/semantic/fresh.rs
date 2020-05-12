use crate::semantic::types::{
    Array, Fun, MonoType, Parameter, PolyType, Property, Record, SemanticMap, Tvar, TvarMap,
};
use std::collections::BTreeMap;
use std::hash::Hash;

// Fresher returns incrementing type variables
pub struct Fresher(pub u64);

// Create a tvar fresher from a u64
impl From<u64> for Fresher {
    fn from(u: u64) -> Fresher {
        Fresher(u)
    }
}

impl Fresher {
    pub fn fresh(&mut self) -> Tvar {
        let u = self.0;
        self.0 += 1;
        Tvar(u)
    }
}

impl Default for Fresher {
    fn default() -> Self {
        Self(0)
    }
}

pub trait Fresh {
    fn fresh(self, f: &mut Fresher, sub: &mut TvarMap) -> Self;
}

impl<T: Fresh> Fresh for Vec<T> {
    fn fresh(self, f: &mut Fresher, sub: &mut TvarMap) -> Self {
        self.into_iter().map(|t| t.fresh(f, sub)).collect::<Self>()
    }
}

impl<T: Fresh> Fresh for Option<T> {
    fn fresh(self, f: &mut Fresher, sub: &mut TvarMap) -> Self {
        match self {
            None => None,
            Some(t) => Some(t.fresh(f, sub)),
        }
    }
}

#[allow(clippy::implicit_hasher)]
impl<T: Fresh> Fresh for SemanticMap<String, T> {
    fn fresh(self, f: &mut Fresher, sub: &mut TvarMap) -> Self {
        self.into_iter()
            .collect::<BTreeMap<String, T>>()
            .into_iter()
            .map(|(s, t)| (s, t.fresh(f, sub)))
            .collect::<Self>()
    }
}

#[allow(clippy::implicit_hasher)]
impl<T: Hash + Ord + Eq + Fresh, S> Fresh for SemanticMap<T, S> {
    fn fresh(self, f: &mut Fresher, sub: &mut TvarMap) -> Self {
        self.into_iter()
            .collect::<BTreeMap<T, S>>()
            .into_iter()
            .map(|(t, s)| (t.fresh(f, sub), s))
            .collect::<Self>()
    }
}

impl<T: Fresh> Fresh for Box<T> {
    fn fresh(self, f: &mut Fresher, sub: &mut TvarMap) -> Self {
        Box::new((*self).fresh(f, sub))
    }
}

impl Fresh for PolyType {
    fn fresh(self, f: &mut Fresher, sub: &mut TvarMap) -> Self {
        let expr = self.expr.fresh(f, sub);
        let vars = self.vars.fresh(f, sub);
        let cons = self.cons.fresh(f, sub);
        PolyType { vars, cons, expr }
    }
}

impl Fresh for MonoType {
    fn fresh(self, f: &mut Fresher, sub: &mut TvarMap) -> Self {
        match self {
            MonoType::Bool
            | MonoType::Int
            | MonoType::Uint
            | MonoType::Float
            | MonoType::String
            | MonoType::Time
            | MonoType::Duration
            | MonoType::Regexp
            | MonoType::Bytes => self,
            MonoType::Var(tvr) => MonoType::Var(tvr.fresh(f, sub)),
            MonoType::Arr(arr) => MonoType::Arr(arr.fresh(f, sub)),
            MonoType::Obj(obj) => MonoType::Obj(obj.fresh(f, sub)),
            MonoType::Par(par) => MonoType::Par(par.fresh(f, sub)),
            MonoType::Fnc(fun) => MonoType::Fnc(fun.fresh(f, sub)),
        }
    }
}

impl Fresh for Record {
    fn fresh(self, f: &mut Fresher, sub: &mut TvarMap) -> Self {
        match self {
            Record::Empty => Record::Empty,
            Record::Extension {
                loc: l,
                lab: a,
                typ: t,
                ext: r,
            } => Record::Extension {
                loc: l,
                lab: a,
                typ: t.fresh(f, sub),
                ext: r.fresh(f, sub),
            },
        }
    }
}

impl Fresh for Parameter {
    fn fresh(self, f: &mut Fresher, sub: &mut TvarMap) -> Self {
        match self {
            Parameter::None => Parameter::None,
            Parameter::Req {
                loc: l,
                lab: a,
                typ: t,
                ext: r,
            } => Parameter::Req {
                loc: l,
                lab: a,
                typ: t.fresh(f, sub),
                ext: r.fresh(f, sub),
            },
            Parameter::Opt {
                loc: l,
                lab: a,
                typ: t,
                ext: r,
            } => Parameter::Opt {
                loc: l,
                lab: a,
                typ: t.fresh(f, sub),
                ext: r.fresh(f, sub),
            },
            Parameter::Pipe {
                loc: l,
                lab: a,
                typ: t,
                ext: r,
            } => Parameter::Pipe {
                loc: l,
                lab: a,
                typ: t.fresh(f, sub),
                ext: r.fresh(f, sub),
            },
        }
    }
}

impl Fresh for Fun {
    fn fresh(self, f: &mut Fresher, sub: &mut TvarMap) -> Self {
        Fun {
            x: self.x.fresh(f, sub),
            e: self.e.fresh(f, sub),
        }
    }
}

impl Fresh for Tvar {
    fn fresh(self, f: &mut Fresher, sub: &mut TvarMap) -> Self {
        *sub.entry(self).or_insert_with(|| f.fresh())
    }
}

impl Fresh for Array {
    fn fresh(self, f: &mut Fresher, sub: &mut TvarMap) -> Self {
        Array(self.0.fresh(f, sub))
    }
}

impl Fresh for Property {
    fn fresh(self, f: &mut Fresher, sub: &mut TvarMap) -> Self {
        Property {
            k: self.k,
            v: self.v.fresh(f, sub),
        }
    }
}
