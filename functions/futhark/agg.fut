
entry sum (col : []f64) =
    f64.sum col

entry mean (col : []f64) =
    let s = f64.sum col in
    let n = length col in
    s / r64 n

entry variance (values: []f64) =
    let len = r64 (length values) in
    let sum = f64.sum values in
    let avg = sum / len in

    let second = f64.sum (map (\v ->
        let shifted = v - avg in
        shifted * shifted
    ) values) in

    second / (len - 1)

entry stddev (values: []f64) =
    f64.sqrt (variance values)

entry skew (values: []f64) =
    let len = r64 (length values) in
    let sum = f64.sum values in
    let avg = sum / len in

    let second = f64.sum (map (\v ->
        let shifted = v - avg in
        shifted * shifted
    ) values) in

    let third = f64.sum (map (\v ->
        let shifted = v - avg in
        shifted * shifted * shifted
    ) values) in

    let std = f64.sqrt second in

    (f64.sqrt len) * third / (std * std * std)

entry kurtosis (values: []f64) =
    let len = r64 (length values) in
    let sum = f64.sum values in
    let avg = sum / len in

    let second = f64.sum (map (\v ->
        let shifted = v - avg in
        shifted * shifted
    ) values) in

    let fourth = f64.sum (map (\v ->
        let shifted = v - avg in
        shifted * shifted * shifted * shifted
    ) values) in

    len * fourth / (second * second)
