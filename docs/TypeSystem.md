# Flux's Type System

A type system provides a framework under which all operations within Flux are described.

## Data and Types

All data that Flux can represent must be described via the type system.
If the type system cannot describe a data set then that data set cannot be processed in Flux unless it is first transformed into a data set that Flux can describe.

As such it is important that Flux's type system can describe a wide variety of data sets.

Postel's law, be conservative in what you do and be liberal in what you accept.

For Flux this means that the type system can be used to describe wide variety of data sets.



Flux is a gradual typed language, meaning that it is both dynamically and statically typed.
When the types of all expressions and values are known then Flux can compile type safe code, a special "dynamic" type is used to describe a value or expression who's type is not known until runtime.

By using gradual types a user can describe their data loosely at first when the types are not necessarily known.
Then as the users provides details on how to clean up the data the types become known and then the rest of the code behaves as a statically typed language.


Flux builds complex types by composing base and structural types.

Base types

* Bool
* Int
* Float
* String

Flux's type system is structural meaning that composite types are defined by their parts.

Structural types

* Array - list of elements all with the same type
* Object - key value pairs where keys are always strings
* Function - Single object argument with return type



Type base and structural types can be composed further to define specific types:

For example:

```

type record = { ; any}

type table = {
    records: [...]record,
}

// function to get table schema from its type
// Maybe just use first class reflection for this?
schema = (table: table) -> []columnMeta


type channel = {
    tables: [...]table,
}

type stream = {
    data: channel,
    meta: channel,
}


type errorTable = table<{error: string; any}>

module tables<r>

type errorChannel
type errorStream

errors : (tables: stream) -> errorStream

type table <r> = {
    record: [...]r
}

type channel <r> = {
    tables: [...]table<r>
}

type stream <r> = {
    data: channel <r>
    meta: channel <r>
}

predicate = (r) => r._measurment == "cpu" and r._field == "usage_idle"

r : { _measurment : string, _field : string ; any}

filter = (stream) =>
    stream.data |> map(fn: (t) => 
        t.records |> _filter(fn: predicate)
    )


_filter : (fn: ('a) -> bool, iter: [...]'a) -> [...]'a








```
