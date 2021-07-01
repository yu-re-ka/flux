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

// mean is a function that computes the mean or average of non-null values
//  in the _value column of each input table.
//
//  Output tables contain a single row with the calculated mean in the
//  _value column.
//
//  mean function is subject to change at any time. By using this function,
//  you accept the risks of experimental functions.
//
// ## Example
//
// ```
// import "experimental"
//
// from(bucket:"example-bucket")
//   |> filter(fn: (r) =>
//     r._measurement == "example-measurement" and
//     r._field == "example-field")
//   |> range(start:-1h)
//   |> experimental.mean()
// ```
builtin mean : (<-tables: [{T with _value: float}]) => [{T with _value: float}]

// mode is a function that computes the mode or value that occurs most often
//  in the _value column in each input table.
//
//  If there are multiple modes, it returns all of them in a sorted table. mode
//  only considers non-null values. If there is no mode, experimental.mode()
//  returns null.
//
//  mode function is subject to change at any time. By using this function, you
//  accept the risks of experimental functions.
//
// ## Supported data types
// - String
// - Float
// - Integer
// - UInteger
// - Boolean
// - Time
//
// ## Empty tables
//  experimental.mode() drops empty tables.
//
// ## Return the mode of windowed data
//
// ```
// import "experimental"
//
// from(bucket: "example-bucket")
//   |> filter(fn: (r) =>
//     r._measurement == "example-measurement" and
//     r._field == "example-field"
//   )
//   |> range(start:-12h)
//   |> window(every:10m)
//   |> experimental.mode()
// ```
builtin mode : (<-tables: [{T with _value: A}]) => [{T with _value: A}]

// quantile is a function that outputs non-null records with the values in the
//  _value column that fall within the specified quantile.
//
//  Which it returns depends on the method used. The _value column must contain
//  float values.
//
//  When using the estimate_tdigest or exact_mean methods, the function outputs
//  non-null records with values that fall within the specified quantile.
//
//  When using the exact_selector method, it outputs the non-null record with the
//  value that represents the specified quantile.
//
//  quantile function is subject to change at any time. By using this function,
//  you accept the risks of experimental funtions.
//
// ## Parameters
// - `q` is a value between 0 and 1 that specifies the quantile.
// - `method` is the computation method.
//
//   Default is estimate_tdigest.
//   Available options are:
//   - estimate_tdigest
//   - exact_mean
//   - exact_selector
//
// # estimate_tdigest
//  An aggregate method that uses a t-digest data structure to compute an accurate
//  quantile estimate on a large data source.
//
// # exact_mean
//  A aggregate method that takes the averge of the two points closest to the
//  quantile value.
//
// # exact_selector
//  A selector method that returns the data point for which at least q points are
//  less than.
//
// ## compression
// Indicates how many centroids to use when compressing the dataset. A larger number
// produces a more accurate result at the cost of increased memory requirements.
// Defaults to 1000.0.
//
// ## Quantile as an aggregate
//
// ```
// import "experimental"
//
// from(bucket: "example-bucket")
// 	|> range(start: -5m)
// 	|> filter(fn: (r) =>
//     r._measurement == "example-measurement" and
//     r._field == "example-field")
// 	|> experimental.quantile(
//     q: 0.99,
//     method: "estimate_tdigest",
//     compression: 1000.0
//   )
// ```
//
// ## Quantile as a selector
//
// ```
// import "experimental"
//
// from(bucket: "example-bucket")
// 	|> range(start: -5m)
// 	|> filter(fn: (r) =>
//     r._measurement == "example-measurement" and
//     r._field == "example-field")
// 	|> experimental.quantile(
//     q: 0.99,
//     method: "exact_selector"
//   )
// ```
builtin quantile : (<-tables: [{T with _value: float}], q: float, ?compression: float, ?method: string) => [{T with _value: float}]

// skew is a function that outputs the skew of non-null values in the
//  _value column for each input table as a float.
//
//  skew function is subject to change at any time. By using this
//  function, you accept the risks of experimental functions.
//
// ## Example
//
// ```
// import "experimental"
//
// from(bucket: "example-bucket")
//   |> range(start: -5m)
//   |> filter(fn: (r) =>
//     r._measurement == "example-measurement" and
//     r._field == "example-field"
//   )
//   |> experimental.skew()
// ```
builtin skew : (<-tables: [{T with _value: float}]) => [{T with _value: float}]

