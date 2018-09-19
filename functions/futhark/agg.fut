
entry sum (col : []f64) =
    f64.sum col

entry mean (col : []f64) =
    let s = f64.sum col in
    let n = length col in
    s / r64 n
