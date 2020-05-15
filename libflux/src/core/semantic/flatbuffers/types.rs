//! This module defines methods for serializing and deserializing MonoTypes
//! and PolyTypes using the flatbuffer encoding.
//!
use crate::semantic::env::Environment;
use crate::semantic::flatbuffers::semantic_generated::fbsemantic as fb;

use flatbuffers;

use crate::semantic::fresh::Fresher;

#[rustfmt::skip]
use crate::semantic::types::{
    Array,
    Kind,
    MonoType,
    MonoTypeMap,
    PolyType,
    PolyTypeMap,
    Property,
    Tvar,
    TvarKinds,
    Function,
    Record,
    Parameter,
};

impl From<fb::Fresher<'_>> for Fresher {
    fn from(f: fb::Fresher) -> Fresher {
        Fresher::from(f.u())
    }
}

impl From<fb::TypeEnvironment<'_>> for Option<Environment> {
    fn from(env: fb::TypeEnvironment) -> Option<Environment> {
        let env = env.assignments()?;
        let mut types = PolyTypeMap::new();
        for value in env.iter() {
            let assignment: Option<(String, PolyType)> = value.into();
            let (id, ty) = assignment?;
            types.insert(id, ty);
        }
        Some(Environment::from(types))
    }
}

impl From<fb::TypeAssignment<'_>> for Option<(String, PolyType)> {
    fn from(a: fb::TypeAssignment) -> Option<(String, PolyType)> {
        let ty: Option<PolyType> = a.ty()?.into();
        Some((a.id()?.into(), ty?))
    }
}

/// Decodes a PolyType from a flatbuffer
impl From<fb::PolyType<'_>> for Option<PolyType> {
    fn from(t: fb::PolyType) -> Option<PolyType> {
        let v = t.vars()?;
        let mut vars = Vec::new();
        for value in v.iter() {
            vars.push(value.into());
        }
        let c = t.cons()?;
        let mut cons = TvarKinds::new();
        for value in c.iter() {
            let constraint: Option<(Tvar, Kind)> = value.into();
            let (tv, kind) = constraint?;
            cons.entry(tv).or_insert_with(Vec::new).push(kind);
        }
        Some(PolyType {
            vars,
            cons,
            expr: from_table(t.expr()?, t.expr_type())?,
        })
    }
}

impl From<fb::Constraint<'_>> for Option<(Tvar, Kind)> {
    fn from(c: fb::Constraint) -> Option<(Tvar, Kind)> {
        Some((c.tvar()?.into(), c.kind().into()))
    }
}

impl From<fb::Kind> for Kind {
    fn from(kind: fb::Kind) -> Kind {
        match kind {
            fb::Kind::Addable => Kind::Addable,
            fb::Kind::Subtractable => Kind::Subtractable,
            fb::Kind::Divisible => Kind::Divisible,
            fb::Kind::Numeric => Kind::Numeric,
            fb::Kind::Comparable => Kind::Comparable,
            fb::Kind::Equatable => Kind::Equatable,
            fb::Kind::Nullable => Kind::Nullable,
            fb::Kind::Row => Kind::Row,
            fb::Kind::Negatable => Kind::Negatable,
        }
    }
}

impl From<Kind> for fb::Kind {
    fn from(kind: Kind) -> fb::Kind {
        match kind {
            Kind::Addable => fb::Kind::Addable,
            Kind::Subtractable => fb::Kind::Subtractable,
            Kind::Divisible => fb::Kind::Divisible,
            Kind::Numeric => fb::Kind::Numeric,
            Kind::Comparable => fb::Kind::Comparable,
            Kind::Equatable => fb::Kind::Equatable,
            Kind::Nullable => fb::Kind::Nullable,
            Kind::Row => fb::Kind::Row,
            Kind::Negatable => fb::Kind::Negatable,
        }
    }
}

