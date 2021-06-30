// Flux eperimental package includes experimental functions that perform
// various tasks.
package experimental

// addDuration is a function that adds a duration to a time value and returns
//  the resulting time value.
//
//  addDuration function is subject to change at any time. By using this funciton,
//  you accept the risks of experimental functions.
//
//  This function will be removed once duration vectors are implementated.
//
// ## Parameters
// - `d` is the duration to add.
// - `to` is the time to add the duration to.
//
// ## Example
//
// ```
// import "experimental"
//
// experimental.addDuration(
//   d: 12h,
//   to: now(),
// )
// ```
//
// ## Add six hours to a timestamp
//
// ```
// import "experimental"
//
// experimental.addDuration(
//   d: 6h,
//   to: 2019-09-16T12:00:00Z,
// )
// // Returns 2019-09-16T18:00:00.000000000Z
// ```
builtin addDuration : (d: duration, to: time) => time

// subDuration is a function that subtracts a duration from a time value
//  and returns the resulting time value.
//
//  subDuration function is subject to change at any time. By using this
//  function, you accept the risks of experimental functions.
//
//  This funciton will be removed once duration vectors are impemented.
//
// ## Parameters
// - `d` is the duration to subtract.
// - `from` is the time to subtract the duration from.
//
// ## Example
//
// ```
// import "experimental"
//
// experimental.subDuration(
//   d: 12h,
//   from: now(),
// )
// ```
//
// ## Subtract six hours from a timestamp
//
// ```
//import "experimental"
//
// experimental.subDuration(
//   d: 6h,
//   from: 2019-09-16T12:00:00Z,
// )
// // Returns 2019-09-16T06:00:00.000000000Z
// ```
builtin subDuration : (d: duration, from: time) => time

// group is a function that introduces an extended mode to the existing group()
//  funciton.
//
//  group function is subject to change at any time. By using this function,
//  you accept the risks of experimental functions.
//
//  This function will be removed once the proposed extend mode is sufficiently
//  vetted.
//
// ## Parameters
// - `mode` is the mode used to group columns.
//
//   Appends columns defined in the columns parameter to all existing group keys.
//   extend is the only mode available to experimental.group()
//
// - `columns` is a list of columns to use in the grouping operation.
//
//   Defaults to [].
//
// ## Include the value column in each group's group key
//
// ```
//import "experimental"
//
// from(bucket: "example-bucket")
//   |> range(start: -1m)
//   |> experimental.group(columns: ["_value"], mode: "extend")
// ```
builtin group : (<-tables: [A], mode: string, columns: [string]) => [A] where A: Record

// objectKeys is a function that returns an array of keys in a
//  specified record.
//
//  objectKeys function is subject to change at any time. By using
//  this function, you accept the risks of experimental functions.
//
// ## Parameters
// - `o` is the record to return keys from.
//
// ## Return all keys in a record
//
// ```
//import "experimental"
//
// user = {
//   firstName: "John",
//   lastName: "Doe",
//   age: 42
// }
//
// experimental.objectKeys(o: user)
// // Returns [firstName, lastName, age]
// ```
builtin objectKeys : (o: A) => [string] where A: Record

// set is a function that sets multiple static column values on all records.
//
//  If a column already exists, the function updates the existing value. If
//  a column does not exist, the function adds it with the specified value.
//
//  set function is subject to change at any time. By using this function,
//  you accept the risks of experimental functions.
//
//  Once sufficiently vetted, experimental.set() will replace the existing
//  set() function.
//
// ## Parameters
// - `o` is a record that defines the columns and values to set.
//
//   The key of each key-value pair defines the column name. The value of each
//   key-value pair defines the column value.
//
// ## Set values for multiple columns
//
// ```
// import "experimental"
//
// data
//   |> experimental.set(
//     o: {
//       _field: "temperature",
//       unit: "°F",
//       location: "San Francisco"
//     }
//   )
// ```
//
// # Example input table
// _time | _field | _value
// --- | --- | ---
// 2019-09-16T12:00:00Z | temp | 71.2
// 2019-09-17T12:00:00Z	| temp | 68.4
// 2019-09-18T12:00:00Z | temp | 70.8
//
// # Example output table
// _time | _field | _value | unit | location
// 2019-09-16T12:00:00Z	| temperature | 71.2 | °F | San Fransisco
// 2019-09-17T12:00:00Z | temperature | 68.4 | °F | San Fransisco
// 2019-09-18T12:00:00Z | temperature | 70.8 | °F | San Fransisco
//
builtin set : (<-tables: [A], o: B) => [C] where A: Record, B: Record, C: Record

