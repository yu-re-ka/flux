import "experimental/stdin"
import "math"

stdin.from() |>
  map(fn: (r) => ({ sin: math.sin(x: r.count) }))