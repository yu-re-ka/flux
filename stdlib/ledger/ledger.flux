package ledger

import "strings"

// From reads a ledger (hledger) file and produces a single table with each posting as a row.
builtin from

assets = (tables=<-) =>
    tables
        |> filter(fn: (r) => strings.toLower(v:r.l0) == "assets")

expenses = (tables=<-) =>
    tables
        |> filter(fn: (r) => strings.toLower(v:r.l0) == "expenses")

income = (tables=<-) =>
    tables
        |> filter(fn: (r) => strings.toLower(v:r.l0) == "income")