// to is a function that writes data to an InfluxDB v2.0 bucket, but in a
//  different structure than the built-in to() function.
//
//  to function is subject to change at any time. By using this function,
//  you accept the risks of experimental functions.
//
// ## Parameters
// - `bucket` is the bucket to write data to.
//
//   bucket and bucketID are mutually exclusive.
//
// - `bucketID` is the ID of the bucket to write the data to.
//
// - `org` is the organization name of the specified bucket.
//
//   Only required when writing to a different organization or a remote
//   host. org and orgID are mutually exclusive.
//
// - `orgID` is the organization ID of the specified bucket.
//
//   Only required when writing to a different organization or a remote
//   host. orgID and org are mutually exclusive.
//
// ## Expected data structure
// # Data structure expected by built-in to()
//  The built-in to() function requires _time, _measurement, _field, and
//  _value columns. The _field column stores the field key and the _value
//  column stores the field value.
//
//  _time | _measurement | _field | _value
//  --- | --- | --- | ---
//  timestamp | measurement-name | field key | field value
//
// # Data structre expected by experimental to()
//  experimental.to() requires _time and _measurement columns, but the field
//  keys and values are stored in single columns with the field key as the
//  column name and the field value as the column value.
//
//  _time | _measurement | _field_key
//  --- | --- | ---
//  timestamp | measurement-name | field value
//
// if using the built-in from() function, use the pivot() to transform data
// into the structure experimental.to() expects.
//
// ## Example
//
// ```
// import "experimental"
//
// experimental.to(
//   bucket: "my-bucket",
//   org: "my-org"
// )
//
// // OR
//
// experimental.to(
//   bucketID: "1234567890",
//   orgID: "0987654321"
// )
// ```
//
// ## Use pivot() to shape the data for experimental.to()
//
// ```
// import "experimental"
//
// from(bucket: "example-bucket")
//   |> range(start: -1h)
//   |> pivot(
//       rowKey:["_time"],
//       columnKey: ["_field"],
//       valueColumn: "_value")
//   |> experimental.to(
//       bucket: "bucket-name",
//       org: "org-name"
//   )
// ```
builtin to : (
    <-tables: [A],
    ?bucket: string,
    ?bucketID: string,
    ?org: string,
    ?orgID: string,
    ?host: string,
    ?token: string,
) => [A] where
    A: Record

// An experimental version of join.
builtin join : (left: [A], right: [B], fn: (left: A, right: B) => C) => [C] where A: Record, B: Record, C: Record
builtin chain : (first: [A], second: [B]) => [B] where A: Record, B: Record

// alignTime is a function that aligns input tables to a common start time.
//
//  alignTime function is subject to change at any time. By using this
//  function, you accept the risks of experimental functions.
//
// ## Parameters
// - `alignTo` is the UTC time to align tables to.
//
//   Defaults to 1970-01-01T00:00:00Z.
//
// ## Example
//
// ```
// import "experimental"
//
// experimental.alignTime(
//   alignTo: 1970-01-01T00:00:00.000000000Z
// )
// ```
//
// ## Compare values month-over-month
//
// ```
// import "experimental"
//
// from(bucket: "example-bucket")
//   |> range(start: -12mo)
//   |> filter(fn: (r) => r._measurement == "example-measurement")
//   |> window(every: 1mo)
//   |> experimental.alignTime()
// ```
//
// # Given the following input
//
// _time | _value
// --- | ---
// 2020-01-01T00:00:00Z | 32.1
// 2020-01-02T00:00:00Z | 32.9
// 2020-01-03T00:00:00Z | 33.2
// 2020-01-04T00:00:00Z | 34.0
// 2020-02-01T00:00:00Z | 38.3
// 2020-02-02T00:00:00Z | 38.4
// 2020-02-03T00:00:00Z | 37.8
// 2020-02-04T00:00:00Z | 37.5
//
// # The following functions
//  1. Window data by calendar month creating two separate tables.
//  2. Align tables to 2020-01-01T00:00:00Z.
//
// # Output
// _time | _value
// --- | ---
// 2020-01-01T00:00:00Z | 32.1
// 2020-01-02T00:00:00Z | 32.9
// 2020-01-03T00:00:00Z | 33.2
// 2020-01-04T00:00:00Z | 34.0
//
// _time | _value
// --- | ---
// 2020-01-01T00:00:00Z | 38.3
// 2020-01-02T00:00:00Z | 38.4
// 2020-01-03T00:00:00Z | 37.8
// 2020-01-04T00:00:00Z | 37.5
//
// Each output table represents data from a calendr month. When visualized,
// data is still grouped by month, but timestamps are aligned to a common start
// time and values can be compared by time.
//
alignTime = (tables=<-, alignTo=time(v: 0)) => tables
    |> stateDuration(
        fn: (r) => true,
        column: "timeDiff",
        unit: 1ns,
    )
    |> map(fn: (r) => ({r with _time: time(v: int(v: alignTo) + r.timeDiff)}))
    |> drop(columns: ["timeDiff"])

