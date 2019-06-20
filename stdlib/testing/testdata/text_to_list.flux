package testdata

import "strings"

text = "_time,_value,_field,_measurement,host
2018-05-22T19:53:26Z,1.83,load1,system,host.local
2018-05-22T19:53:36Z,1.7,load1,system,host.local
2018-05-22T19:53:46Z,1.74,load1,system,host.local
2018-05-22T19:53:56Z,1.63,load1,system,host.local
2018-05-22T19:54:06Z,1.91,load1,system,host.local
2018-05-22T19:54:16Z,1.84,load1,system,host.local
2018-05-22T19:53:26Z,1.98,load15,system,host.local
2018-05-22T19:53:36Z,1.97,load15,system,host.local
2018-05-22T19:53:46Z,1.97,load15,system,host.local
2018-05-22T19:53:56Z,1.96,load15,system,host.local
2018-05-22T19:54:06Z,1.98,load15,system,host.local
2018-05-22T19:54:16Z,1.97,load15,system,host.local
2018-05-22T19:53:26Z,1.95,load5,system,host.local
2018-05-22T19:53:36Z,1.92,load5,system,host.local
2018-05-22T19:53:46Z,1.92,load5,system,host.local
2018-05-22T19:53:56Z,1.89,load5,system,host.local
2018-05-22T19:54:06Z,1.94,load5,system,host.local
2018-05-22T19:54:16Z,1.93,load5,system,host.local"

lined = () => strings.split(v: text, t: "\n") // to get each line of text separately
header = () => strings.split(v: lined()[0], t: ",") // to get each column name separately
lst = []
/*
for i in range(1, 18):
    data = () => strings.split(v: lined()[i], t: ",")
    time = data()[0]
    value = data()[1]
    field = data()[2]
    measurement = data()[3]
    host = data()[4]
    lst.append({time, value, field, measurement, host})
*/