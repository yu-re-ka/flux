package outlier


import "join"

iqr = (tables=<-, s) => {
    first =
        tables
            |> quantile(q: 0.25)
    third =
        tables
            |> quantile(q: 0.75)

    inner =
        join.inner(
            left: first,
            right: third,
            on: (l, r) => l._stop == r._stop,
            as: (l, r) => ({l with first: l._value, third: r._value}),
        )

    return
        join.inner(
            left: tables,
            right: inner,
            on: (l, r) => l._stop == r._stop,
            as: (l, r) => ({l with first: r.first, thrid: r.third}),
        )
            |> filter(
                fn: (r) => {
                    iqr = (r.third - r.first) * s

                    return r._value > r.thrid + iqr or r._value < r.first - iqr
                },
            )
}
