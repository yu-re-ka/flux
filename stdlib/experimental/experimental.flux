package experimental


// experimental.addDuration is a function that adds a duration to
// a time value and returns the resulting time value.
//
// - `d` is the duration to add.
// - `to` is the time to add the duration to.
//
builtin addDuration : (d: duration, to: time) => time

// experimental.subDuration is a function that subtracts a duration
// from a time value and returns the resulting time value.
//
// - `d` is the duration to subtract
// - `from` is the time to subtract the duration from.
//
builtin subDuration : (d: duration, from: time) => time

// experimental.group is a function that introduces an extend mode
// to the existing group() function.
//
// - `columns` is the list of coluns to use in the grouping operation.
//	Defaults to [].
// - `mode` is the mode used to group columns.
//
builtin group : (<-tables: [A], mode: string, columns: [string]) => [A] where A: Record

// experimental.objectKeys is a function that produces a list
// of the keys existing on the object.
//
// - `o` is the record to return keys from.
//
builtin objectKeys : (o: A) => [string] where A: Record

// experimental.set is a function that sets multiple static column values
// on all records. If a column already exists, the function updates the
// existing value. If the column does not exist, the function adds it
// with the specified value.
//
// - `o` is a record that defines the column and values to set. The
//	key of each key-value pair defines the column value.
//
builtin set : (<-tables: [A], o: B) => [C] where A: Record, B: Record, C: Record

// experimental.to is a function that writes data to an InfluxDB v2.0
// bucket, but in a different structure than that of the built-in
// to() function.
//
// - `bucket` is the bucket to write data to. bucket and bucketID are
//	mutually exclusive.
// - `bucketID` is the ID of the bucket to write data to. bucketID and
//	bucket are mutually exclusive.
// - `org` is the organization name of the specified bucket. Only
//	required when writing to a different organization or a
//	remote host. org and orgID are mutually exclusive.
// - `orgID` is the organization ID of the specified bucket.
//	Only required when writing to a different organization
//	or a remote host. orgID and org are mutually exclusive.
//
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

// experimental.join is a function that joins two streams of tables
// on the group key and the _time column. Use the fn parameter to
// map new output tables using values from input tables.
//
// - `left` is first of the two streams of tables to join.
// - `right` is second of the two streams of tables to join.
// - `fn` is a function with left and right arguments that maps a
//	new output record using values from the left and right
//	input records. The return value mest be a record.
//
builtin join : (left: [A], right: [B], fn: (left: A, right: B) => C) => [C] where A: Record, B: Record, C: Record

// experimental.chain is a function that runs two queries in a single
// flux script sequentially, and outputs the results of the second
// query. Flux typically executes multiple queries in a single script
// in parallel. Running the querries sequentially ensures any
// dependencies the second query has on the results of the first query
// are met.
//
// - `first` is the first query to execute.
// - `second` is the second query to execute.
//
builtin chain : (first: [A], second: [B]) => [B] where A: Record, B: Record

// Aligns all tables to a common start time by using the same _time value for
// the first record in each table and incrementing all subsequent _time values
// using time elapsed between input records.
// By default, it aligns to tables to 1970-01-01T00:00:00Z UTC.
alignTime = (tables=<-, alignTo=time(v: 0)) => tables
    |> stateDuration(
        fn: (r) => true,
        column: "timeDiff",
        unit: 1ns,
    )
    |> map(fn: (r) => ({r with _time: time(v: int(v: alignTo) + r.timeDiff)}))
    |> drop(columns: ["timeDiff"])

// experimental.window is a function that groups records based on a time
// value. New columns are added to uniquely identify each window. Those
// columns are added to the group key of the output table. Input
// tables must have _start, _stop, and _time columns.
//
// - `every` is the duration of time between windows. Defaults to
//	period value.
// - `period` is the duration of the window. Period is the length of
//	each interval. It can be negative, indicating the start and stop
//	boundaries are reversed. Defaults to every value.
// - `ofset` is the duration by which to shift the window boundaries. It
//	can be negative, indicating that the offset goes backwards in
//	time. Defaults to 0, which will align window end boundaries
//	with every duration.
// - `createEmpty` Specifies whether empty tables should be created.
//	Defaults to false.
//
builtin window : (
    <-tables: [{T with _start: time, _stop: time, _time: time}],
    ?every: duration,
    ?period: duration,
    ?offset: duration,
    ?createEmpty: bool,
) => [{T with _start: time, _stop: time, _time: time}]

// experimental.integral is a function that computes the area under
// the curve per unit of time of subsequent non-null records. The curve
// is defined using _time as the domain and the record values as the
// range. Input tables must have _time and _value columns.
//
// - `unit` is the time duration used to compute the integral.
// - `interpolate` is the type of interpolation used. Defaults to "".
//
builtin integral : (<-tables: [{T with _time: time, _value: B}], ?unit: duration, ?interpolate: string) => [{T with _value: B}]

// experimental.count is a function that outputs the number of records
// in each input table and returns the count in the _value column. This
// function counts both null and non-null records.
//
//
builtin count : (<-tables: [{T with _value: A}]) => [{T with _value: int}]

// experimental.histogramQuantile is a function that approximates a quantile
// given a histogram with the cumulative distribution of the dataset.
// Each input table represents a single histogram. Input tables must
// have two columns - a count column (_value) and an upper bounds column
// (le), and neither column can be part of the group key.
// The count is the number of values that are less than or equal to the upper
// bound value (le). Input tables can have an unlimited number of records;
// each record represents an entry in the histogram. The counts must be
// monotonically increasing when sorted by upper bounds (le). If any
// values in the _value or le columns are null, the function returns error.
//
// - `quantile` is a value between 0 and 1 indicating the desired quantile
//	to compute.
// - `minValue` is the assumed minimum value of the dataset. When the quantile
//	falls below the lowest upper bound, interpolation is performed
//	between minValue and the lowest upper bound. When minValue is equal
//	to negative infinity, the lowest upper bound is used. Defaults to
//	0.0.
//
builtin histogramQuantile : (<-tables: [{T with _value: float, le: float}], ?quantile: float, ?minValue: float) => [{T with _value: float}]

