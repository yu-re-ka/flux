package testdata_test

import "testing"

option now = () => (2030-01-01T00:00:00Z)

inData = "
#datatype,string,long,dateTime:RFC3339,double,string,string
#group,false,false,false,false,true,true
#default,_result,,,,,
,result,table,_time,_value,_field,_measurement
,,0,2018-05-22T19:53:30Z,1.2,active,mem
,,0,2018-05-23T19:53:40Z,5.7,active,mem
,,0,2018-05-24T19:53:50Z,56.4,active,mem
,,0,2018-05-25T19:54:00Z,93.2,active,mem
,,0,2018-05-26T19:54:10Z,34.9,active,mem
,,0,2018-05-27T19:54:20Z,11.1,active,mem
,,1,2018-05-22T19:53:30Z,12.3,f,m2
,,1,2018-05-23T19:53:40Z,15.2,f,m2
,,1,2018-05-24T19:53:50Z,43.1,f,m2
,,1,2018-05-25T19:54:00Z,21.9,f,m2
,,1,2018-05-26T19:54:10Z,32.5,f,m2
,,1,2018-05-27T19:54:20Z,75.2,f,m2
"

outData = "
#datatype,string,long,dateTime:RFC3339,dateTime:RFC3339,string,string,dateTime:RFC3339,double
#group,false,false,true,true,true,true,false,false
#default,_result,,,,,,,
,result,table,_start,_stop,_field,_measurement,_time,_value
,,0,2018-05-22T19:53:26.000000000Z,2030-01-01T00:00:00.000000000Z,active,mem,2018-05-24T00:00:00Z,20.25
,,0,2018-05-22T19:53:26.000000000Z,2030-01-01T00:00:00.000000000Z,active,mem,2018-05-25T00:00:00Z,2570.49
,,0,2018-05-22T19:53:26.000000000Z,2030-01-01T00:00:00.000000000Z,active,mem,2018-05-26T00:00:00Z,1354.2400000000002
,,0,2018-05-22T19:53:26.000000000Z,2030-01-01T00:00:00.000000000Z,active,mem,2018-05-27T00:00:00Z,3398.8900000000003
,,0,2018-05-22T19:53:26.000000000Z,2030-01-01T00:00:00.000000000Z,active,mem,2018-05-28T00:00:00Z,566.4399999999998
"

t_map = (table=<-) =>
	(table
		|> range(start: 2018-05-22T19:53:26Z)
        |> filter(fn: (r) => r._measurement == "mem" and r._field == "active")
        |> aggregateWindow(every: 1d, fn: mean, createEmpty: false)
        |> difference(nonNegative: false, columns: ["_value"])
        |> map(fn: (r) => ({r with _value: r._value * r._value}))
    )

test _map_return = () =>
	({input: testing.loadStorage(csv: inData), want: testing.loadMem(csv: outData), fn: t_map})