fn from_table(table: flatbuffers::Table, t: fb::MonoType) -> Option<MonoType> {
    match t {
        fb::MonoType::Basic => {
            let basic = fb::Basic::init_from_table(table);
            Some(basic.into())
        }
        fb::MonoType::Var => {
            let var = fb::Var::init_from_table(table);
            Some(MonoType::Var(Tvar::from(var)))
        }
        fb::MonoType::Arr => {
            let opt: Option<Array> = fb::Arr::init_from_table(table).into();
            Some(MonoType::Arr(Box::new(opt?)))
        }
        fb::MonoType::Fun => {
            let opt: Option<Function> = fb::Fun::init_from_table(table).into();
            Some(MonoType::Fun(Box::new(opt?)))
        }
        fb::MonoType::Row => fb::Row::init_from_table(table).into(),
        fb::MonoType::NONE => None,
    }
}

impl From<fb::Basic<'_>> for MonoType {
    fn from(t: fb::Basic) -> MonoType {
        match t.t() {
            fb::Type::Bool => MonoType::Bool,
            fb::Type::Int => MonoType::Int,
            fb::Type::Uint => MonoType::Uint,
            fb::Type::Float => MonoType::Float,
            fb::Type::String => MonoType::String,
            fb::Type::Duration => MonoType::Duration,
            fb::Type::Time => MonoType::Time,
            fb::Type::Regexp => MonoType::Regexp,
            fb::Type::Bytes => MonoType::Bytes,
        }
    }
}

impl From<fb::Var<'_>> for Tvar {
    fn from(t: fb::Var) -> Tvar {
        Tvar(t.i())
    }
}

impl From<fb::Arr<'_>> for Option<Array> {
    fn from(t: fb::Arr) -> Option<Array> {
        Some(Array(from_table(t.t()?, t.t_type())?))
    }
}

impl From<fb::Row<'_>> for Option<MonoType> {
    fn from(t: fb::Row) -> Option<MonoType> {
        let mut r = match t.extends() {
            None => MonoType::Obj(Box::new(Record::Empty { loc: None })),
            Some(tv) => MonoType::Var(tv.into()),
        };
        let p = t.props()?;
        for value in p.iter().rev() {
            match value.into() {
                None => {
                    return None;
                }
                Some(Property { k, v }) => {
                    r = MonoType::Obj(Box::new(Record::Extension {
                        loc: None,
                        lab: k,
                        typ: v,
                        ext: r,
                    }));
                }
            }
        }
        Some(r)
    }
}

impl From<fb::Prop<'_>> for Option<Property> {
    fn from(t: fb::Prop) -> Option<Property> {
        Some(Property {
            k: t.k()?.to_owned(),
            v: from_table(t.v()?, t.v_type())?,
        })
    }
}

impl From<fb::Fun<'_>> for Option<Function> {
    fn from(t: fb::Fun) -> Option<Function> {
        let args = t.args()?;
        let mut r = MonoType::Par(Box::new(Parameter::None { loc: None }));
        for value in args.iter().rev() {
            match value.into() {
                None => {
                    return None;
                }
                Some((k, v, true, _)) => {
                    r = MonoType::Par(Box::new(Parameter::Pipe {
                        loc: None,
                        lab: if k == "<-" { None } else { Some(k) },
                        typ: v,
                        ext: r,
                    }));
                }
                Some((name, t, _, true)) => {
                    r = MonoType::Par(Box::new(Parameter::Opt {
                        loc: None,
                        lab: name,
                        typ: t,
                        ext: r,
                    }));
                }
                Some((name, t, false, false)) => {
                    r = MonoType::Par(Box::new(Parameter::Req {
                        loc: None,
                        lab: name,
                        typ: t,
                        ext: r,
                    }));
                }
            };
        }
        Some(Function {
            x: r,
            e: from_table(t.retn()?, t.retn_type())?,
        })
    }
}