// window is a function that groups records based on a time value.
//
//  New columns are added to uniquely identify each window. Those columns are
//  added to the group key of the output tables. Input tables must have _start
//  _stop, and _time columns. 
//
//  A single input record will be placed into zero or more output tables, depending
//  on the specific windowing function.
//
//  By default the start boundary of a window will align with the unix epoch
//  (zero time) modified by the offset of the location option.
//
//  window function is subject to change at any time. By using this function,
//  you accept the risks of experimental functions.
//
// ## Parameters
// - `every` is the duration of time between windows.
//
//   Defaults to period value.
//
// - `period` is the duration of the window.
//
//   Period is the length of each interval. It can be negative, indicating the
//   start and stop boundaries are reversed. Defaults to every value.
//
// - `offset` is the duration by which to shift the window boundaries.
//
//   It can be negative, indicating that the offset goes backwards in time.
//   Defaults to 0, which will align window end boundaries with the every
//   duration.
//
// - `createEmpty` specifies whether empty tables should be created.
//
//   Defaults to false.
//
// # Parameters
//  every, period, and offset support support all valid duration units,
//  including calendar months (1mo) and years (1y).
//
// ## Example
//
// ```
// window(
//   every: 5m,
//   period: 5m,
//   offset: 12h,
//   createEmpty: false
// )
// ```
//
// ## Window data into 10 minute intervals
//
// ```
// from(bucket:"example-bucket")
//   |> range(start: -12h)
//   |> window(every: 10m)
//   // ...
// ```
//
// ## Window by calendar month
//
// ```
// from(bucket:"example-bucket")
//   |> range(start: -1y)
//   |> window(every: 1mo)
//   // ...
// ```
builtin window : (
    <-tables: [{T with _start: time, _stop: time, _time: time}],
    ?every: duration,
    ?period: duration,
    ?offset: duration,
    ?createEmpty: bool,
) => [{T with _start: time, _stop: time, _time: time}]

// integral is a function that computes the area under the curve per unit of
//  time of subsequent non-null records.
//
//  The curve is defined using _time as the domain and record values as the
//  range. Input tables must have _time and _value columns.
//
//  integral function is subject to change at any time. By using this function,
//  you accept the risks of experimental functions.
//
// ## Parameters
// - `unit` is the time duration used to compute the integral.
// - `interpolate` is the type of interpolation to use.
//
//   Defaults to "".
//   Use one of the following interpolation options:
//    - empty string for no interpolation.
//    - linear.
//
// ## Calculate the integral
//
// ```
// from(bucket: "example-bucket")
//   |> range(start: -5m)
//   |> filter(fn: (r) =>
//     r._measurement == "cpu" and
//     r._field == "usage_system"
//   )
//   |> integral(unit:10s)
// ```
//
// ## Calculate the integral with linear interpolation
//
// ```
// from(bucket: "example-bucket")
//   |> range(start: -5m)
//   |> filter(fn: (r) =>
//     r._measurement == "cpu" and
//     r._field == "usage_system"
//   )
//   |> integral(unit:10s, interpolate: "linear")
// ```
builtin integral : (<-tables: [{T with _time: time, _value: B}], ?unit: duration, ?interpolate: string) => [{T with _value: B}]

