package aggregate


import "experimental"

// aggregate.rate is a function that calculates the rate of change per
// windows of time.
//
// - `every` is the duration of the time windows.
// - `groupColumns` is the list of columns to group by. Defaults
//	to [].
// - `unit` is the time duration to use when calculating the rate.
//	Defaults to 1s.
//
rate = (tables=<-, every, groupColumns=[], unit=1s) => tables
    |> derivative(nonNegative: true, unit: unit)
    |> aggregateWindow(
        every: every,
        fn: (tables=<-, column) => tables
            |> mean(column: column)
            |> group(columns: groupColumns)
            |> experimental.group(columns: ["_start", "_stop"], mode: "extend")
            |> sum(),
    )