impl From<fb::Argument<'_>> for Option<(String, MonoType, bool, bool)> {
    fn from(t: fb::Argument) -> Option<(String, MonoType, bool, bool)> {
        Some((
            t.name()?.to_owned(),
            from_table(t.t()?, t.t_type())?,
            t.pipe(),
            t.optional(),
        ))
    }
}

pub fn serialize<'a, 'b, T, S, F>(
    builder: &'a mut flatbuffers::FlatBufferBuilder<'b>,
    t: T,
    f: F,
) -> &'a [u8]
where
    F: Fn(&mut flatbuffers::FlatBufferBuilder<'b>, T) -> flatbuffers::WIPOffset<S>,
{
    let offset = f(builder, t);
    builder.finish(offset, None);
    builder.finished_data()
}

pub fn deserialize<'a, T: 'a, S>(buf: &'a [u8]) -> S
where
    T: flatbuffers::Follow<'a>,
    S: std::convert::From<T::Inner>,
{
    flatbuffers::get_root::<T>(buf).into()
}

fn build_vec<T, S, F, B>(v: Vec<T>, b: &mut B, f: F) -> Vec<S>
where
    F: Fn(&mut B, T) -> S,
{
    let mut mapped = Vec::new();
    for t in v {
        mapped.push(f(b, t));
    }
    mapped
}

pub fn build_fresher<'a>(
    builder: &mut flatbuffers::FlatBufferBuilder<'a>,
    f: Fresher,
) -> flatbuffers::WIPOffset<fb::Fresher<'a>> {
    fb::Fresher::create(builder, &fb::FresherArgs { u: f.0 })
}

pub fn build_env<'a>(
    builder: &mut flatbuffers::FlatBufferBuilder<'a>,
    env: Environment,
) -> flatbuffers::WIPOffset<fb::TypeEnvironment<'a>> {
    let assignments = build_vec(
        env.values.into_iter().collect(),
        builder,
        build_type_assignment,
    );
    let assignments = builder.create_vector(assignments.as_slice());
    fb::TypeEnvironment::create(
        builder,
        &fb::TypeEnvironmentArgs {
            assignments: Some(assignments),
        },
    )
}

fn build_type_assignment<'a>(
    builder: &mut flatbuffers::FlatBufferBuilder<'a>,
    assignment: (String, PolyType),
) -> flatbuffers::WIPOffset<fb::TypeAssignment<'a>> {
    let id = builder.create_string(&assignment.0);
    let ty = build_polytype(builder, assignment.1);
    fb::TypeAssignment::create(
        builder,
        &fb::TypeAssignmentArgs {
            id: Some(id),
            ty: Some(ty),
        },
    )
}

/// Encodes a polytype as a flatbuffer
pub fn build_polytype<'a>(
    builder: &mut flatbuffers::FlatBufferBuilder<'a>,
    t: PolyType,
) -> flatbuffers::WIPOffset<fb::PolyType<'a>> {
    let vars = build_vec(t.vars, builder, build_var);
    let vars = builder.create_vector(vars.as_slice());

    let mut cons = Vec::new();
    for (tv, kinds) in t.cons {
        for k in kinds {
            cons.push((tv, k));
        }
    }
    let cons = build_vec(cons, builder, build_constraint);
    let cons = builder.create_vector(cons.as_slice());

    let (buf_offset, expr) = build_type(builder, t.expr);
    fb::PolyType::create(
        builder,
        &fb::PolyTypeArgs {
            vars: Some(vars),
            cons: Some(cons),
            expr_type: expr,
            expr: Some(buf_offset),
        },
    )
}

fn build_constraint<'a>(
    builder: &mut flatbuffers::FlatBufferBuilder<'a>,
    constraint: (Tvar, Kind),
) -> flatbuffers::WIPOffset<fb::Constraint<'a>> {
    let tvar = build_var(builder, constraint.0);
    fb::Constraint::create(
        builder,
        &fb::ConstraintArgs {
            tvar: Some(tvar),
            kind: constraint.1.into(),
        },
    )
}