// spread is a function that outputs the difference between the minimum and
//  maximum values in the value column for each input table.
//
//  The function supports uint, int, and float values. The output value type
//  depends on the input value type.
//  - uint or int values return int values.
//  - float input values return float output values.
//
//  spread function is subject to change at any time. By using this function,
//  you accept the risks of experimental functions.
//
// ## Example
//
// ```
// import "experimental"
//
// from(bucket: "example-bucket")
//   |> range(start: -5m)
//   |> filter(fn: (r) =>
//     r._measurement == "example-measurement" and
//     r._field == "example-field"
//   )
//   |> experimental.spread()
// ```
builtin spread : (<-tables: [{T with _value: A}]) => [{T with _value: A}] where A: Numeric

// stddev is a function that computes the standard deviation of non-null values
//  in the _value column for each input table.
//
//  stddev function is subject to change at any time. By using this function,
//  you accept the risks of experimental functions.
//
// ## Parameters
// - `mode` is the standard deviation mode or type of standard deviation to calculate.
//
//   Defaults to "sample".
//   Available options are:
//   - sample
//   - population
//
// # sample
//  Calculate the sample standard deviation where the data is considered to be part of
//  a larger population.
//
// # population
//  Calculate the population standard deviation where the data is considered a
//  population of its own.
//
// ## Example
//
// ```
// import "experimental"
//
// from(bucket: "example-bucket")
//   |> range(start: -5m)
//   |> filter(fn: (r) =>
//     r._measurement == "cpu" and
//     r._field == "usage_system"
//   )
//   |> experimental.stddev()
// ```
builtin stddev : (<-tables: [{T with _value: float}], ?mode: string) => [{T with _value: float}]

// sum is a function that computes the sum of non-null values in the _value
//  column for each input table.
//
//  sum function is subject to change at any time. By using this function,
//  you accept the risks of experimental functions.
//
// ## Example
//
// ```
// import "experimental"
//
// from(bucket: "example-bucket")
//   |> range(start: -5m)
//   |> filter(fn: (r) =>
//     r._measurement == "example-measurement" and
//     r._field == "example-field"
//   )
//   |> experimental.sum()
// ```
builtin sum : (<-tables: [{T with _value: A}]) => [{T with _value: A}] where A: Numeric

// kaufmansAMA is a function that calculate the Kaufman's Adaptive Moving
//  Average (KAMA) of input tables using the _value column in each table.
//
//  Kaufman’s Adaptive Moving Average is a trend-following indicator designed
//  to account for market noise or volatility.
//
//  kaufmansAMA function is subject to change at any time. By using this function,
//  you accept the risks of experimental functions.
//
// ## Parameters
// - `n` is the period or number of points to use in the calculation.
//
// ## Example
//
// ```
// import "experimental"
//
// experimental.from(bucket: "example-bucket"):
//   |> range(start: -7d)
//   |> kaufmansAMA(n: 10)
// ```
builtin kaufmansAMA : (<-tables: [{T with _value: A}], n: int) => [{T with _value: float}] where A: Numeric

// distinct is a function that returns unique values from the _value column.
//
//  The _value of each output record is set to a distinct value in the specified
//  column. null is considered a distinct value.
//
//  distinct function is subject to change at any time. By using this function,
//  you accept the risks of experimental functions.
//
// ## Output schema
// experimental.distinct() outputs a single table for each input table and
// does the following:
//
// - Outputs a single record for each distinct value.
// - Drops all columns not in the group key.
//
// ## Empty tables
// experimental.distinct() drops empty tables.
//
// ## Parameters
// - `tables` is the input data
//
//   Default is pipe-forwarded data.
//
// ## Example
//
// ```
// import "experimental"
//
// data
// 	|> experimental.distinct()
// ```
//
// # Input data
//  _time | _field | _value
//  --- | --- | ---
// 2021-01-01T00:00:00Z | ver | v1
// 2021-01-01T00:01:00Z | ver | v1
// 2021-01-01T00:02:00Z | ver | v2
// 2021-01-01T00:03:00Z | ver | 
// 2021-01-01T00:04:00Z | ver | v3
// 2021-01-01T00:05:00Z | ver | v3
//
// # Output table
//  _value
//  ---
//  v1
//  v2
//    
//  v3
//
builtin distinct : (<-tables: [{T with _value: A}]) => [{T with _value: A}]