// count is a function that outputs the number of records in each input table
//  and returns the count in the _value column.
//
//  This function counts both null and non-null records.
//
//  count function is subject to change at any time. By using this function,
//  you accept the risks of experimental functions.
//
// ## Empty tables
// experimental.count() returns 0 for empty tables. to keep empty tables in
// your data, set the following parameters for the following functions:
//
// Function | Parameters
// --- | ---
// filter() | onEmpty: "keep"
// window() | createEmpty: true
// aggregateWindow() | createEmpty: true
//
// ## Example
//
// ```
// import "experimental"
//
// from(bucket: "example-bucket")
//   |> range(start: -5m)
//   |> experimental.count()
// ```
builtin count : (<-tables: [{T with _value: A}]) => [{T with _value: int}]

// histogramQuantile is a function that approximates a quantile given a histogram
//  with the cumulative distribution of the dataset.
//
//  Each input table represents a single histogram. Input tables must have two
//  columns - a count column (_value) and an upper bound column (le), and neither
//  column can be part of the group key.
//
//  The count is the number of values that are less than or equal to the upper
//  bound value (le). Input tables can have an unlimited number of records; each
//  record represents an entry in the histogram. The count must be monotonically
//  increasing when sorted by upper bound (le). If any values in the _value or le
//  columns are null, the function returns an error.
//
//  Linear interpolation between the two closest bounds is used to compute the
//  quantile. If either of the bounds used in interpolation are infinite, then
//  the other finite bound is used and no interpolation is performed.
//
//  The output table has the same group key as the input table. The function returns
//  the value of the specified quantile from the histogram in the _value column and
//  drops all columns not part of the group key.
//
//  histogramQuantile function is subject to change at any time. By using this function,
//  you accept the risks of experimental functions.
//
// ## Parameters
// - `quantile` is a value between 0 and 1 indicating the desired quantile to compute.
// - `minValue` is the asumed minimum value of the dataset.
//
//   When the quantile falls below the lowest upper bound, interpolation is performed
//   between minValue and the lowest upper bound. When minValue is equal to negative
//   infinity, the lowest upper bound is used. Defaults to 0.0
//
// ## Compute the 90th quantile
//
// ```
// import "experimental"
//
// from(bucket: "example-bucket")
//   |> range(start: -1d)
//   |> filter(fn: (r) =>
//     r._meausrement == "example-measurement" and
//     r._field == "example-field"
//   )
//   |> experimental.histogramQuantile(quantile: 0.9)
// ```
builtin histogramQuantile : (<-tables: [{T with _value: float, le: float}], ?quantile: float, ?minValue: float) => [{T with _value: float}]

// An experimental version of mean.
builtin mean : (<-tables: [{T with _value: float}]) => [{T with _value: float}]

// An experimental version of mode.
builtin mode : (<-tables: [{T with _value: A}]) => [{T with _value: A}]

// An experimental version of quantile.
builtin quantile : (<-tables: [{T with _value: float}], q: float, ?compression: float, ?method: string) => [{T with _value: float}]

// An experimental version of skew.
builtin skew : (<-tables: [{T with _value: float}]) => [{T with _value: float}]

// An experimental version of spread.
builtin spread : (<-tables: [{T with _value: A}]) => [{T with _value: A}] where A: Numeric

// An experimental version of stddev.
builtin stddev : (<-tables: [{T with _value: float}], ?mode: string) => [{T with _value: float}]

// An experimental version of sum.
builtin sum : (<-tables: [{T with _value: A}]) => [{T with _value: A}] where A: Numeric

// An experimental version of kaufmansAMA.
builtin kaufmansAMA : (<-tables: [{T with _value: A}], n: int) => [{T with _value: float}] where A: Numeric

// An experimental version of distinct
builtin distinct : (<-tables: [{T with _value: A}]) => [{T with _value: A}]

// An experimental version of fill
builtin fill : (<-tables: [{T with _value: A}], ?value: A, ?usePrevious: bool) => [{T with _value: A}]

// An experimental version of first
builtin first : (<-tables: [{T with _value: A}]) => [{T with _value: A}]

// An experimental version of last
builtin last : (<-tables: [{T with _value: A}]) => [{T with _value: A}]

// An experimental version of max
builtin max : (<-tables: [{T with _value: A}]) => [{T with _value: A}]

// An experimental version of min
builtin min : (<-tables: [{T with _value: A}]) => [{T with _value: A}]

// An experimental version of unique
builtin unique : (<-tables: [{T with _value: A}]) => [{T with _value: A}]

// An experimental version of histogram
builtin histogram : (<-tables: [{T with _value: float}], bins: [float], ?normalize: bool) => [{T with _value: float, le: float}]