pub fn build_type<'a>(
    builder: &mut flatbuffers::FlatBufferBuilder<'a>,
    t: MonoType,
) -> (
    flatbuffers::WIPOffset<flatbuffers::UnionWIPOffset>,
    fb::MonoType,
) {
    match t {
        MonoType::Bool => {
            let a = fb::BasicArgs { t: fb::Type::Bool };
            let v = fb::Basic::create(builder, &a);
            (v.as_union_value(), fb::MonoType::Basic)
        }
        MonoType::Int => {
            let a = fb::BasicArgs { t: fb::Type::Int };
            let v = fb::Basic::create(builder, &a);
            (v.as_union_value(), fb::MonoType::Basic)
        }
        MonoType::Uint => {
            let a = fb::BasicArgs { t: fb::Type::Uint };
            let v = fb::Basic::create(builder, &a);
            (v.as_union_value(), fb::MonoType::Basic)
        }
        MonoType::Float => {
            let a = fb::BasicArgs { t: fb::Type::Float };
            let v = fb::Basic::create(builder, &a);
            (v.as_union_value(), fb::MonoType::Basic)
        }
        MonoType::String => {
            let a = fb::BasicArgs {
                t: fb::Type::String,
            };
            let v = fb::Basic::create(builder, &a);
            (v.as_union_value(), fb::MonoType::Basic)
        }
        MonoType::Duration => {
            let a = fb::BasicArgs {
                t: fb::Type::Duration,
            };
            let v = fb::Basic::create(builder, &a);
            (v.as_union_value(), fb::MonoType::Basic)
        }
        MonoType::Time => {
            let a = fb::BasicArgs { t: fb::Type::Time };
            let v = fb::Basic::create(builder, &a);
            (v.as_union_value(), fb::MonoType::Basic)
        }
        MonoType::Regexp => {
            let a = fb::BasicArgs {
                t: fb::Type::Regexp,
            };
            let v = fb::Basic::create(builder, &a);
            (v.as_union_value(), fb::MonoType::Basic)
        }
        MonoType::Bytes => {
            let a = fb::BasicArgs { t: fb::Type::Bytes };
            let v = fb::Basic::create(builder, &a);
            (v.as_union_value(), fb::MonoType::Basic)
        }
        MonoType::Var(tvr) => {
            let offset = build_var(builder, tvr);
            (offset.as_union_value(), fb::MonoType::Var)
        }
        MonoType::Arr(arr) => {
            let offset = build_arr(builder, *arr);
            (offset.as_union_value(), fb::MonoType::Arr)
        }
        MonoType::Obj(obj) => {
            let offset = build_record(builder, *obj, Vec::new());
            (offset.as_union_value(), fb::MonoType::Row)
        }
        MonoType::Fun(fun) => {
            let offset = build_fun(builder, fun.x, fun.e, Vec::new());
            (offset.as_union_value(), fb::MonoType::Fun)
        }
        MonoType::Par(_) => panic!("invalid type"),
    }
}

fn build_var<'a>(
    builder: &mut flatbuffers::FlatBufferBuilder<'a>,
    var: Tvar,
) -> flatbuffers::WIPOffset<fb::Var<'a>> {
    fb::Var::create(builder, &fb::VarArgs { i: var.0 })
}

fn build_arr<'a>(
    builder: &mut flatbuffers::FlatBufferBuilder<'a>,
    mut arr: Array,
) -> flatbuffers::WIPOffset<fb::Arr<'a>> {
    let (off, typ) = build_type(builder, arr.0);
    fb::Arr::create(
        builder,
        &fb::ArrArgs {
            t_type: typ,
            t: Some(off),
        },
    )
}

