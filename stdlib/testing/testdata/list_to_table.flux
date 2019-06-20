package testdata

import "strings"

list =
[{2018-05-22T19:53:26Z,1.83,load1,system,host.local},
{2018-05-22T19:53:36Z,1.7,load1,system,host.local},
{2018-05-22T19:53:46Z,1.74,load1,system,host.local}]

lst = () => list

fn = (list=<-) => {
    table
        |> range(start: 2018-05-22T19:53:26Z)
        |> map(fn: (r) => ({time: r[0], value: r[1], field: r[2], measurement: r[3], host: r[4]}))
}