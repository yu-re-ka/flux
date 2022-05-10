// Package rand provides functions for generating random data.
//
// This package is meant to be used with the sources in `experimental/generate`.
//
// ## Metadata
// introduced: NEXT
// tags: generate
package rand


// int generates a random integer from [0, n).
//
// ## Parameters
// - n: Upper bound for random number. Default is all positive integers.
//
// ## Examples
//
// ### Random number from 0 to 100 (exclusive)
// ```no_run
// import "experimental/rand"
//
// n = rand.int(n: 100)
// ```
//
// ## Metadata
// introduced: NEXT
// tags: generate
//
builtin int : (?n: int) => int