fn build_record<'a>(
    builder: &mut flatbuffers::FlatBufferBuilder<'a>,
    mut r: Record,
    mut props: Vec<Property>,
) -> flatbuffers::WIPOffset<fb::Row<'a>> {
    match r {
        Record::Empty { .. } => {
            let props = build_vec(props, builder, build_prop);
            let props = builder.create_vector(props.as_slice());
            fb::Row::create(
                builder,
                &fb::RowArgs {
                    props: Some(props),
                    extends: None,
                },
            )
        }
        Record::Extension {
            loc: _,
            lab: a,
            typ: t,
            ext: MonoType::Var(tv),
        } => {
            props.push(Property { k: a, v: t });
            let props = build_vec(props, builder, build_prop);
            let props = builder.create_vector(props.as_slice());
            let r = Some(build_var(builder, tv));
            fb::Row::create(
                builder,
                &fb::RowArgs {
                    props: Some(props),
                    extends: r,
                },
            )
        }
        Record::Extension {
            loc: _,
            lab: a,
            typ: t,
            ext: MonoType::Obj(r),
        } => {
            props.push(Property { k: a, v: t });
            build_record(builder, *r, props)
        }
        Record::Extension {
            loc: _,
            lab: a,
            typ: t,
            ext: r,
        } => build_record(builder, Record::Empty { loc: None }, props),
    }
}

fn build_fun<'a>(
    builder: &mut flatbuffers::FlatBufferBuilder<'a>,
    r: MonoType,
    e: MonoType,
    mut args: Vec<(String, MonoType, bool, bool)>,
) -> flatbuffers::WIPOffset<fb::Fun<'a>> {
    match r {
        MonoType::Par(r) => match *r {
            Parameter::None { .. } => {
                let args = build_vec(args, builder, build_arg);
                let args = builder.create_vector(args.as_slice());
                let (e, typ) = build_type(builder, e);
                fb::Fun::create(
                    builder,
                    &fb::FunArgs {
                        args: Some(args),
                        retn_type: typ,
                        retn: Some(e),
                    },
                )
            }
            Parameter::Req {
                loc: _,
                lab: a,
                typ: t,
                ext: r,
            } => {
                args.push((a, t, false, false));
                build_fun(builder, r, e, args)
            }
            Parameter::Opt {
                loc: _,
                lab: a,
                typ: t,
                ext: r,
            } => {
                args.push((a, t, false, true));
                build_fun(builder, r, e, args)
            }
            Parameter::Pipe {
                loc: _,
                lab: None,
                typ: t,
                ext: r,
            } => {
                args.push((String::from("<-"), t, true, false));
                build_fun(builder, r, e, args)
            }
            Parameter::Pipe {
                loc: _,
                lab: Some(a),
                typ: t,
                ext: r,
            } => {
                args.push((a, t, true, false));
                build_fun(builder, r, e, args)
            }
        },
        _ => build_fun(
            builder,
            MonoType::Par(Box::new(Parameter::None { loc: None })),
            e,
            args,
        ),
    }
}

fn build_prop<'a>(
    builder: &mut flatbuffers::FlatBufferBuilder<'a>,
    prop: Property,
) -> flatbuffers::WIPOffset<fb::Prop<'a>> {
    let (off, typ) = build_type(builder, prop.v);
    let k = builder.create_string(&prop.k);
    fb::Prop::create(
        builder,
        &fb::PropArgs {
            k: Some(k),
            v_type: typ,
            v: Some(off),
        },
    )
}