// fill is a function that replaces all null values in the _value column
//  with a non-null value.
//
//  fill function is subject to change at any time. By using this function,
//  you accept the risks of experimental functions.
//
// ## Parameters
// - `value` is the value to replace the null values with.
//
//   Data type must match the type of the _value column.
//
// - `usePrevious` when true, replaces null values with the value of the previous
//   non-null row.
//
// - `tables` is the input data.
//
//   Default is pipe-forwarded data.
//
// ## Fill null values with a specified non-null value
//
// ```
// import "experimental"
//
// data
//   |> experimental.fill(value: 0.0)
// ```
//
// # Input data
//  _time | _value
//  --- | ---
//  2021-01-01T00:00:00Z | 1.2
//  2021-01-01T00:01:00Z | 
//  2021-01-01T00:02:00Z | 2.3
//  2021-01-01T00:03:00Z | 
//  2021-01-01T00:04:00Z | 2.8
//  2021-01-01T00:05:00Z | 1.1
//
// # Output data
//  _time | _value
//  --- | ---
//  2021-01-01T00:00:00Z | 1.2
//  2021-01-01T00:01:00Z | 0.0
//  2021-01-01T00:02:00Z | 2.3
//  2021-01-01T00:03:00Z | 0.0
//  2021-01-01T00:04:00Z | 2.8
//  2021-01-01T00:05:00Z | 1.1
//
// ## Fill null values with the previous non-null value
//
// ```
// import "experimental"
//
// data
//   |> experimental.fill(usePrevious: true)
// ```
//
// # Input data
//  _time | _value
//  --- | ---
//  2021-01-01T00:00:00Z | 1.2
//  2021-01-01T00:01:00Z | 
//  2021-01-01T00:02:00Z | 2.3
//  2021-01-01T00:03:00Z | 
//  2021-01-01T00:04:00Z | 2.8
//  2021-01-01T00:05:00Z | 1.1
//
// # Output data
//  _time | _value
//  --- | ---
//  2021-01-01T00:00:00Z | 1.2
//  2021-01-01T00:01:00Z | 1.2
//  2021-01-01T00:02:00Z | 2.3
//  2021-01-01T00:03:00Z | 2.3
//  2021-01-01T00:04:00Z | 2.8
//  2021-01-01T00:05:00Z | 1.1
//
builtin fill : (<-tables: [{T with _value: A}], ?value: A, ?usePrevious: bool) => [{T with _value: A}]

// first is a function that returns the first record with a non-null value in
//  the _value column.
//
//  first function is subject to change at any time. By using this function,
//  you accept the risks of experimental functions.
//
// ## Empty tables
// experimental.first() drops empty tables.
//
// ## Parameters
// - `tables` is the input data.
//
//   Default is pipe-forwarded data.
//
// ## Return the first non-null value
//
// ```
// import "experimental"
//
// data
//   |> experimental.first()
// ```
//
// # Input data
//  _time | _value
//  --- | ---
//  2021-01-01T00:00:00Z | 1.2
//  2021-01-01T00:01:00Z | 0.6
//  2021-01-01T00:02:00Z | 2.3
//  2021-01-01T00:03:00Z | 0.9
//
// # Output table
//  _time | _value
//  --- | ---
//  2021-01-01T00:00:00Z | 1.2
//
builtin first : (<-tables: [{T with _value: A}]) => [{T with _value: A}]

// last is a function that returns the last record with a non-null value
//  in the _value column. 
//
//  last function is subject to change at any time. By using this function,
//  you accept the risks of experimental functions.
//
// ## Empty tables
// experimental.last drops empty tables.
//
// ## Parameters
// - `tables` is the input data.
//
//   Default is pipe-forwarded data.
//
// ## Return the last non-null value
//
// ```
// import "experimental"
//
// data
//   |> experimental.last()
// ```
//
// # Input data
//  _time | _value
//  --- | ---
//  2021-01-01T00:00:00Z | 1.2
//  2021-01-01T00:01:00Z | 0.6
//  2021-01-01T00:02:00Z | 2.3
//  2021-01-01T00:03:00Z | 0.9
//
// # Output data
//  _time | _value
//  --- | ---
//  2021-01-01T00:03:00Z | 0.9
//
builtin last : (<-tables: [{T with _value: A}]) => [{T with _value: A}]

// max is a function that returns the record with the highest value in the _value
//  column for each input table.
//
//  max function is subject to change at any time. By using this function, you
//  accept the risks of experimental functions.
//
// ## Empty tables
// experimental.max() drops empty tables.
//
// ## Parameters
// - `tables` is the input data.
//
//   Default is pipe-forwarded data.
//
// ## Example
//
// ```
// import "experimental"
//
// data
//   |> experimental.max()
// ```
//
// # Input data
//  _time | _value
//  --- | ---
//  2021-01-01T00:00:00Z | 1.2
//  2021-01-01T00:01:00Z | 0.6
//  2021-01-01T00:02:00Z | 2.3
//  2021-01-01T00:03:00Z | 0.9
//
// # Output table
//  _time | _value
//  --- | ---
//  2021-01-01T00:02:00Z | 2.3
//
builtin max : (<-tables: [{T with _value: A}]) => [{T with _value: A}]