// experimental.mean is a function that computes the mean or average of
// non-null values in the _value column of each input table. Output tables
// contain a single row with the calculated mean in the _value column.
//
builtin mean : (<-tables: [{T with _value: float}]) => [{T with _value: float}]

// experimental.mode is a function that computes the mode or value that
// occurs most often in the _value column in each input table
//
builtin mode : (<-tables: [{T with _value: A}]) => [{T with _value: A}]

// experimental.quantile is a function that outputs non-null records with
// values in the _value column that fall within the specified quantile or
// represent a specified quantile. Which it returns depends on the method
// used. the _value column must contain float values.
// When using estimate_tdigest or exact_mean methods, the function outputs
// non-null records with values that fall within the specified quantile.
// When using the exact_selector method, it outputs the non-null record
// with the value that represents the specified quantile.
//
// - `q` is a value between 0 and 1 that specifies the quantile.
// - `method` is the computation method. Default is estimate_tdigest.
// - `compression` indicates how many centroids to use when compressing
//	the dataset. A larger number produces a more accurate result at
//	the cost of increased memory requrements. Defaults to 1000.0.
//
builtin quantile : (<-tables: [{T with _value: float}], q: float, ?compression: float, ?method: string) => [{T with _value: float}]

// experimental.skew is a function that outputs the skew of non-null values
// in the _value column for each input table, as a float.
//
builtin skew : (<-tables: [{T with _value: float}]) => [{T with _value: float}]

// An experimental version of spread.
builtin spread : (<-tables: [{T with _value: A}]) => [{T with _value: A}] where A: Numeric

// experimental.stddev is a function that computes the standard deviation of
// non-null values in the _value column for each input table.
//
// - `mode`is the standard deviation mode or type of standard deviation to
//	calculate. Defaults to "sample"
//
builtin stddev : (<-tables: [{T with _value: float}], ?mode: string) => [{T with _value: float}]

// experimental.sum is a function that computes the sum of non-null values
// in the _value column for each input table.
//
builtin sum : (<-tables: [{T with _value: A}]) => [{T with _value: A}] where A: Numeric

// experimental.kaufmansAMA is a function that calculates the Kaufman's
// Adaptive Moving Average (KAMA) of input tables using the _value column
// in each table.
//
// - `n` is the period or number of points to use in the calculation.
//
builtin kaufmansAMA : (<-tables: [{T with _value: A}], n: int) => [{T with _value: float}] where A: Numeric

// experimental.distinct is a function that returns unique values from the
// _value column. The _value of each output record is set to  a distinct
// value in the specified column. null is considered a distinct value.
// This function outputs a single table for each input table and does the
// following: Outputs a single record for each distinct value, and drops
// all columns not in the group key.
//
// - `tables` is the input data. Default is pipe-forwarded data.
//
builtin distinct : (<-tables: [{T with _value: A}]) => [{T with _value: A}]

// experimental.fill is a function that replaces are null values in the
// _value column with a non-null value.
//
// - `value` is the value to be used to replace null values with. Data
//	type must match the type of the _value column.
// - `usePrevious` when set to true, causes the replace of null values with
//	the value of the previous non-null row.
// - `tables` is the input data. Default is pipe-forwarded data.
//
builtin fill : (<-tables: [{T with _value: A}], ?value: A, ?usePrevious: bool) => [{T with _value: A}]

// experimental.first is a function that returns the first record with a
// non-null value in the _value column.
//
// - `tables` is the input data. Default is pip-forwarded data.
//
builtin first : (<-tables: [{T with _value: A}]) => [{T with _value: A}]

// experimental.last is a function that returns the last record with a
// non-null value in the _value column.
//
// - `tables` is the input data. Default is pipe-forwarded data.
//
builtin last : (<-tables: [{T with _value: A}]) => [{T with _value: A}]

// experimental.max is a function that returns the record with the highest
// value in the _value column for each input table.
//
// - `tables` is the input data. Default is pipe-forwarded data.
//
builtin max : (<-tables: [{T with _value: A}]) => [{T with _value: A}]

// experimental.min is a funciton that returns the record with the lowest
// value in the _value column for each input table.
//
// - `tables` is the input data. Default is pipe-forwarded data.
//
builtin min : (<-tables: [{T with _value: A}]) => [{T with _value: A}]

// experimental.unique is a function that returns all records containing
// unique values in the _value column. null is considered a unique
// value.
//
// - `tables` is the input data. Default is pipe-forwarded data.
//
builtin unique : (<-tables: [{T with _value: A}]) => [{T with _value: A}]

// experimental.histogram is a function that approximates the cumulative
// distribution of a database by counting data frequencies for a list of
// bins. A bin is defined by an upper bound where all data points that
// are less than or equal to the bound are counted in the bin. Bin counts
// are cumulative.
//
// - `tables` is the input data. Default is pipe-forwarded data.
// - `bins` is a list of upper bounds to use when computing the histogram
//	frequencies, including the maximum value of the data set. This
//	value can be set to positive infinity if no maximum is known.
// - `normalize` converts the count values into frequency values
//	between 0 and 1. Default is false.
//
builtin histogram : (<-tables: [{T with _value: float}], bins: [float], ?normalize: bool) => [{T with _value: float, le: float}]