fn build_arg<'a>(
    builder: &mut flatbuffers::FlatBufferBuilder<'a>,
    arg: (String, MonoType, bool, bool),
) -> flatbuffers::WIPOffset<fb::Argument<'a>> {
    let name = builder.create_string(&arg.0);
    let (buf_offset, typ) = build_type(builder, arg.1);
    fb::Argument::create(
        builder,
        &fb::ArgumentArgs {
            name: Some(name),
            t_type: typ,
            t: Some(buf_offset),
            pipe: arg.2,
            optional: arg.3,
        },
    )
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::semantic::parser;
    use crate::semantic::types::SemanticMap;

    #[rustfmt::skip]
    use crate::semantic::flatbuffers::semantic_generated::fbsemantic::{
        Expression,
        ExpressionStatement,
        ExpressionStatementArgs,
        File,
        FileArgs,
        FloatLiteral,
        FloatLiteralArgs,
        Operator,
        Package,
        PackageArgs,
        Statement,
        UnaryExpression,
        UnaryExpressionArgs,
        WrappedStatement,
        WrappedStatementArgs,
    };

    fn test_serde(expr: &'static str) {
        let want = parser::parse(expr).unwrap();
        let mut builder = flatbuffers::FlatBufferBuilder::new();
        let buf = serialize(&mut builder, want.clone(), build_polytype);
        let got = deserialize::<fb::PolyType, Option<PolyType>>(buf);
        assert_eq!(want, got.unwrap())
    }

    #[test]
    fn serde_type_environment() {
        let a = parser::parse("forall [] bool").unwrap();
        let b = parser::parse("forall [] time").unwrap();

        let want: Environment = semantic_map! {
            String::from("a") => a,
            String::from("b") => b,
        }
        .into();

        let mut builder = flatbuffers::FlatBufferBuilder::new();
        let buf = serialize(&mut builder, want.clone(), build_env);
        let got = deserialize::<fb::TypeEnvironment, Option<Environment>>(buf);

        assert_eq!(want, got.unwrap());
    }
    #[test]
    fn serde_basic_types() {
        test_serde("forall [] bool");
        test_serde("forall [] int");
        test_serde("forall [] uint");
        test_serde("forall [] float");
        test_serde("forall [] string");
        test_serde("forall [] duration");
        test_serde("forall [] time");
        test_serde("forall [] regexp");
        test_serde("forall [] bytes");
    }
    #[test]
    fn serde_array_type() {
        test_serde("forall [t0] [t0]");
    }
    #[test]
    fn serde_function_types() {
        test_serde("forall [t0] (<-tables: [t0], ?flag: bool, fn: (r: t0) -> bool) -> [t0]");
        test_serde("forall [t0, t1] where t0: Addable, t1: Divisible (a: t0, b: t1) -> bool");
    }
    #[test]
    fn serde_record_types() {
        test_serde(
            "forall [t0] {a: int | b: float | c: {d: string | d: string | d: time | d: {}} | t0}",
        );
    }
    #[test]
    fn test_flatbuffers_semantic() {
        let mut builder = flatbuffers::FlatBufferBuilder::new_with_capacity(256);

        // Testing out a unary expression using a float
        let floatval = FloatLiteral::create(
            &mut builder,
            &FloatLiteralArgs {
                value: 3.5,
                ..FloatLiteralArgs::default()
            },
        );

        let increment = UnaryExpression::create(
            &mut builder,
            &UnaryExpressionArgs {
                operator: Operator::SubtractionOperator,
                argument: Some(floatval.as_union_value()),
                ..UnaryExpressionArgs::default()
            },
        );

        let statement = ExpressionStatement::create(
            &mut builder,
            &ExpressionStatementArgs {
                expression_type: Expression::UnaryExpression,
                expression: Some(increment.as_union_value()),
                ..ExpressionStatementArgs::default()
            },
        );

        let wrappedStatement = WrappedStatement::create(
            &mut builder,
            &WrappedStatementArgs {
                statement_type: Statement::ExpressionStatement,
                statement: Some(statement.as_union_value()),
            },
        );

        let statements = builder.create_vector(&[wrappedStatement]);

        let file = File::create(
            &mut builder,
            &FileArgs {
                body: Some(statements),
                ..FileArgs::default()
            },
        );

        let files = builder.create_vector(&[file]);

        let pkg = Package::create(
            &mut builder,
            &PackageArgs {
                files: Some(files),
                ..PackageArgs::default()
            },
        );

        builder.finish(pkg, None);
        let bytes = builder.finished_data();
        assert_ne!(bytes.len(), 0);
    }
}