// min is a function that returns the record with the lowest value in the
//  _value column for each input table.
//
//  min function is subject to change at any time. By using this function,
//  you accept the risks of experimental functions.
//
// ## Empty tables
// experimental.min() drops empty tables.
//
// ## Parameters
// - `tables` is the input data.
//
//   Default is pipe-forwarded data.
//
// ## Example
//
// ```
// import "experimental"
//
// data
//   |> experimental.min()
// ```
//
// # Input data
//  _time | _value
//  --- | ---
//  2021-01-01T00:00:00Z | 1.2
//  2021-01-01T00:01:00Z | 0.6
//  2021-01-01T00:02:00Z | 2.3
//  2021-01-01T00:03:00Z | 0.9
//
// # Output data
//  _time | _value
//  --- | ---
//  2021-01-01T00:01:00Z | 0.6
//
builtin min : (<-tables: [{T with _value: A}]) => [{T with _value: A}]

// unique is a function that returns all records containing unique values in the
//  _value column.
//
//  null is considered a unique value.
//
//  unique function is subject to change at any time. By using this function,
//  you accept the risks of experimental functions.
//
// ## Output schema
// experimental.unique() outputs a single table for each input table and does
// the following:
//
// - Outputs a single record for each unique value.
// - Leaves group keys, columns, and values unmodified.
//
// ## Empty tables
// experimental.unique() drops empty tables.
//
// ## Example
//
// ```
// import "experimental"
//
// data
//  |> experimental.unique()
// ```
//
// # Input table
//  _time | _field | _value
//  --- | --- | ---
//  2021-01-01T00:00:00Z | ver | v1
//  2021-01-01T00:01:00Z | ver | v1
//  2021-01-01T00:02:00Z | ver | v2
//  2021-01-01T00:03:00Z | ver | 
//  2021-01-01T00:04:00Z | ver | v3
//  2021-01-01T00:05:00Z | ver | v3
//
// # Output table
//  _time | _field | _value
//  --- | --- | ---
//  2021-01-01T00:00:00Z | ver | v1
//  2021-01-01T00:02:00Z | ver | v2
//  2021-01-01T00:03:00Z | ver | 
//  2021-01-01T00:04:00Z | ver | v3
//
builtin unique : (<-tables: [{T with _value: A}]) => [{T with _value: A}]

// histogram is a function that approximates the cumulative distribution of a
//  dataset by counting data frequencies for a list of bins.
//
//  A bin is defined by an upper bound where all data points that are less than
//  or equal to the bound are counted in the bin. Bin counts are cumulative.
//
// ## Output schema
// experimental.histogram() outputs a single table for each input table. Each
// output table represents a unique histogram. Output tables have the same group
// key as the corresponding input table.
//
//  The function does the following:
//  - Drops columns that are not part of the group key.
//  - Adds an le column to store upper bound values.
//  - Stores bin counts in the _value column.
//
// ## Parameters
// - `bins` is a list of upper bounds to use when computing the histogram frequencies,
//   including the maximum value of the data set.
//
//   This value can be set to positive infinity if no maximum is known.
//   Bin helper functions (the following can be used to generate bins):
//   - linearBins()
//   - logarithmicBins()
//
// - `normalize` can convert values into frequency values between 0 and 1.
//
//   Default is false.
//
// - `tables` is the input data.
//
//   Default is pipe-forwarded data.
//
// ## Histogram with dynamically generated bins
//
// ```
// import "experimental"
//
// data
//   |> experimental.histogram(
//     bins: linearBins(start:0.0, width:20.0, count:5)
//   )
// ```
//
// # Input data
//  _time | host | _value
//  --- | --- | ---
//  2021-01-01T00:00:00Z | host1 | 33.4
//  2021-01-01T00:01:00Z | host1 | 57.2
//  2021-01-01T00:02:00Z | host1 | 78.1
//  2021-01-01T00:03:00Z | host1 | 79.6
//
//  _time | host | _value
//  --- | --- | ---
//  2021-01-01T00:00:00Z | host2 | 10.3
//  2021-01-01T00:01:00Z | host2 | 19.8
//  2021-01-01T00:02:00Z | host2 | 54.6
//  2021-01-01T00:03:00Z | host2 | 56.9
//
// # Output data
//  host | le | _value
//  --- | --- | ---
//  host1 | 0 | 0
//  host1 | 20 | 0
//  host1 | 40 | 1
//  host1 | 60 | 2
//  host1 | 80 | 4
//  host1 | +Inf | 4
//
//  host | le | _value
//  --- | --- | ---
//  host2 | 0 | 0
//  host2 | 20 | 2
//  host2 | 40 | 2
//  host2 | 60 | 4
//  host2 | 80 | 4
//  host2 | +Inf | 4
//
builtin histogram : (<-tables: [{T with _value: float}], bins: [float], ?normalize: bool) => [{T with _value: float, le: float}]
