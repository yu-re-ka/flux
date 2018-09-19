
entry sum (col : []f64) =
    f64.sum col

entry mean (col : []f64) =
    let s = f64.sum col in
    let n = length col in
    s / r64 n

entry variance (values: []f64) =
    let compute (values: []f64): (f64, f64, f64) =
        let count = r64 (length values) in
        if count == 0.0 then (0, 0, 0) else
        let avg = f64.sum values / count in
        let var = f64.sum (map (\x -> (x - avg) * (x - avg)) values) / count in
        trace (avg, count, var)
    in

    let combine (avg_a, count_a, var_a) (avg_b, count_b, var_b) =
        -- count of zero means neutral element. return the other
        if count_a == 0.0 then (avg_b, count_b, var_b) else
        if count_b == 0.0 then (avg_a, count_a, var_a) else

        -- combine the counts
        let count_c = count_a + count_b in

        -- compute the combined average
        let sum_a = count_a * avg_a in
        let sum_b = count_b * avg_b in
        let avg_c = (sum_a + sum_b) / count_c in

        -- compute the combined variance
        let m_a = var_a * (count_a - 1) in
        let m_b = var_b * (count_b - 1) in
        let delta = avg_b - avg_a in
        let m2 = m_a + m_b + (delta * delta * count_a * count_b / count_c) in
        let var_c = m2 / (count_c - 1) in

        (avg_c, count_c, var_c)
    in

    let (_avg, _count, var) = stream_red combine compute values in

    var

entry stddev (values: []f64) =
    f64.sqrt (variance values)
