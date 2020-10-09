package csv_test

import (
	"bytes"
	"context"
	"errors"
	"io/ioutil"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/andreyvit/diff"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/influxdata/flux"
	"github.com/influxdata/flux/csv"
	"github.com/influxdata/flux/execute/executetest"
	"github.com/influxdata/flux/memory"
	"github.com/influxdata/flux/values"
)

type TestCase struct {
	name          string
	skip          bool
	encoded       []byte
	result        *executetest.Result
	err           error
	decoderConfig csv.ResultDecoderConfig
	encoderConfig csv.ResultEncoderConfig
}

var symmetricalTestCases = []TestCase{
	{
		name:          "single table",
		encoderConfig: csv.DefaultEncoderConfig(),
		encoded: toCRLF(`#datatype,string,long,dateTime:RFC3339,dateTime:RFC3339,dateTime:RFC3339,string,string,double
#group,false,false,true,true,false,true,true,false
#default,_result,,,,,,,
,result,table,_start,_stop,_time,_measurement,host,_value
,,0,2018-04-17T00:00:00Z,2018-04-17T00:05:00Z,2018-04-17T00:00:00Z,cpu,A,42
,,0,2018-04-17T00:00:00Z,2018-04-17T00:05:00Z,2018-04-17T00:00:01Z,cpu,A,43
`),
		result: &executetest.Result{
			Nm: "_result",
			Tbls: []*executetest.Table{{
				KeyCols: []string{"_start", "_stop", "_measurement", "host"},
				ColMeta: []flux.ColMeta{
					{Label: "_start", Type: flux.TTime},
					{Label: "_stop", Type: flux.TTime},
					{Label: "_time", Type: flux.TTime},
					{Label: "_measurement", Type: flux.TString},
					{Label: "host", Type: flux.TString},
					{Label: "_value", Type: flux.TFloat},
				},
				Data: [][]interface{}{
					{
						values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 0, 0, time.UTC)),
						values.ConvertTime(time.Date(2018, 4, 17, 0, 5, 0, 0, time.UTC)),
						values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 0, 0, time.UTC)),
						"cpu",
						"A",
						42.0,
					},
					{
						values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 0, 0, time.UTC)),
						values.ConvertTime(time.Date(2018, 4, 17, 0, 5, 0, 0, time.UTC)),
						values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 1, 0, time.UTC)),
						"cpu",
						"A",
						43.0,
					},
				},
			}},
		},
	},
	{
		name:          "single table with null",
		encoderConfig: csv.DefaultEncoderConfig(),
		encoded: toCRLF(`#datatype,string,long,dateTime:RFC3339,dateTime:RFC3339,dateTime:RFC3339,string,string,double
#group,false,false,true,true,false,true,true,false
#default,_result,,,,,,,
,result,table,_start,_stop,_time,_measurement,host,_value
,,0,2018-04-17T00:00:00Z,2018-04-17T00:05:00Z,2018-04-17T00:00:00Z,cpu,A,42
,,0,2018-04-17T00:00:00Z,2018-04-17T00:05:00Z,2018-04-17T00:00:01Z,cpu,A,
`),
		result: &executetest.Result{
			Nm: "_result",
			Tbls: []*executetest.Table{{
				KeyCols: []string{"_start", "_stop", "_measurement", "host"},
				ColMeta: []flux.ColMeta{
					{Label: "_start", Type: flux.TTime},
					{Label: "_stop", Type: flux.TTime},
					{Label: "_time", Type: flux.TTime},
					{Label: "_measurement", Type: flux.TString},
					{Label: "host", Type: flux.TString},
					{Label: "_value", Type: flux.TFloat},
				},
				Data: [][]interface{}{
					{
						values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 0, 0, time.UTC)),
						values.ConvertTime(time.Date(2018, 4, 17, 0, 5, 0, 0, time.UTC)),
						values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 0, 0, time.UTC)),
						"cpu",
						"A",
						42.0,
					},
					{
						values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 0, 0, time.UTC)),
						values.ConvertTime(time.Date(2018, 4, 17, 0, 5, 0, 0, time.UTC)),
						values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 1, 0, time.UTC)),
						"cpu",
						"A",
						nil,
					},
				},
			}},
		},
	},
	{
		name:          "single table with null in group key column",
		encoderConfig: csv.DefaultEncoderConfig(),
		encoded: toCRLF(`#datatype,string,long,dateTime:RFC3339,dateTime:RFC3339,dateTime:RFC3339,string,string,double
#group,false,false,true,true,false,true,true,false
#default,_result,,,,,,,
,result,table,_start,_stop,_time,_measurement,host,_value
,,0,2018-04-17T00:00:00Z,2018-04-17T00:05:00Z,2018-04-17T00:00:00Z,cpu,,42
,,0,2018-04-17T00:00:00Z,2018-04-17T00:05:00Z,2018-04-17T00:00:01Z,cpu,,43
`),
		result: &executetest.Result{
			Nm: "_result",
			Tbls: []*executetest.Table{{
				KeyCols: []string{"_start", "_stop", "_measurement", "host"},
				ColMeta: []flux.ColMeta{
					{Label: "_start", Type: flux.TTime},
					{Label: "_stop", Type: flux.TTime},
					{Label: "_time", Type: flux.TTime},
					{Label: "_measurement", Type: flux.TString},
					{Label: "host", Type: flux.TString},
					{Label: "_value", Type: flux.TFloat},
				},
				Data: [][]interface{}{
					{
						values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 0, 0, time.UTC)),
						values.ConvertTime(time.Date(2018, 4, 17, 0, 5, 0, 0, time.UTC)),
						values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 0, 0, time.UTC)),
						"cpu",
						nil,
						42.0,
					},
					{
						values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 0, 0, time.UTC)),
						values.ConvertTime(time.Date(2018, 4, 17, 0, 5, 0, 0, time.UTC)),
						values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 1, 0, time.UTC)),
						"cpu",
						nil,
						43.0,
					},
				},
			}},
		},
	},
	{
		name:          "single empty table",
		encoderConfig: csv.DefaultEncoderConfig(),
		encoded: toCRLF(`#datatype,string,long,dateTime:RFC3339,dateTime:RFC3339,dateTime:RFC3339,string,string,double
#group,false,false,true,true,false,true,true,false
#default,_result,0,2018-04-17T00:00:00Z,2018-04-17T00:05:00Z,,cpu,A,
,result,table,_start,_stop,_time,_measurement,host,_value
`),
		result: &executetest.Result{
			Nm: "_result",
			Tbls: []*executetest.Table{{
				KeyCols: []string{"_start", "_stop", "_measurement", "host"},
				KeyValues: []interface{}{
					values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 0, 0, time.UTC)),
					values.ConvertTime(time.Date(2018, 4, 17, 0, 5, 0, 0, time.UTC)),
					"cpu",
					"A",
				},
				ColMeta: []flux.ColMeta{
					{Label: "_start", Type: flux.TTime},
					{Label: "_stop", Type: flux.TTime},
					{Label: "_time", Type: flux.TTime},
					{Label: "_measurement", Type: flux.TString},
					{Label: "host", Type: flux.TString},
					{Label: "_value", Type: flux.TFloat},
				},
			}},
		},
	},
	{
		name:          "single empty table with no columns",
		encoderConfig: csv.DefaultEncoderConfig(),
		encoded: toCRLF(`#datatype,string,long
#group,false,false
#default,_result,0
,result,table
`),
		result: &executetest.Result{
			Nm: "_result",
			Tbls: []*executetest.Table{{
				KeyCols:   []string(nil),
				KeyValues: []interface{}(nil),
				ColMeta:   []flux.ColMeta(nil),
			}},
		},
	},
	{
		name:          "multiple tables",
		encoderConfig: csv.DefaultEncoderConfig(),
		encoded: toCRLF(`#datatype,string,long,dateTime:RFC3339,dateTime:RFC3339,dateTime:RFC3339,string,string,double
#group,false,false,true,true,false,true,true,false
#default,_result,,,,,,,
,result,table,_start,_stop,_time,_measurement,host,_value
,,0,2018-04-17T00:00:00Z,2018-04-17T00:05:00Z,2018-04-17T00:00:00Z,cpu,A,42
,,0,2018-04-17T00:00:00Z,2018-04-17T00:05:00Z,2018-04-17T00:00:01Z,cpu,A,43
,,1,2018-04-17T00:05:00Z,2018-04-17T00:10:00Z,2018-04-17T00:06:00Z,mem,A,52
,,1,2018-04-17T00:05:00Z,2018-04-17T00:10:00Z,2018-04-17T00:07:01Z,mem,A,53
`),
		result: &executetest.Result{
			Nm: "_result",
			Tbls: []*executetest.Table{
				{
					KeyCols: []string{"_start", "_stop", "_measurement", "host"},
					ColMeta: []flux.ColMeta{
						{Label: "_start", Type: flux.TTime},
						{Label: "_stop", Type: flux.TTime},
						{Label: "_time", Type: flux.TTime},
						{Label: "_measurement", Type: flux.TString},
						{Label: "host", Type: flux.TString},
						{Label: "_value", Type: flux.TFloat},
					},
					Data: [][]interface{}{
						{
							values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 0, 0, time.UTC)),
							values.ConvertTime(time.Date(2018, 4, 17, 0, 5, 0, 0, time.UTC)),
							values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 0, 0, time.UTC)),
							"cpu",
							"A",
							42.0,
						},
						{
							values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 0, 0, time.UTC)),
							values.ConvertTime(time.Date(2018, 4, 17, 0, 5, 0, 0, time.UTC)),
							values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 1, 0, time.UTC)),
							"cpu",
							"A",
							43.0,
						},
					},
				},
				{
					KeyCols: []string{"_start", "_stop", "_measurement", "host"},
					ColMeta: []flux.ColMeta{
						{Label: "_start", Type: flux.TTime},
						{Label: "_stop", Type: flux.TTime},
						{Label: "_time", Type: flux.TTime},
						{Label: "_measurement", Type: flux.TString},
						{Label: "host", Type: flux.TString},
						{Label: "_value", Type: flux.TFloat},
					},
					Data: [][]interface{}{
						{
							values.ConvertTime(time.Date(2018, 4, 17, 0, 5, 0, 0, time.UTC)),
							values.ConvertTime(time.Date(2018, 4, 17, 0, 10, 0, 0, time.UTC)),
							values.ConvertTime(time.Date(2018, 4, 17, 0, 6, 0, 0, time.UTC)),
							"mem",
							"A",
							52.0,
						},
						{
							values.ConvertTime(time.Date(2018, 4, 17, 0, 5, 0, 0, time.UTC)),
							values.ConvertTime(time.Date(2018, 4, 17, 0, 10, 0, 0, time.UTC)),
							values.ConvertTime(time.Date(2018, 4, 17, 0, 7, 1, 0, time.UTC)),
							"mem",
							"A",
							53.0,
						},
					},
				},
			},
		},
	},
	{
		name:          "multiple tables with differing schemas",
		encoderConfig: csv.DefaultEncoderConfig(),
		encoded: toCRLF(`#datatype,string,long,dateTime:RFC3339,dateTime:RFC3339,dateTime:RFC3339,string,string,double
#group,false,false,true,true,false,true,true,false
#default,_result,,,,,,,
,result,table,_start,_stop,_time,_measurement,host,_value
,,0,2018-04-17T00:00:00Z,2018-04-17T00:05:00Z,2018-04-17T00:00:00Z,cpu,A,42
,,0,2018-04-17T00:00:00Z,2018-04-17T00:05:00Z,2018-04-17T00:00:01Z,cpu,A,43
,,1,2018-04-17T00:05:00Z,2018-04-17T00:10:00Z,2018-04-17T00:06:00Z,mem,A,52
,,1,2018-04-17T00:05:00Z,2018-04-17T00:10:00Z,2018-04-17T00:07:01Z,mem,A,53

#datatype,string,long,dateTime:RFC3339,dateTime:RFC3339,dateTime:RFC3339,string,string,double,double
#group,false,false,true,true,false,true,false,false,false
#default,_result,,,,,,,,
,result,table,_start,_stop,_time,location,device,min,max
,,2,2018-04-17T00:00:00Z,2018-04-17T00:05:00Z,2018-04-17T00:00:00Z,USA,1563,42,67.9
,,2,2018-04-17T00:00:00Z,2018-04-17T00:05:00Z,2018-04-17T00:00:01Z,USA,1414,43,44.7
,,3,2018-04-17T00:05:00Z,2018-04-17T00:10:00Z,2018-04-17T00:06:00Z,Europe,4623,52,89.3
,,3,2018-04-17T00:05:00Z,2018-04-17T00:10:00Z,2018-04-17T00:07:01Z,Europe,3163,53,55.6
`),
		result: &executetest.Result{
			Nm: "_result",
			Tbls: []*executetest.Table{
				{
					KeyCols: []string{"_start", "_stop", "_measurement", "host"},
					ColMeta: []flux.ColMeta{
						{Label: "_start", Type: flux.TTime},
						{Label: "_stop", Type: flux.TTime},
						{Label: "_time", Type: flux.TTime},
						{Label: "_measurement", Type: flux.TString},
						{Label: "host", Type: flux.TString},
						{Label: "_value", Type: flux.TFloat},
					},
					Data: [][]interface{}{
						{
							values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 0, 0, time.UTC)),
							values.ConvertTime(time.Date(2018, 4, 17, 0, 5, 0, 0, time.UTC)),
							values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 0, 0, time.UTC)),
							"cpu",
							"A",
							42.0,
						},
						{
							values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 0, 0, time.UTC)),
							values.ConvertTime(time.Date(2018, 4, 17, 0, 5, 0, 0, time.UTC)),
							values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 1, 0, time.UTC)),
							"cpu",
							"A",
							43.0,
						},
					},
				},
				{
					KeyCols: []string{"_start", "_stop", "_measurement", "host"},
					ColMeta: []flux.ColMeta{
						{Label: "_start", Type: flux.TTime},
						{Label: "_stop", Type: flux.TTime},
						{Label: "_time", Type: flux.TTime},
						{Label: "_measurement", Type: flux.TString},
						{Label: "host", Type: flux.TString},
						{Label: "_value", Type: flux.TFloat},
					},
					Data: [][]interface{}{
						{
							values.ConvertTime(time.Date(2018, 4, 17, 0, 5, 0, 0, time.UTC)),
							values.ConvertTime(time.Date(2018, 4, 17, 0, 10, 0, 0, time.UTC)),
							values.ConvertTime(time.Date(2018, 4, 17, 0, 6, 0, 0, time.UTC)),
							"mem",
							"A",
							52.0,
						},
						{
							values.ConvertTime(time.Date(2018, 4, 17, 0, 5, 0, 0, time.UTC)),
							values.ConvertTime(time.Date(2018, 4, 17, 0, 10, 0, 0, time.UTC)),
							values.ConvertTime(time.Date(2018, 4, 17, 0, 7, 1, 0, time.UTC)),
							"mem",
							"A",
							53.0,
						},
					},
				},
				{
					KeyCols: []string{"_start", "_stop", "location"},
					ColMeta: []flux.ColMeta{
						{Label: "_start", Type: flux.TTime},
						{Label: "_stop", Type: flux.TTime},
						{Label: "_time", Type: flux.TTime},
						{Label: "location", Type: flux.TString},
						{Label: "device", Type: flux.TString},
						{Label: "min", Type: flux.TFloat},
						{Label: "max", Type: flux.TFloat},
					},
					Data: [][]interface{}{
						{
							values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 0, 0, time.UTC)),
							values.ConvertTime(time.Date(2018, 4, 17, 0, 5, 0, 0, time.UTC)),
							values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 0, 0, time.UTC)),
							"USA",
							"1563",
							42.0,
							67.9,
						},
						{
							values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 0, 0, time.UTC)),
							values.ConvertTime(time.Date(2018, 4, 17, 0, 5, 0, 0, time.UTC)),
							values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 1, 0, time.UTC)),
							"USA",
							"1414",
							43.0,
							44.7,
						},
					},
				},
				{
					KeyCols: []string{"_start", "_stop", "location"},
					ColMeta: []flux.ColMeta{
						{Label: "_start", Type: flux.TTime},
						{Label: "_stop", Type: flux.TTime},
						{Label: "_time", Type: flux.TTime},
						{Label: "location", Type: flux.TString},
						{Label: "device", Type: flux.TString},
						{Label: "min", Type: flux.TFloat},
						{Label: "max", Type: flux.TFloat},
					},
					Data: [][]interface{}{
						{
							values.ConvertTime(time.Date(2018, 4, 17, 0, 5, 0, 0, time.UTC)),
							values.ConvertTime(time.Date(2018, 4, 17, 0, 10, 0, 0, time.UTC)),
							values.ConvertTime(time.Date(2018, 4, 17, 0, 6, 0, 0, time.UTC)),
							"Europe",
							"4623",
							52.0,
							89.3,
						},
						{
							values.ConvertTime(time.Date(2018, 4, 17, 0, 5, 0, 0, time.UTC)),
							values.ConvertTime(time.Date(2018, 4, 17, 0, 10, 0, 0, time.UTC)),
							values.ConvertTime(time.Date(2018, 4, 17, 0, 7, 1, 0, time.UTC)),
							"Europe",
							"3163",
							53.0,
							55.6,
						},
					},
				},
			},
		},
	},
	{
		name:          "multiple tables with one empty",
		encoderConfig: csv.DefaultEncoderConfig(),
		encoded: toCRLF(`#datatype,string,long,dateTime:RFC3339,dateTime:RFC3339,dateTime:RFC3339,string,string,double
#group,false,false,true,true,false,true,true,false
#default,_result,,,,,,,
,result,table,_start,_stop,_time,_measurement,host,_value
,,0,2018-04-17T00:00:00Z,2018-04-17T00:05:00Z,2018-04-17T00:00:00Z,cpu,A,42
,,0,2018-04-17T00:00:00Z,2018-04-17T00:05:00Z,2018-04-17T00:00:01Z,cpu,A,43
,,1,2018-04-17T00:05:00Z,2018-04-17T00:10:00Z,2018-04-17T00:06:00Z,mem,A,52
,,1,2018-04-17T00:05:00Z,2018-04-17T00:10:00Z,2018-04-17T00:07:01Z,mem,A,53

#datatype,string,long,dateTime:RFC3339,dateTime:RFC3339,dateTime:RFC3339,string,string,double
#group,false,false,true,true,false,true,true,false
#default,_result,2,2018-04-17T00:00:00Z,2018-04-17T00:05:00Z,,cpu,A,
,result,table,_start,_stop,_time,_measurement,host,_value
`),
		result: &executetest.Result{
			Nm: "_result",
			Tbls: []*executetest.Table{
				{
					KeyCols: []string{"_start", "_stop", "_measurement", "host"},
					ColMeta: []flux.ColMeta{
						{Label: "_start", Type: flux.TTime},
						{Label: "_stop", Type: flux.TTime},
						{Label: "_time", Type: flux.TTime},
						{Label: "_measurement", Type: flux.TString},
						{Label: "host", Type: flux.TString},
						{Label: "_value", Type: flux.TFloat},
					},
					Data: [][]interface{}{
						{
							values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 0, 0, time.UTC)),
							values.ConvertTime(time.Date(2018, 4, 17, 0, 5, 0, 0, time.UTC)),
							values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 0, 0, time.UTC)),
							"cpu",
							"A",
							42.0,
						},
						{
							values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 0, 0, time.UTC)),
							values.ConvertTime(time.Date(2018, 4, 17, 0, 5, 0, 0, time.UTC)),
							values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 1, 0, time.UTC)),
							"cpu",
							"A",
							43.0,
						},
					},
				},
				{
					KeyCols: []string{"_start", "_stop", "_measurement", "host"},
					ColMeta: []flux.ColMeta{
						{Label: "_start", Type: flux.TTime},
						{Label: "_stop", Type: flux.TTime},
						{Label: "_time", Type: flux.TTime},
						{Label: "_measurement", Type: flux.TString},
						{Label: "host", Type: flux.TString},
						{Label: "_value", Type: flux.TFloat},
					},
					Data: [][]interface{}{
						{
							values.ConvertTime(time.Date(2018, 4, 17, 0, 5, 0, 0, time.UTC)),
							values.ConvertTime(time.Date(2018, 4, 17, 0, 10, 0, 0, time.UTC)),
							values.ConvertTime(time.Date(2018, 4, 17, 0, 6, 0, 0, time.UTC)),
							"mem",
							"A",
							52.0,
						},
						{
							values.ConvertTime(time.Date(2018, 4, 17, 0, 5, 0, 0, time.UTC)),
							values.ConvertTime(time.Date(2018, 4, 17, 0, 10, 0, 0, time.UTC)),
							values.ConvertTime(time.Date(2018, 4, 17, 0, 7, 1, 0, time.UTC)),
							"mem",
							"A",
							53.0,
						},
					},
				},
				{
					KeyCols: []string{"_start", "_stop", "_measurement", "host"},
					KeyValues: []interface{}{
						values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 0, 0, time.UTC)),
						values.ConvertTime(time.Date(2018, 4, 17, 0, 5, 0, 0, time.UTC)),
						"cpu",
						"A",
					},
					ColMeta: []flux.ColMeta{
						{Label: "_start", Type: flux.TTime},
						{Label: "_stop", Type: flux.TTime},
						{Label: "_time", Type: flux.TTime},
						{Label: "_measurement", Type: flux.TString},
						{Label: "host", Type: flux.TString},
						{Label: "_value", Type: flux.TFloat},
					},
				},
			},
		},
	},
}

func TestResultDecoder(t *testing.T) {
	testCases := []TestCase{
		{
			name:          "single table with defaults",
			encoderConfig: csv.DefaultEncoderConfig(),
			encoded: toCRLF(`#datatype,string,long,dateTime:RFC3339,dateTime:RFC3339,dateTime:RFC3339,string,string,double
#group,false,false,true,true,false,true,true,false
#default,_result,0,2018-04-17T00:00:00Z,2018-04-17T00:05:00Z,,cpu,A,
,result,table,_start,_stop,_time,_measurement,host,_value
,,,,,2018-04-17T00:00:00Z,cpu,A,42.0
,,,,,2018-04-17T00:00:01Z,cpu,A,43.0
`),
			result: &executetest.Result{
				Nm: "_result",
				Tbls: []*executetest.Table{{
					KeyCols: []string{"_start", "_stop", "_measurement", "host"},
					ColMeta: []flux.ColMeta{
						{Label: "_start", Type: flux.TTime},
						{Label: "_stop", Type: flux.TTime},
						{Label: "_time", Type: flux.TTime},
						{Label: "_measurement", Type: flux.TString},
						{Label: "host", Type: flux.TString},
						{Label: "_value", Type: flux.TFloat},
					},
					Data: [][]interface{}{
						{
							values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 0, 0, time.UTC)),
							values.ConvertTime(time.Date(2018, 4, 17, 0, 5, 0, 0, time.UTC)),
							values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 0, 0, time.UTC)),
							"cpu",
							"A",
							42.0,
						},
						{
							values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 0, 0, time.UTC)),
							values.ConvertTime(time.Date(2018, 4, 17, 0, 5, 0, 0, time.UTC)),
							values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 1, 0, time.UTC)),
							"cpu",
							"A",
							43.0,
						},
					},
				}},
			},
		},
		{
			name:          "single table with unnecessary default tableID",
			encoderConfig: csv.DefaultEncoderConfig(),
			encoded: toCRLF(`#datatype,string,long,dateTime:RFC3339,dateTime:RFC3339,dateTime:RFC3339,string,string,double
#group,false,false,true,true,false,true,true,false
#default,_result,111,,,,,,
,result,table,_start,_stop,_time,_measurement,host,_value
,,0,2018-04-17T00:00:00Z,2018-04-17T00:05:00Z,2018-04-17T00:00:00Z,cpu,A,42
,,0,2018-04-17T00:00:00Z,2018-04-17T00:05:00Z,2018-04-17T00:00:01Z,cpu,A,43
`),
			result: &executetest.Result{
				Nm: "_result",
				Tbls: []*executetest.Table{{
					KeyCols: []string{"_start", "_stop", "_measurement", "host"},
					ColMeta: []flux.ColMeta{
						{Label: "_start", Type: flux.TTime},
						{Label: "_stop", Type: flux.TTime},
						{Label: "_time", Type: flux.TTime},
						{Label: "_measurement", Type: flux.TString},
						{Label: "host", Type: flux.TString},
						{Label: "_value", Type: flux.TFloat},
					},
					Data: [][]interface{}{
						{
							values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 0, 0, time.UTC)),
							values.ConvertTime(time.Date(2018, 4, 17, 0, 5, 0, 0, time.UTC)),
							values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 0, 0, time.UTC)),
							"cpu",
							"A",
							42.0,
						},
						{
							values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 0, 0, time.UTC)),
							values.ConvertTime(time.Date(2018, 4, 17, 0, 5, 0, 0, time.UTC)),
							values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 1, 0, time.UTC)),
							"cpu",
							"A",
							43.0,
						},
					},
				}},
			},
		},
		{
			name:          "single empty table and an error",
			encoderConfig: csv.DefaultEncoderConfig(),
			encoded: toCRLF(`#datatype,string,long,dateTime:RFC3339,dateTime:RFC3339,dateTime:RFC3339,string,string,double
#group,false,false,true,true,false,true,true,false
#default,_result,0,2018-04-17T00:00:00Z,2018-04-18T00:00:00Z,2018-04-17T12:00:00Z,m,localhost,6.0
,result,table,_start,_stop,_time,_measurement,host,_value

#datatype,string,string
#group,true,true
#default,,
,error,reference
,here is an error,
`),
			result: &executetest.Result{
				Nm: "_result",
				Tbls: []*executetest.Table{
					{
						ColMeta: []flux.ColMeta{
							{Label: "_start", Type: flux.TTime},
							{Label: "_stop", Type: flux.TTime},
							{Label: "_time", Type: flux.TTime},
							{Label: "_measurement", Type: flux.TString},
							{Label: "host", Type: flux.TString},
							{Label: "_value", Type: flux.TFloat},
						},
						KeyCols: []string{"_start", "_stop", "_measurement", "host"},
						KeyValues: []interface{}{
							values.ConvertTime(time.Date(2018, 04, 17, 0, 0, 0, 0, time.UTC)),
							values.ConvertTime(time.Date(2018, 04, 18, 0, 0, 0, 0, time.UTC)),
							"m",
							"localhost",
						},
					},
				},
				Err: errors.New("here is an error"),
			},
		},
		{
			name:          "single table with bad default tableID",
			encoderConfig: csv.DefaultEncoderConfig(),
			encoded: toCRLF(`#datatype,string,long,dateTime:RFC3339,dateTime:RFC3339,dateTime:RFC3339,string,string,double
#group,false,false,true,true,false,true,true,false
#default,_result,long,,,,,,
,result,table,_start,_stop,_time,_measurement,host,_value
,,0,2018-04-17T00:00:00Z,2018-04-17T00:05:00Z,2018-04-17T00:00:00Z,cpu,A,42
,,0,2018-04-17T00:00:00Z,2018-04-17T00:05:00Z,2018-04-17T00:00:01Z,cpu,A,43
`),
			result: &executetest.Result{
				Err: errors.New("failed to read metadata: default Table ID is not an integer"),
			},
		},
		{
			name:          "simple error message",
			encoderConfig: csv.DefaultEncoderConfig(),
			encoded: toCRLF(`#datatype,string,string
#group,true,true
#default,,
,error,reference
,failed to create physical plan: query must specify explicit yields when there is more than one result.,
`),
			result: &executetest.Result{
				Err: errors.New("failed to create physical plan: query must specify explicit yields when there is more than one result."),
			},
		},
		{
			name:          "csv with EOF",
			encoderConfig: csv.DefaultEncoderConfig(),
			encoded:       toCRLF(""),
			result: &executetest.Result{
				Err: errors.New("EOF"),
			},
		},
		{
			name:          "csv with no metadata",
			encoderConfig: csv.DefaultEncoderConfig(),
			encoded:       toCRLF(`1,2`),
			result: &executetest.Result{
				Err: errors.New("failed to read metadata: missing expected annotation datatype"),
			},
		},

		{
			name:          "single table with unknown annotations",
			encoderConfig: csv.DefaultEncoderConfig(),
			encoded: toCRLF(`#datatype,string,long,dateTime:RFC3339,dateTime:RFC3339,dateTime:RFC3339,string,string,double
#unsupported,,,,,,,,
#group,false,false,true,true,false,true,true,false
#default,_result,,,,,,,
,result,table,_start,_stop,_time,_measurement,host,_value
,,0,2018-04-17T00:00:00Z,2018-04-17T00:05:00Z,2018-04-17T00:00:00Z,cpu,A,42
,,0,2018-04-17T00:00:00Z,2018-04-17T00:05:00Z,2018-04-17T00:00:01Z,cpu,A,43
`),
			result: &executetest.Result{
				Nm: "_result",
				Tbls: []*executetest.Table{{
					KeyCols: []string{"_start", "_stop", "_measurement", "host"},
					ColMeta: []flux.ColMeta{
						{Label: "_start", Type: flux.TTime},
						{Label: "_stop", Type: flux.TTime},
						{Label: "_time", Type: flux.TTime},
						{Label: "_measurement", Type: flux.TString},
						{Label: "host", Type: flux.TString},
						{Label: "_value", Type: flux.TFloat},
					},
					Data: [][]interface{}{
						{
							values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 0, 0, time.UTC)),
							values.ConvertTime(time.Date(2018, 4, 17, 0, 5, 0, 0, time.UTC)),
							values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 0, 0, time.UTC)),
							"cpu",
							"A",
							42.0,
						},
						{
							values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 0, 0, time.UTC)),
							values.ConvertTime(time.Date(2018, 4, 17, 0, 5, 0, 0, time.UTC)),
							values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 1, 0, time.UTC)),
							"cpu",
							"A",
							43.0,
						},
					},
				}},
			},
		},
	}
	testCases = append(testCases, symmetricalTestCases...)
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if tc.skip {
				t.Skip()
			}
			decoder := csv.NewResultDecoder(tc.decoderConfig)
			result, err := decoder.Decode(bytes.NewReader(tc.encoded))
			if err != nil {
				if tc.result.Err != nil {
					if want, got := tc.result.Err.Error(), err.Error(); got != want {
						t.Error("unexpected error -want/+got", cmp.Diff(want, got))
					}
					return
				}
				t.Fatal(err)
			}
			got := &executetest.Result{
				Nm: result.Name(),
			}
			if err := result.Tables().Do(func(tbl flux.Table) error {
				cb, err := executetest.ConvertTable(tbl)
				if err != nil {
					return err
				}
				got.Tbls = append(got.Tbls, cb)
				return nil
			}); err != nil {
				if tc.result.Err == nil {
					t.Fatal(err)
				}
				got.Err = err
			}

			got.Normalize()
			tc.result.Normalize()

			cmpOpts := cmpopts.IgnoreFields(executetest.Result{}, "Err")
			if !cmp.Equal(got, tc.result, cmpOpts) {
				t.Error("unexpected results -want/+got", cmp.Diff(tc.result, got))
			}
			if (got.Err == nil) != (tc.result.Err == nil) {
				t.Errorf("error mismatch in result: -want/+got:\n- %q\n+ %q", tc.result.Err, got.Err)
			} else if got.Err != nil && got.Err.Error() != tc.result.Err.Error() {
				t.Errorf("unexpected error message: -want/+got:\n- %q\n+ %q", tc.result.Err, got.Err)
			}
		})
	}
}

func TestResultEncoder(t *testing.T) {
	testCases := []TestCase{
		// Add tests cases specific to encoding here
		{
			name:          "no annotations",
			encoderConfig: csv.ResultEncoderConfig{},
			encoded: toCRLF(`,result,table,_start,_stop,_time,_measurement,host,_value
,_result,0,2018-04-17T00:00:00Z,2018-04-17T00:05:00Z,2018-04-17T00:00:00Z,cpu,A,42
,_result,0,2018-04-17T00:00:00Z,2018-04-17T00:05:00Z,2018-04-17T00:00:01Z,cpu,A,43
`),
			result: &executetest.Result{
				Nm: "_result",
				Tbls: []*executetest.Table{{
					KeyCols: []string{"_start", "_stop", "_measurement", "host"},
					ColMeta: []flux.ColMeta{
						{Label: "_start", Type: flux.TTime},
						{Label: "_stop", Type: flux.TTime},
						{Label: "_time", Type: flux.TTime},
						{Label: "_measurement", Type: flux.TString},
						{Label: "host", Type: flux.TString},
						{Label: "_value", Type: flux.TFloat},
					},
					Data: [][]interface{}{
						{
							values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 0, 0, time.UTC)),
							values.ConvertTime(time.Date(2018, 4, 17, 0, 5, 0, 0, time.UTC)),
							values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 0, 0, time.UTC)),
							"cpu",
							"A",
							42.0,
						},
						{
							values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 0, 0, time.UTC)),
							values.ConvertTime(time.Date(2018, 4, 17, 0, 5, 0, 0, time.UTC)),
							values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 1, 0, time.UTC)),
							"cpu",
							"A",
							43.0,
						},
					},
				}},
			},
		},
		{
			name:          "no annotations, multiple tables",
			encoderConfig: csv.ResultEncoderConfig{},
			encoded: toCRLF(`,result,table,_start,_stop,_time,_measurement,host,_value
,_result,0,2018-04-17T00:00:00Z,2018-04-17T00:05:00Z,2018-04-17T00:00:00Z,cpu,A,42
,_result,0,2018-04-17T00:00:00Z,2018-04-17T00:05:00Z,2018-04-17T00:00:01Z,cpu,A,43
,_result,1,2018-04-17T00:00:00Z,2018-04-17T00:05:00Z,2018-04-17T00:00:00Z,mem,A,42
,_result,1,2018-04-17T00:00:00Z,2018-04-17T00:05:00Z,2018-04-17T00:00:01Z,mem,A,43
`),
			result: &executetest.Result{
				Nm: "_result",
				Tbls: []*executetest.Table{
					{
						KeyCols: []string{"_start", "_stop", "_measurement", "host"},
						ColMeta: []flux.ColMeta{
							{Label: "_start", Type: flux.TTime},
							{Label: "_stop", Type: flux.TTime},
							{Label: "_time", Type: flux.TTime},
							{Label: "_measurement", Type: flux.TString},
							{Label: "host", Type: flux.TString},
							{Label: "_value", Type: flux.TFloat},
						},
						Data: [][]interface{}{
							{
								values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 0, 0, time.UTC)),
								values.ConvertTime(time.Date(2018, 4, 17, 0, 5, 0, 0, time.UTC)),
								values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 0, 0, time.UTC)),
								"cpu",
								"A",
								42.0,
							},
							{
								values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 0, 0, time.UTC)),
								values.ConvertTime(time.Date(2018, 4, 17, 0, 5, 0, 0, time.UTC)),
								values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 1, 0, time.UTC)),
								"cpu",
								"A",
								43.0,
							},
						},
					},
					{
						KeyCols: []string{"_start", "_stop", "_measurement", "host"},
						ColMeta: []flux.ColMeta{
							{Label: "_start", Type: flux.TTime},
							{Label: "_stop", Type: flux.TTime},
							{Label: "_time", Type: flux.TTime},
							{Label: "_measurement", Type: flux.TString},
							{Label: "host", Type: flux.TString},
							{Label: "_value", Type: flux.TFloat},
						},
						Data: [][]interface{}{
							{
								values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 0, 0, time.UTC)),
								values.ConvertTime(time.Date(2018, 4, 17, 0, 5, 0, 0, time.UTC)),
								values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 0, 0, time.UTC)),
								"mem",
								"A",
								42.0,
							},
							{
								values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 0, 0, time.UTC)),
								values.ConvertTime(time.Date(2018, 4, 17, 0, 5, 0, 0, time.UTC)),
								values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 1, 0, time.UTC)),
								"mem",
								"A",
								43.0,
							},
						},
					},
				},
			},
		},
		{
			name: "table error",
			result: &executetest.Result{
				Nm: "_result",
				Tbls: []*executetest.Table{{
					Err: errors.New("test error"),
				}},
			},
			err: errors.New("test error"),
		},
	}
	testCases = append(testCases, symmetricalTestCases...)
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if tc.skip {
				t.Skip()
			}
			encoder := csv.NewResultEncoder(tc.encoderConfig)
			var got bytes.Buffer
			n, err := encoder.Encode(&got, tc.result)
			if err != nil {
				if tc.err == nil {
					t.Fatal(err)
				} else if g, w := err.Error(), tc.err.Error(); g != w {
					t.Errorf("unexpected error -want/+got:\n\t- %q\n\t+ %q", g, w)
				}
			}

			if g, w := got.String(), string(tc.encoded); g != w {
				t.Errorf("unexpected encoding -want/+got:\n%s", diff.LineDiff(w, g))
			}
			if g, w := n, int64(len(tc.encoded)); g != w {
				t.Errorf("unexpected encoding count -want/+got:\n%s", cmp.Diff(w, g))
			}
		})
	}
}

func TestMultiResultEncoder(t *testing.T) {
	testCases := []struct {
		name    string
		results flux.ResultIterator
		encoded []byte
		err     error
		config  csv.ResultEncoderConfig
	}{
		{
			name:   "single result",
			config: csv.DefaultEncoderConfig(),
			results: flux.NewSliceResultIterator([]flux.Result{&executetest.Result{
				Nm: "_result",
				Tbls: []*executetest.Table{{
					KeyCols: []string{"_start", "_stop", "_measurement", "host"},
					ColMeta: []flux.ColMeta{
						{Label: "_start", Type: flux.TTime},
						{Label: "_stop", Type: flux.TTime},
						{Label: "_time", Type: flux.TTime},
						{Label: "_measurement", Type: flux.TString},
						{Label: "host", Type: flux.TString},
						{Label: "_value", Type: flux.TFloat},
					},
					Data: [][]interface{}{
						{
							values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 0, 0, time.UTC)),
							values.ConvertTime(time.Date(2018, 4, 17, 0, 5, 0, 0, time.UTC)),
							values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 0, 0, time.UTC)),
							"cpu",
							"A",
							42.0,
						},
						{
							values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 0, 0, time.UTC)),
							values.ConvertTime(time.Date(2018, 4, 17, 0, 5, 0, 0, time.UTC)),
							values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 1, 0, time.UTC)),
							"cpu",
							"A",
							43.0,
						},
					},
				}},
			}}),
			encoded: toCRLF(`#datatype,string,long,dateTime:RFC3339,dateTime:RFC3339,dateTime:RFC3339,string,string,double
#group,false,false,true,true,false,true,true,false
#default,_result,,,,,,,
,result,table,_start,_stop,_time,_measurement,host,_value
,,0,2018-04-17T00:00:00Z,2018-04-17T00:05:00Z,2018-04-17T00:00:00Z,cpu,A,42
,,0,2018-04-17T00:00:00Z,2018-04-17T00:05:00Z,2018-04-17T00:00:01Z,cpu,A,43

`),
		},
		{
			name:   "empty result",
			config: csv.DefaultEncoderConfig(),
			results: flux.NewSliceResultIterator([]flux.Result{&executetest.Result{
				Nm: "_result",
				Tbls: []*executetest.Table{{
					KeyCols: []string{"_start", "_stop", "_measurement", "host"},
					KeyValues: []interface{}{
						values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 0, 0, time.UTC)),
						values.ConvertTime(time.Date(2018, 4, 17, 0, 5, 0, 0, time.UTC)),
						"cpu",
						"A",
					},
					ColMeta: []flux.ColMeta{
						{Label: "_start", Type: flux.TTime},
						{Label: "_stop", Type: flux.TTime},
						{Label: "_time", Type: flux.TTime},
						{Label: "_measurement", Type: flux.TString},
						{Label: "host", Type: flux.TString},
						{Label: "_value", Type: flux.TFloat},
					},
					Data: [][]interface{}{},
				}},
			}}),
			encoded: toCRLF(`#datatype,string,long,dateTime:RFC3339,dateTime:RFC3339,dateTime:RFC3339,string,string,double
#group,false,false,true,true,false,true,true,false
#default,_result,0,2018-04-17T00:00:00Z,2018-04-17T00:05:00Z,,cpu,A,
,result,table,_start,_stop,_time,_measurement,host,_value

`),
		},
		{
			name:   "two results",
			config: csv.DefaultEncoderConfig(),
			results: flux.NewSliceResultIterator([]flux.Result{
				&executetest.Result{
					Nm: "_result",
					Tbls: []*executetest.Table{{
						KeyCols: []string{"_start", "_stop", "_measurement", "host"},
						ColMeta: []flux.ColMeta{
							{Label: "_start", Type: flux.TTime},
							{Label: "_stop", Type: flux.TTime},
							{Label: "_time", Type: flux.TTime},
							{Label: "_measurement", Type: flux.TString},
							{Label: "host", Type: flux.TString},
							{Label: "_value", Type: flux.TFloat},
						},
						Data: [][]interface{}{
							{
								values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 0, 0, time.UTC)),
								values.ConvertTime(time.Date(2018, 4, 17, 0, 5, 0, 0, time.UTC)),
								values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 0, 0, time.UTC)),
								"cpu",
								"A",
								42.0,
							},
							{
								values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 0, 0, time.UTC)),
								values.ConvertTime(time.Date(2018, 4, 17, 0, 5, 0, 0, time.UTC)),
								values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 1, 0, time.UTC)),
								"cpu",
								"A",
								43.0,
							},
						},
					}},
				},
				&executetest.Result{
					Nm: "mean",
					Tbls: []*executetest.Table{{
						KeyCols: []string{"_start", "_stop", "_measurement", "host"},
						ColMeta: []flux.ColMeta{
							{Label: "_start", Type: flux.TTime},
							{Label: "_stop", Type: flux.TTime},
							{Label: "_time", Type: flux.TTime},
							{Label: "_measurement", Type: flux.TString},
							{Label: "host", Type: flux.TString},
							{Label: "_value", Type: flux.TFloat},
						},
						Data: [][]interface{}{
							{
								values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 0, 0, time.UTC)),
								values.ConvertTime(time.Date(2018, 4, 17, 0, 5, 0, 0, time.UTC)),
								values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 0, 0, time.UTC)),
								"cpu",
								"A",
								40.0,
							},
							{
								values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 0, 0, time.UTC)),
								values.ConvertTime(time.Date(2018, 4, 17, 0, 5, 0, 0, time.UTC)),
								values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 1, 0, time.UTC)),
								"cpu",
								"A",
								40.1,
							},
						},
					}},
				},
			}),
			encoded: toCRLF(`#datatype,string,long,dateTime:RFC3339,dateTime:RFC3339,dateTime:RFC3339,string,string,double
#group,false,false,true,true,false,true,true,false
#default,_result,,,,,,,
,result,table,_start,_stop,_time,_measurement,host,_value
,,0,2018-04-17T00:00:00Z,2018-04-17T00:05:00Z,2018-04-17T00:00:00Z,cpu,A,42
,,0,2018-04-17T00:00:00Z,2018-04-17T00:05:00Z,2018-04-17T00:00:01Z,cpu,A,43

#datatype,string,long,dateTime:RFC3339,dateTime:RFC3339,dateTime:RFC3339,string,string,double
#group,false,false,true,true,false,true,true,false
#default,mean,,,,,,,
,result,table,_start,_stop,_time,_measurement,host,_value
,,0,2018-04-17T00:00:00Z,2018-04-17T00:05:00Z,2018-04-17T00:00:00Z,cpu,A,40
,,0,2018-04-17T00:00:00Z,2018-04-17T00:05:00Z,2018-04-17T00:00:01Z,cpu,A,40.1

`),
		},
		{
			name:   "error results",
			config: csv.DefaultEncoderConfig(),
			results: flux.NewSliceResultIterator([]flux.Result{
				&executetest.Result{
					Nm: "mean",
					Tbls: []*executetest.Table{{
						KeyCols: []string{"_start", "_stop", "_measurement", "host"},
						ColMeta: []flux.ColMeta{
							{Label: "_start", Type: flux.TTime},
							{Label: "_stop", Type: flux.TTime},
							{Label: "_time", Type: flux.TTime},
							{Label: "_measurement", Type: flux.TString},
							{Label: "host", Type: flux.TString},
							{Label: "_value", Type: flux.TFloat},
						},
						Data: [][]interface{}{
							{
								values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 0, 0, time.UTC)),
								values.ConvertTime(time.Date(2018, 4, 17, 0, 5, 0, 0, time.UTC)),
								values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 0, 0, time.UTC)),
								"cpu",
								"A",
								40.0,
							},
							{
								values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 0, 0, time.UTC)),
								values.ConvertTime(time.Date(2018, 4, 17, 0, 5, 0, 0, time.UTC)),
								values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 1, 0, time.UTC)),
								"cpu",
								"A",
								40.1,
							},
						},
					}},
				},
				// errors only get encoded after something has been written to the encoder.
				&executetest.Result{
					Nm:  "test",
					Err: errors.New("test error"),
				},
			}),
			encoded: toCRLF(`#datatype,string,long,dateTime:RFC3339,dateTime:RFC3339,dateTime:RFC3339,string,string,double
#group,false,false,true,true,false,true,true,false
#default,mean,,,,,,,
,result,table,_start,_stop,_time,_measurement,host,_value
,,0,2018-04-17T00:00:00Z,2018-04-17T00:05:00Z,2018-04-17T00:00:00Z,cpu,A,40
,,0,2018-04-17T00:00:00Z,2018-04-17T00:05:00Z,2018-04-17T00:00:01Z,cpu,A,40.1


#datatype,string,string
#group,true,true
#default,,
,error,reference
,test error,
`),
		},
		{
			name:   "returns table errors",
			config: csv.DefaultEncoderConfig(),
			results: flux.NewSliceResultIterator([]flux.Result{&executetest.Result{
				Nm: "mean",
				Tbls: []*executetest.Table{{
					Err: errors.New("test error"),
				}},
			}}),
			encoded: nil,
			err:     errors.New("test error"),
		},
		{
			name:   "returns encoding errors",
			config: csv.DefaultEncoderConfig(),
			results: flux.NewSliceResultIterator([]flux.Result{&executetest.Result{
				Nm: "mean",
				Tbls: []*executetest.Table{{
					KeyCols: []string{"_start", "_stop", "_measurement", "host"},
					ColMeta: []flux.ColMeta{
						{Label: "_start", Type: flux.TTime},
						{Label: "_stop", Type: flux.TTime},
						{Label: "_time", Type: flux.TTime},
						{Label: "_measurement", Type: flux.TString},
						{Label: "host", Type: flux.TString},
						// Deliberately use invalid column type
						{Label: "_value", Type: flux.TInvalid},
					},
					Data: [][]interface{}{
						{
							values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 0, 0, time.UTC)),
							values.ConvertTime(time.Date(2018, 4, 17, 0, 5, 0, 0, time.UTC)),
							values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 0, 0, time.UTC)),
							"cpu",
							"A",
							40.0,
						},
					},
				}},
			}}),
			encoded: nil,
			err:     errors.New("csv encoder error: unknown column type invalid"),
		},
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			encoder := csv.NewMultiResultEncoder(tc.config)
			var got bytes.Buffer
			n, err := encoder.Encode(&got, tc.results)
			if err != nil && tc.err != nil {
				if err.Error() != tc.err.Error() {
					t.Errorf("unexpected error want: %s\n got: %s\n", tc.err.Error(), err.Error())
				}
			} else if err != nil {
				t.Errorf("unexpected error want: none\n got: %s\n", err.Error())
			} else if tc.err != nil {
				t.Errorf("unexpected error want: %s\n got: none", tc.err.Error())
			}

			if g, w := got.String(), string(tc.encoded); g != w {
				t.Errorf("unexpected encoding -want/+got:\n%s", diff.LineDiff(w, g))
			}
			if g, w := n, int64(len(tc.encoded)); g != w {
				t.Errorf("unexpected encoding count -want/+got:\n%s", cmp.Diff(w, g))
			}
		})
	}
}

func TestMultiResultDecoder(t *testing.T) {
	testCases := []struct {
		name    string
		config  csv.ResultDecoderConfig
		encoded []byte
		results []*executetest.Result
		err     error
	}{
		{
			name: "single result",
			encoded: toCRLF(`#datatype,string,long,dateTime:RFC3339,dateTime:RFC3339,dateTime:RFC3339,string,string,double
#group,false,false,true,true,false,true,true,false
#default,_result,0,2018-04-17T00:00:00Z,2018-04-17T00:05:00Z,,cpu,A,
,result,table,_start,_stop,_time,_measurement,host,_value
,,,,,2018-04-17T00:00:00Z,cpu,A,42.0
,,,,,2018-04-17T00:00:01Z,cpu,A,43.0

`),
			results: []*executetest.Result{{
				Nm: "_result",
				Tbls: []*executetest.Table{{
					KeyCols: []string{"_start", "_stop", "_measurement", "host"},
					ColMeta: []flux.ColMeta{
						{Label: "_start", Type: flux.TTime},
						{Label: "_stop", Type: flux.TTime},
						{Label: "_time", Type: flux.TTime},
						{Label: "_measurement", Type: flux.TString},
						{Label: "host", Type: flux.TString},
						{Label: "_value", Type: flux.TFloat},
					},
					Data: [][]interface{}{
						{
							values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 0, 0, time.UTC)),
							values.ConvertTime(time.Date(2018, 4, 17, 0, 5, 0, 0, time.UTC)),
							values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 0, 0, time.UTC)),
							"cpu",
							"A",
							42.0,
						},
						{
							values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 0, 0, time.UTC)),
							values.ConvertTime(time.Date(2018, 4, 17, 0, 5, 0, 0, time.UTC)),
							values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 1, 0, time.UTC)),
							"cpu",
							"A",
							43.0,
						},
					},
				}},
			}},
		},
		{
			name: "empty result",
			encoded: toCRLF(`#datatype,string,long,dateTime:RFC3339,dateTime:RFC3339,dateTime:RFC3339,string,string,double
#group,false,false,true,true,false,true,true,false
#default,_result,0,2018-04-17T00:00:00Z,2018-04-17T00:05:00Z,,cpu,A,
,result,table,_start,_stop,_time,_measurement,host,_value

`),
			results: []*executetest.Result{{
				Nm: "_result",
				Tbls: []*executetest.Table{{
					KeyCols: []string{"_start", "_stop", "_measurement", "host"},
					KeyValues: []interface{}{
						values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 0, 0, time.UTC)),
						values.ConvertTime(time.Date(2018, 4, 17, 0, 5, 0, 0, time.UTC)),
						"cpu",
						"A",
					},
					ColMeta: []flux.ColMeta{
						{Label: "_start", Type: flux.TTime},
						{Label: "_stop", Type: flux.TTime},
						{Label: "_time", Type: flux.TTime},
						{Label: "_measurement", Type: flux.TString},
						{Label: "host", Type: flux.TString},
						{Label: "_value", Type: flux.TFloat},
					},
					Data: [][]interface{}(nil),
				}},
			}},
		},
		{
			name: "two results",
			encoded: toCRLF(`#datatype,string,long,dateTime:RFC3339,dateTime:RFC3339,dateTime:RFC3339,string,string,double
#group,false,false,true,true,false,true,true,false
#default,_result,,,,,,,
,result,table,_start,_stop,_time,_measurement,host,_value
,,0,2018-04-17T00:00:00Z,2018-04-17T00:05:00Z,2018-04-17T00:00:00Z,cpu,A,42
,,0,2018-04-17T00:00:00Z,2018-04-17T00:05:00Z,2018-04-17T00:00:01Z,cpu,A,43

#datatype,string,long,dateTime:RFC3339,dateTime:RFC3339,dateTime:RFC3339,string,string,double
#group,false,false,true,true,false,true,true,false
#default,mean,,,,,,,
,result,table,_start,_stop,_time,_measurement,host,_value
,,0,2018-04-17T00:00:00Z,2018-04-17T00:05:00Z,2018-04-17T00:00:00Z,cpu,A,40
,,0,2018-04-17T00:00:00Z,2018-04-17T00:05:00Z,2018-04-17T00:00:01Z,cpu,A,40.1

`),
			results: []*executetest.Result{
				{
					Nm: "_result",
					Tbls: []*executetest.Table{{
						KeyCols: []string{"_start", "_stop", "_measurement", "host"},
						ColMeta: []flux.ColMeta{
							{Label: "_start", Type: flux.TTime},
							{Label: "_stop", Type: flux.TTime},
							{Label: "_time", Type: flux.TTime},
							{Label: "_measurement", Type: flux.TString},
							{Label: "host", Type: flux.TString},
							{Label: "_value", Type: flux.TFloat},
						},
						Data: [][]interface{}{
							{
								values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 0, 0, time.UTC)),
								values.ConvertTime(time.Date(2018, 4, 17, 0, 5, 0, 0, time.UTC)),
								values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 0, 0, time.UTC)),
								"cpu",
								"A",
								42.0,
							},
							{
								values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 0, 0, time.UTC)),
								values.ConvertTime(time.Date(2018, 4, 17, 0, 5, 0, 0, time.UTC)),
								values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 1, 0, time.UTC)),
								"cpu",
								"A",
								43.0,
							},
						},
					}},
				},
				{
					Nm: "mean",
					Tbls: []*executetest.Table{{
						KeyCols: []string{"_start", "_stop", "_measurement", "host"},
						ColMeta: []flux.ColMeta{
							{Label: "_start", Type: flux.TTime},
							{Label: "_stop", Type: flux.TTime},
							{Label: "_time", Type: flux.TTime},
							{Label: "_measurement", Type: flux.TString},
							{Label: "host", Type: flux.TString},
							{Label: "_value", Type: flux.TFloat},
						},
						Data: [][]interface{}{
							{
								values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 0, 0, time.UTC)),
								values.ConvertTime(time.Date(2018, 4, 17, 0, 5, 0, 0, time.UTC)),
								values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 0, 0, time.UTC)),
								"cpu",
								"A",
								40.0,
							},
							{
								values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 0, 0, time.UTC)),
								values.ConvertTime(time.Date(2018, 4, 17, 0, 5, 0, 0, time.UTC)),
								values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 1, 0, time.UTC)),
								"cpu",
								"A",
								40.1,
							},
						},
					}},
				},
			},
		},
		{
			name: "two empty results",
			encoded: toCRLF(`#datatype,string,long,dateTime:RFC3339,dateTime:RFC3339,dateTime:RFC3339,string,string,double
#group,false,false,true,true,false,true,true,false
#default,_result1,0,2018-04-17T00:00:00Z,2018-04-17T00:05:00Z,,cpu,A,
,result,table,_start,_stop,_time,_measurement,host,_value


#datatype,string,long,dateTime:RFC3339,dateTime:RFC3339,dateTime:RFC3339,string,string,double
#group,false,false,true,true,false,true,true,false
#default,_result2,0,2018-04-17T00:00:00Z,2018-04-17T00:05:00Z,,cpu-0,B,
,result,table,_start,_stop,_time,_measurement,host,_value


`),
			results: []*executetest.Result{{
				Nm: "_result1",
				Tbls: []*executetest.Table{{
					KeyCols: []string{"_start", "_stop", "_measurement", "host"},
					KeyValues: []interface{}{
						values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 0, 0, time.UTC)),
						values.ConvertTime(time.Date(2018, 4, 17, 0, 5, 0, 0, time.UTC)),
						"cpu",
						"A",
					},
					ColMeta: []flux.ColMeta{
						{Label: "_start", Type: flux.TTime},
						{Label: "_stop", Type: flux.TTime},
						{Label: "_time", Type: flux.TTime},
						{Label: "_measurement", Type: flux.TString},
						{Label: "host", Type: flux.TString},
						{Label: "_value", Type: flux.TFloat},
					},
					Data: [][]interface{}(nil),
				}},
			},
				{
					Nm: "_result2",
					Tbls: []*executetest.Table{{
						KeyCols: []string{"_start", "_stop", "_measurement", "host"},
						KeyValues: []interface{}{
							values.ConvertTime(time.Date(2018, 4, 17, 0, 0, 0, 0, time.UTC)),
							values.ConvertTime(time.Date(2018, 4, 17, 0, 5, 0, 0, time.UTC)),
							"cpu-0",
							"B",
						},
						ColMeta: []flux.ColMeta{
							{Label: "_start", Type: flux.TTime},
							{Label: "_stop", Type: flux.TTime},
							{Label: "_time", Type: flux.TTime},
							{Label: "_measurement", Type: flux.TString},
							{Label: "host", Type: flux.TString},
							{Label: "_value", Type: flux.TFloat},
						},
						Data: [][]interface{}(nil),
					}},
				}},
		},
		{
			name: "decodes errors",
			encoded: toCRLF(`#datatype,string,string
#group,true,true
#default,,
,error,reference
,test error,
`),
			err: errors.New("test error"),
		},
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			decoder := csv.NewMultiResultDecoder(tc.config)
			results, err := decoder.Decode(ioutil.NopCloser(bytes.NewReader(tc.encoded)))
			if err != nil {
				t.Fatal(err)
			}

			var got []*executetest.Result
			for results.More() {
				result := results.Next()
				res := &executetest.Result{
					Nm: result.Name(),
				}
				if err := result.Tables().Do(func(tbl flux.Table) error {
					cb, err := executetest.ConvertTable(tbl)
					if err != nil {
						return err
					}
					res.Tbls = append(res.Tbls, cb)
					return nil
				}); err != nil {
					t.Fatal(err)
				}
				res.Normalize()
				got = append(got, res)
			}

			if err := results.Err(); err != nil {
				if tc.err == nil {
					t.Errorf("unexpected error: %s", tc.err)
				} else if got, want := err.Error(), tc.err.Error(); got != want {
					t.Error("unexpected error -want/+got", cmp.Diff(want, got))
				}
			} else if tc.err != nil {
				t.Error("expected error")
			}

			// Normalize all of the tables for the test case.
			for _, result := range tc.results {
				result.Normalize()
			}

			if !cmp.Equal(got, tc.results) {
				t.Error("unexpected results -want/+got", cmp.Diff(tc.results, got))
			}
		})
	}
}

func TestTable(t *testing.T) {
	executetest.RunTableTests(t, executetest.TableTest{
		NewFn: func(ctx context.Context, alloc *memory.Allocator) flux.TableIterator {
			decoder := csv.NewResultDecoder(csv.ResultDecoderConfig{
				// Set this to a low value so we can have a table with
				// multiple buffers.
				MaxBufferCount: 5,
				Allocator:      alloc,
			})
			r, err := decoder.Decode(strings.NewReader(`#datatype,string,long,dateTime:RFC3339,string,double
#group,false,false,false,true,false
#default,_result,0,,A,
,result,table,_time,host,_value
,,,2018-04-17T00:00:00Z,,1.0
,,,2018-04-17T01:00:00Z,,2.0
,,,2018-04-17T02:00:00Z,,3.0
,,,2018-04-17T03:00:00Z,,4.0

#datatype,string,long,dateTime:RFC3339,string,double
#group,false,false,false,true,false
#default,_result,1,,B,
,result,table,_time,host,_value

#datatype,string,long,dateTime:RFC3339,string,double
#group,false,false,false,true,false
#default,_result,2,,C,
,result,table,_time,host,_value
,,,2018-04-17T00:00:00Z,,1.0
,,,2018-04-17T01:00:00Z,,2.0
,,,2018-04-17T02:00:00Z,,3.0
,,,2018-04-17T03:00:00Z,,4.0
,,,2018-04-17T04:00:00Z,,1.0
,,,2018-04-17T05:00:00Z,,2.0
,,,2018-04-17T06:00:00Z,,3.0
,,,2018-04-17T07:00:00Z,,4.0
`))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			return r.Tables()
		},
		IsDone: func(tbl flux.Table) bool {
			return tbl.(interface{ IsDone() bool }).IsDone()
		},
	})
}

func TestTable2(t *testing.T) {
	executetest.RunTableTests(t, executetest.TableTest{
		NewFn: func(ctx context.Context, alloc *memory.Allocator) flux.TableIterator {
			decoder := csv.NewResultDecoder(csv.ResultDecoderConfig{
				// Set this to a low value so we can have a table with
				// multiple buffers.
				MaxBufferCount: 5,
				Allocator:      alloc,
			})
			r, err := decoder.Decode(strings.NewReader(`#group,false,false,false,false,false,false,false,false,false,false,false,false,false,false,false,false,false
#datatype,string,long,string,dateTime:RFC3339,dateTime:RFC3339,dateTime:RFC3339,string,string,string,string,string,string,string,string,string,string,string
#default,_result,,,,,,,,,,,,,,,,
,result,table,_field,_start,_stop,_time,_value,env,error,errorCode,errorType,host,hostname,nodename,orgID,request,source
,,0,request,2020-09-02T12:00:00Z,2020-09-02T13:00:00Z,2020-09-02T12:51:01.660882959Z,"{""request"":{""authorization"":{""id"":""063fb56f630e2000"",""token"":"""",""status"":""active"",""description"":"""",""orgID"":""a2ec9780bc3b0e58"",""userID"":""061a208c88f50000"",""permissions"":[{""action"":""read"",""resource"":{""type"":""authorizations"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""authorizations"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""buckets"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""buckets"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""dashboards"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""dashboards"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""orgs"",""id"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""orgs"",""id"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""sources"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""sources"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""tasks"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""tasks"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""telegrafs"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""telegrafs"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""users"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""users"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""variables"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""variables"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""scrapers"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""scrapers"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""secrets"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""secrets"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""labels"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""labels"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""views"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""views"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""documents"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""documents"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""notificationRules"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""notificationRules"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""notificationEndpoints"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""notificationEndpoints"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""checks"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""checks"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""dbrp"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""dbrp"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""authorizations"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""authorizations"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""buckets"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""buckets"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""checks"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""checks"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""dashboards"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""dashboards"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""dbrp"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""dbrp"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""documents"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""documents"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""labels"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""labels"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""notificationRules"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""notificationRules"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""notificationEndpoints"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""notificationEndpoints"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""orgs"",""id"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""secrets"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""secrets"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""scrapers"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""scrapers"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""sources"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""sources"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""tasks"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""tasks"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""telegrafs"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""telegrafs"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""users"",""id"":""061a208c88f50000""}},{""action"":""write"",""resource"":{""type"":""users"",""id"":""061a208c88f50000""}},{""action"":""read"",""resource"":{""type"":""variables"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""variables"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""views"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""views"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""users"",""id"":""061a208c88f50000""}},{""action"":""write"",""resource"":{""type"":""users"",""id"":""061a208c88f50000""}}],""createdAt"":""0001-01-01T00:00:00Z"",""updatedAt"":""0001-01-01T00:00:00Z""},""organization_id"":""a2ec9780bc3b0e58"",""compiler"":{""Now"":""2020-09-02T12:51:01.65041241Z"",""extern"":{""type"":""File"",""package"":null,""imports"":null,""body"":[{""type"":""OptionStatement"",""assignment"":{""type"":""VariableAssignment"",""id"":{""type"":""Identifier"",""name"":""v""},""init"":{""type"":""ObjectExpression"",""properties"":[{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""timeRangeStart""},""value"":{""type"":""DateTimeLiteral"",""value"":""2014-12-31T23:00:00.000Z""}},{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""timeRangeStop""},""value"":{""type"":""DateTimeLiteral"",""value"":""2015-12-31T23:00:00.000Z""}},{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""windowPeriod""},""value"":{""type"":""DurationLiteral"",""values"":[{""magnitude"":87600000,""unit"":""ms""}]}}]}}}]},""query"":""from(bucket: \""ExtranetDumpCustomerTag\"")\n  |\u003e range(start: v.timeRangeStart, stop: v.timeRangeStop)\n  |\u003e filter(fn: (r) =\u003e r[\""_measurement\""] == \""EnergyValue\"")\n  |\u003e filter(fn: (r) =\u003e r[\""_field\""] == \""current1\"")\n  |\u003e filter(fn: (r) =\u003e r[\""customer\""] == '')\n  |\u003e aggregateWindow(every: 1d, fn: mean, createEmpty: false)\n  |\u003e yield(name: \""mean\"")""},""source"":""Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/84.0.4147.135 Safari/537.36"",""compiler_type"":""flux""},""dialect"":{""header"":true,""delimiter"":"","",""annotations"":[""group"",""datatype"",""default""]},""dialect_type"":""csv""}",prod01-eu-central-1,compilation failed: error at @5:13-5:42: invalid expression @5:41-5:42: ',invalid,user,queryd-v1-d6b75bfbc-5mpmf,queryd-v1-d6b75bfbc-5mpmf,ip-10-153-10-96.eu-central-1.compute.internal,a2ec9780bc3b0e58,"from(bucket: ""ExtranetDumpCustomerTag"")  |> range(start: v.timeRangeStart, stop: v.timeRangeStop)  |> filter(fn: (r) => r[""_measurement""] == ""EnergyValue"")  |> filter(fn: (r) => r[""_field""] == ""current1"")  |> filter(fn: (r) => r[""customer""] == '')  |> aggregateWindow(every: 1d, fn: mean, createEmpty: false)  |> yield(name: ""mean"")",Chrome
,,0,request,2020-09-02T12:00:00Z,2020-09-02T13:00:00Z,2020-09-02T12:20:49.452876991Z,"{""request"":{""authorization"":{""id"":""063fa8ce348c7000"",""token"":"""",""status"":""active"",""description"":"""",""orgID"":""a2ec9780bc3b0e58"",""userID"":""06199a135239f000"",""permissions"":[{""action"":""read"",""resource"":{""type"":""authorizations"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""authorizations"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""buckets"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""buckets"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""dashboards"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""dashboards"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""orgs"",""id"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""orgs"",""id"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""sources"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""sources"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""tasks"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""tasks"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""telegrafs"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""telegrafs"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""users"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""users"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""variables"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""variables"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""scrapers"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""scrapers"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""secrets"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""secrets"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""labels"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""labels"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""views"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""views"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""documents"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""documents"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""notificationRules"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""notificationRules"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""notificationEndpoints"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""notificationEndpoints"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""checks"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""checks"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""dbrp"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""dbrp"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""authorizations"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""authorizations"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""buckets"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""buckets"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""checks"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""checks"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""dashboards"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""dashboards"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""dbrp"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""dbrp"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""documents"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""documents"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""labels"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""labels"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""notificationRules"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""notificationRules"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""notificationEndpoints"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""notificationEndpoints"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""orgs"",""id"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""secrets"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""secrets"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""scrapers"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""scrapers"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""sources"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""sources"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""tasks"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""tasks"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""telegrafs"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""telegrafs"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""users"",""id"":""06199a135239f000""}},{""action"":""write"",""resource"":{""type"":""users"",""id"":""06199a135239f000""}},{""action"":""read"",""resource"":{""type"":""variables"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""variables"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""views"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""views"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""users"",""id"":""06199a135239f000""}},{""action"":""write"",""resource"":{""type"":""users"",""id"":""06199a135239f000""}}],""createdAt"":""0001-01-01T00:00:00Z"",""updatedAt"":""0001-01-01T00:00:00Z""},""organization_id"":""a2ec9780bc3b0e58"",""compiler"":{""Now"":""2020-09-02T12:20:49.439287155Z"",""extern"":{""type"":""File"",""package"":null,""imports"":null,""body"":[{""type"":""OptionStatement"",""assignment"":{""type"":""VariableAssignment"",""id"":{""type"":""Identifier"",""name"":""v""},""init"":{""type"":""ObjectExpression"",""properties"":[{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""timeRangeStart""},""value"":{""type"":""DateTimeLiteral"",""value"":""2015-02-09T10:38:00.000Z""}},{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""timeRangeStop""},""value"":{""type"":""DateTimeLiteral"",""value"":""2015-02-09T10:40:00.000Z""}},{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""windowPeriod""},""value"":{""type"":""DurationLiteral"",""values"":[{""magnitude"":333,""unit"":""ms""}]}}]}}}]},""query"":""from(bucket: \""ExtranetDumpCustomerTag\"")\n|\u003erange(start: \""2015-02-09 11:38:00.000\"", stop: \""2015-02-09 11:40:00.000\"")\n|\u003efilter(fn: (r) =\u003e r[\""_measurement\""] ==\""EnergyValue\"")\n|\u003eyield()\n\n""},""source"":""Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/85.0.4183.83 Safari/537.36"",""compiler_type"":""flux""},""dialect"":{""header"":true,""delimiter"":"","",""annotations"":[""group"",""datatype"",""default""]},""dialect_type"":""csv""}",prod01-eu-central-1,"error calling function ""yield"": error calling function ""filter"": error calling function ""range"": value is not a time, got string",invalid,user,queryd-v1-d6b75bfbc-2zlj8,queryd-v1-d6b75bfbc-2zlj8,ip-10-153-10-96.eu-central-1.compute.internal,a2ec9780bc3b0e58,"from(bucket: ""ExtranetDumpCustomerTag"")|>range(start: ""2015-02-09 11:38:00.000"", stop: ""2015-02-09 11:40:00.000"")|>filter(fn: (r) => r[""_measurement""] ==""EnergyValue"")|>yield()",Chrome
,,0,request,2020-09-02T12:00:00Z,2020-09-02T13:00:00Z,2020-09-02T12:20:50.155990686Z,"{""request"":{""authorization"":{""id"":""063fa8ce348c7000"",""token"":"""",""status"":""active"",""description"":"""",""orgID"":""a2ec9780bc3b0e58"",""userID"":""06199a135239f000"",""permissions"":[{""action"":""read"",""resource"":{""type"":""authorizations"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""authorizations"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""buckets"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""buckets"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""dashboards"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""dashboards"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""orgs"",""id"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""orgs"",""id"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""sources"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""sources"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""tasks"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""tasks"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""telegrafs"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""telegrafs"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""users"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""users"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""variables"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""variables"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""scrapers"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""scrapers"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""secrets"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""secrets"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""labels"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""labels"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""views"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""views"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""documents"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""documents"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""notificationRules"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""notificationRules"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""notificationEndpoints"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""notificationEndpoints"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""checks"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""checks"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""dbrp"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""dbrp"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""authorizations"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""authorizations"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""buckets"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""buckets"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""checks"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""checks"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""dashboards"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""dashboards"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""dbrp"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""dbrp"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""documents"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""documents"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""labels"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""labels"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""notificationRules"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""notificationRules"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""notificationEndpoints"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""notificationEndpoints"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""orgs"",""id"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""secrets"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""secrets"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""scrapers"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""scrapers"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""sources"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""sources"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""tasks"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""tasks"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""telegrafs"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""telegrafs"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""users"",""id"":""06199a135239f000""}},{""action"":""write"",""resource"":{""type"":""users"",""id"":""06199a135239f000""}},{""action"":""read"",""resource"":{""type"":""variables"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""variables"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""views"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""views"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""users"",""id"":""06199a135239f000""}},{""action"":""write"",""resource"":{""type"":""users"",""id"":""06199a135239f000""}}],""createdAt"":""0001-01-01T00:00:00Z"",""updatedAt"":""0001-01-01T00:00:00Z""},""organization_id"":""a2ec9780bc3b0e58"",""compiler"":{""Now"":""2020-09-02T12:20:50.141860031Z"",""extern"":{""type"":""File"",""package"":null,""imports"":null,""body"":[{""type"":""OptionStatement"",""assignment"":{""type"":""VariableAssignment"",""id"":{""type"":""Identifier"",""name"":""v""},""init"":{""type"":""ObjectExpression"",""properties"":[{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""timeRangeStart""},""value"":{""type"":""DateTimeLiteral"",""value"":""2015-02-09T10:38:00.000Z""}},{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""timeRangeStop""},""value"":{""type"":""DateTimeLiteral"",""value"":""2015-02-09T10:40:00.000Z""}},{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""windowPeriod""},""value"":{""type"":""DurationLiteral"",""values"":[{""magnitude"":333,""unit"":""ms""}]}}]}}}]},""query"":""from(bucket: \""ExtranetDumpCustomerTag\"")\n|\u003erange(start: \""2015-02-09 11:38:00.000\"", stop: \""2015-02-09 11:40:00.000\"")\n|\u003efilter(fn: (r) =\u003e r[\""_measurement\""] ==\""EnergyValue\"")\n|\u003eyield()\n\n""},""source"":""Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/85.0.4183.83 Safari/537.36"",""compiler_type"":""flux""},""dialect"":{""header"":true,""delimiter"":"","",""annotations"":[""group"",""datatype"",""default""]},""dialect_type"":""csv""}",prod01-eu-central-1,"error calling function ""yield"": error calling function ""filter"": error calling function ""range"": value is not a time, got string",invalid,user,queryd-v1-d6b75bfbc-frcxv,queryd-v1-d6b75bfbc-frcxv,ip-10-153-10-187.eu-central-1.compute.internal,a2ec9780bc3b0e58,"from(bucket: ""ExtranetDumpCustomerTag"")|>range(start: ""2015-02-09 11:38:00.000"", stop: ""2015-02-09 11:40:00.000"")|>filter(fn: (r) => r[""_measurement""] ==""EnergyValue"")|>yield()",Chrome
,,0,request,2020-09-02T12:00:00Z,2020-09-02T13:00:00Z,2020-09-02T12:20:47.816634703Z,"{""request"":{""authorization"":{""id"":""063fa8ce348c7000"",""token"":"""",""status"":""active"",""description"":"""",""orgID"":""a2ec9780bc3b0e58"",""userID"":""06199a135239f000"",""permissions"":[{""action"":""read"",""resource"":{""type"":""authorizations"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""authorizations"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""buckets"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""buckets"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""dashboards"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""dashboards"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""orgs"",""id"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""orgs"",""id"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""sources"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""sources"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""tasks"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""tasks"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""telegrafs"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""telegrafs"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""users"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""users"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""variables"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""variables"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""scrapers"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""scrapers"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""secrets"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""secrets"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""labels"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""labels"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""views"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""views"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""documents"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""documents"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""notificationRules"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""notificationRules"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""notificationEndpoints"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""notificationEndpoints"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""checks"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""checks"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""dbrp"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""dbrp"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""authorizations"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""authorizations"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""buckets"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""buckets"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""checks"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""checks"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""dashboards"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""dashboards"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""dbrp"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""dbrp"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""documents"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""documents"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""labels"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""labels"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""notificationRules"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""notificationRules"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""notificationEndpoints"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""notificationEndpoints"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""orgs"",""id"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""secrets"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""secrets"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""scrapers"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""scrapers"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""sources"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""sources"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""tasks"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""tasks"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""telegrafs"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""telegrafs"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""users"",""id"":""06199a135239f000""}},{""action"":""write"",""resource"":{""type"":""users"",""id"":""06199a135239f000""}},{""action"":""read"",""resource"":{""type"":""variables"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""variables"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""views"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""views"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""users"",""id"":""06199a135239f000""}},{""action"":""write"",""resource"":{""type"":""users"",""id"":""06199a135239f000""}}],""createdAt"":""0001-01-01T00:00:00Z"",""updatedAt"":""0001-01-01T00:00:00Z""},""organization_id"":""a2ec9780bc3b0e58"",""compiler"":{""Now"":""2020-09-02T12:20:47.802110236Z"",""extern"":{""type"":""File"",""package"":null,""imports"":null,""body"":[{""type"":""OptionStatement"",""assignment"":{""type"":""VariableAssignment"",""id"":{""type"":""Identifier"",""name"":""v""},""init"":{""type"":""ObjectExpression"",""properties"":[{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""timeRangeStart""},""value"":{""type"":""DateTimeLiteral"",""value"":""2015-02-09T10:38:00.000Z""}},{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""timeRangeStop""},""value"":{""type"":""DateTimeLiteral"",""value"":""2015-02-09T10:40:00.000Z""}},{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""windowPeriod""},""value"":{""type"":""DurationLiteral"",""values"":[{""magnitude"":333,""unit"":""ms""}]}}]}}}]},""query"":""from(bucket: \""ExtranetDumpCustomerTag\"")\n|\u003erange(start: \""2015-02-09 11:38:00.000\"", stop: \""2015-02-09 11:40:00.000\"")\n|\u003efilter(fn: (r) =\u003e r[\""_measurement\""] ==\""EnergyValue\"")\n|\u003eyield()\n\n""},""source"":""Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/85.0.4183.83 Safari/537.36"",""compiler_type"":""flux""},""dialect"":{""header"":true,""delimiter"":"","",""annotations"":[""group"",""datatype"",""default""]},""dialect_type"":""csv""}",prod01-eu-central-1,"error calling function ""yield"": error calling function ""filter"": error calling function ""range"": value is not a time, got string",invalid,user,queryd-v1-d6b75bfbc-gg655,queryd-v1-d6b75bfbc-gg655,ip-10-153-10-231.eu-central-1.compute.internal,a2ec9780bc3b0e58,"from(bucket: ""ExtranetDumpCustomerTag"")|>range(start: ""2015-02-09 11:38:00.000"", stop: ""2015-02-09 11:40:00.000"")|>filter(fn: (r) => r[""_measurement""] ==""EnergyValue"")|>yield()",Chrome
,,0,request,2020-09-02T12:00:00Z,2020-09-02T13:00:00Z,2020-09-02T12:20:29.660346479Z,"{""request"":{""authorization"":{""id"":""063fa8ce348c7000"",""token"":"""",""status"":""active"",""description"":"""",""orgID"":""a2ec9780bc3b0e58"",""userID"":""06199a135239f000"",""permissions"":[{""action"":""read"",""resource"":{""type"":""authorizations"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""authorizations"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""buckets"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""buckets"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""dashboards"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""dashboards"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""orgs"",""id"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""orgs"",""id"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""sources"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""sources"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""tasks"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""tasks"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""telegrafs"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""telegrafs"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""users"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""users"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""variables"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""variables"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""scrapers"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""scrapers"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""secrets"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""secrets"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""labels"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""labels"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""views"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""views"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""documents"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""documents"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""notificationRules"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""notificationRules"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""notificationEndpoints"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""notificationEndpoints"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""checks"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""checks"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""dbrp"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""dbrp"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""authorizations"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""authorizations"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""buckets"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""buckets"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""checks"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""checks"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""dashboards"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""dashboards"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""dbrp"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""dbrp"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""documents"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""documents"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""labels"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""labels"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""notificationRules"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""notificationRules"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""notificationEndpoints"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""notificationEndpoints"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""orgs"",""id"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""secrets"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""secrets"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""scrapers"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""scrapers"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""sources"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""sources"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""tasks"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""tasks"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""telegrafs"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""telegrafs"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""users"",""id"":""06199a135239f000""}},{""action"":""write"",""resource"":{""type"":""users"",""id"":""06199a135239f000""}},{""action"":""read"",""resource"":{""type"":""variables"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""variables"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""views"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""views"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""users"",""id"":""06199a135239f000""}},{""action"":""write"",""resource"":{""type"":""users"",""id"":""06199a135239f000""}}],""createdAt"":""0001-01-01T00:00:00Z"",""updatedAt"":""0001-01-01T00:00:00Z""},""organization_id"":""a2ec9780bc3b0e58"",""compiler"":{""Now"":""2020-09-02T12:20:29.646198981Z"",""extern"":{""type"":""File"",""package"":null,""imports"":null,""body"":[{""type"":""OptionStatement"",""assignment"":{""type"":""VariableAssignment"",""id"":{""type"":""Identifier"",""name"":""v""},""init"":{""type"":""ObjectExpression"",""properties"":[{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""timeRangeStart""},""value"":{""type"":""DateTimeLiteral"",""value"":""2015-02-09T10:38:00.000Z""}},{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""timeRangeStop""},""value"":{""type"":""DateTimeLiteral"",""value"":""2015-02-09T10:40:00.000Z""}},{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""windowPeriod""},""value"":{""type"":""DurationLiteral"",""values"":[{""magnitude"":333,""unit"":""ms""}]}}]}}}]},""query"":""from(bucket: \""ExtranetDumpCustomerTag\"")\n|\u003erange(start: \""2015-02-09 11:38:00.000\"", stop: \""2015-02-09 11:40:00.000\"")\n|\u003efilter(fn: (r) =\u003e r[\""_measurement\""] ==\""EnergyValue\"")\n|\u003eyield()\n\n""},""source"":""Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/85.0.4183.83 Safari/537.36"",""compiler_type"":""flux""},""dialect"":{""header"":true,""delimiter"":"","",""annotations"":[""group"",""datatype"",""default""]},""dialect_type"":""csv""}",prod01-eu-central-1,"error calling function ""yield"": error calling function ""filter"": error calling function ""range"": value is not a time, got string",invalid,user,queryd-v1-d6b75bfbc-nfg48,queryd-v1-d6b75bfbc-nfg48,ip-10-153-10-112.eu-central-1.compute.internal,a2ec9780bc3b0e58,"from(bucket: ""ExtranetDumpCustomerTag"")|>range(start: ""2015-02-09 11:38:00.000"", stop: ""2015-02-09 11:40:00.000"")|>filter(fn: (r) => r[""_measurement""] ==""EnergyValue"")|>yield()",Chrome
,,0,request,2020-09-02T12:00:00Z,2020-09-02T13:00:00Z,2020-09-02T12:04:19.641886579Z,"{""request"":{""authorization"":{""id"":""063faadc65bf7000"",""token"":"""",""status"":""active"",""description"":"""",""orgID"":""bae87e9ccb059b86"",""userID"":""06155b1cdae7c000"",""permissions"":[{""action"":""read"",""resource"":{""type"":""authorizations"",""orgID"":""bae87e9ccb059b86""}},{""action"":""write"",""resource"":{""type"":""authorizations"",""orgID"":""bae87e9ccb059b86""}},{""action"":""read"",""resource"":{""type"":""buckets"",""orgID"":""bae87e9ccb059b86""}},{""action"":""write"",""resource"":{""type"":""buckets"",""orgID"":""bae87e9ccb059b86""}},{""action"":""read"",""resource"":{""type"":""dashboards"",""orgID"":""bae87e9ccb059b86""}},{""action"":""write"",""resource"":{""type"":""dashboards"",""orgID"":""bae87e9ccb059b86""}},{""action"":""read"",""resource"":{""type"":""orgs"",""id"":""bae87e9ccb059b86""}},{""action"":""write"",""resource"":{""type"":""orgs"",""id"":""bae87e9ccb059b86""}},{""action"":""read"",""resource"":{""type"":""sources"",""orgID"":""bae87e9ccb059b86""}},{""action"":""write"",""resource"":{""type"":""sources"",""orgID"":""bae87e9ccb059b86""}},{""action"":""read"",""resource"":{""type"":""tasks"",""orgID"":""bae87e9ccb059b86""}},{""action"":""write"",""resource"":{""type"":""tasks"",""orgID"":""bae87e9ccb059b86""}},{""action"":""read"",""resource"":{""type"":""telegrafs"",""orgID"":""bae87e9ccb059b86""}},{""action"":""write"",""resource"":{""type"":""telegrafs"",""orgID"":""bae87e9ccb059b86""}},{""action"":""read"",""resource"":{""type"":""users"",""orgID"":""bae87e9ccb059b86""}},{""action"":""write"",""resource"":{""type"":""users"",""orgID"":""bae87e9ccb059b86""}},{""action"":""read"",""resource"":{""type"":""variables"",""orgID"":""bae87e9ccb059b86""}},{""action"":""write"",""resource"":{""type"":""variables"",""orgID"":""bae87e9ccb059b86""}},{""action"":""read"",""resource"":{""type"":""scrapers"",""orgID"":""bae87e9ccb059b86""}},{""action"":""write"",""resource"":{""type"":""scrapers"",""orgID"":""bae87e9ccb059b86""}},{""action"":""read"",""resource"":{""type"":""secrets"",""orgID"":""bae87e9ccb059b86""}},{""action"":""write"",""resource"":{""type"":""secrets"",""orgID"":""bae87e9ccb059b86""}},{""action"":""read"",""resource"":{""type"":""labels"",""orgID"":""bae87e9ccb059b86""}},{""action"":""write"",""resource"":{""type"":""labels"",""orgID"":""bae87e9ccb059b86""}},{""action"":""read"",""resource"":{""type"":""views"",""orgID"":""bae87e9ccb059b86""}},{""action"":""write"",""resource"":{""type"":""views"",""orgID"":""bae87e9ccb059b86""}},{""action"":""read"",""resource"":{""type"":""documents"",""orgID"":""bae87e9ccb059b86""}},{""action"":""write"",""resource"":{""type"":""documents"",""orgID"":""bae87e9ccb059b86""}},{""action"":""read"",""resource"":{""type"":""notificationRules"",""orgID"":""bae87e9ccb059b86""}},{""action"":""write"",""resource"":{""type"":""notificationRules"",""orgID"":""bae87e9ccb059b86""}},{""action"":""read"",""resource"":{""type"":""notificationEndpoints"",""orgID"":""bae87e9ccb059b86""}},{""action"":""write"",""resource"":{""type"":""notificationEndpoints"",""orgID"":""bae87e9ccb059b86""}},{""action"":""read"",""resource"":{""type"":""checks"",""orgID"":""bae87e9ccb059b86""}},{""action"":""write"",""resource"":{""type"":""checks"",""orgID"":""bae87e9ccb059b86""}},{""action"":""read"",""resource"":{""type"":""dbrp"",""orgID"":""bae87e9ccb059b86""}},{""action"":""write"",""resource"":{""type"":""dbrp"",""orgID"":""bae87e9ccb059b86""}},{""action"":""read"",""resource"":{""type"":""buckets"",""orgID"":""bae87e9ccb059b86""}},{""action"":""write"",""resource"":{""type"":""buckets"",""orgID"":""bae87e9ccb059b86""}},{""action"":""read"",""resource"":{""type"":""buckets"",""id"":""e8055ded433caff2"",""orgID"":""bae87e9ccb059b86""}},{""action"":""read"",""resource"":{""type"":""users"",""id"":""06155b1cdae7c000""}},{""action"":""write"",""resource"":{""type"":""users"",""id"":""06155b1cdae7c000""}}],""createdAt"":""0001-01-01T00:00:00Z"",""updatedAt"":""0001-01-01T00:00:00Z""},""organization_id"":""bae87e9ccb059b86"",""compiler"":{""Now"":""2020-09-02T12:04:19.588166657Z"",""extern"":{""type"":""File"",""package"":null,""imports"":null,""body"":[{""type"":""OptionStatement"",""assignment"":{""type"":""VariableAssignment"",""id"":{""type"":""Identifier"",""name"":""v""},""init"":{""type"":""ObjectExpression"",""properties"":[{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""Tæki""},""value"":{""type"":""StringLiteral"",""value"":""8""}},{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""timeRangeStart""},""value"":{""type"":""DateTimeLiteral"",""value"":""2020-08-25T10:58:54.000Z""}},{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""timeRangeStop""},""value"":{""type"":""DateTimeLiteral"",""value"":""2020-08-26T11:58:54.000Z""}},{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""windowPeriod""},""value"":{""type"":""DurationLiteral"",""values"":[{""magnitude"":250000,""unit"":""ms""}]}}]}}}]},""query"":""from(bucket: \""observations\"")\r\n  |\u003e range(start: v.timeRangeStart, stop: v.timeRangeStop)\r\n  |\u003e filter(fn: (r) =\u003e r[\""_measurement\""] == \""lkx\"" or r[\""_measurement\""] == \""weather\"")\r\n  |\u003e filter(fn: (r) =\u003e r[\""Alias\""] == \""LKX201-2\"")\r\n  |\u003e filter(fn: (r) =\u003e r[\""_field\""] == \""WindDirection\"")\r\n  |\u003e filter(fn: (r) =\u003e r[\""_value\""] \u003c= 359)\r\n  |\u003e aggregateWindow(every: v.windowPeriod, fn: median, createEmpty: false)\r\n  |\u003e yield(name: \""median\"")""},""source"":""Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/84.0.4147.135 Safari/537.36"",""compiler_type"":""flux""},""dialect"":{""header"":true,""delimiter"":"","",""annotations"":[""group"",""datatype"",""default""]},""dialect_type"":""csv""}",prod01-eu-central-1,unsupported aggregate column type int; unsupported aggregate column type int,invalid,user,queryd-v1-d6b75bfbc-cpd82,queryd-v1-d6b75bfbc-cpd82,ip-10-153-10-85.eu-central-1.compute.internal,bae87e9ccb059b86,"rom(bucket: ""observations"")\r  |> range(start: v.timeRangeStart, stop: v.timeRangeStop)\r  |> filter(fn: (r) => r[""_measurement""] == ""lkx"" or r[""_measurement""] == ""weather"")\r  |> filter(fn: (r) => r[""Alias""] == ""LKX201-2"")\r  |> filter(fn: (r) => r[""_field""] == ""WindDirection"")\r  |> filter(fn: (r) => r[""_value""] \u003c= 359)\r  |> aggregateWindow(every: v.windowPeriod, fn: median, createEmpty: false)\r  |> yield(name: ""median"")",Chrome
,,0,request,2020-09-02T12:00:00Z,2020-09-02T13:00:00Z,2020-09-02T12:01:25.234216651Z,"{""request"":{""authorization"":{""id"":""063faadc65bf7000"",""token"":"""",""status"":""active"",""description"":"""",""orgID"":""bae87e9ccb059b86"",""userID"":""06155b1cdae7c000"",""permissions"":[{""action"":""read"",""resource"":{""type"":""authorizations"",""orgID"":""bae87e9ccb059b86""}},{""action"":""write"",""resource"":{""type"":""authorizations"",""orgID"":""bae87e9ccb059b86""}},{""action"":""read"",""resource"":{""type"":""buckets"",""orgID"":""bae87e9ccb059b86""}},{""action"":""write"",""resource"":{""type"":""buckets"",""orgID"":""bae87e9ccb059b86""}},{""action"":""read"",""resource"":{""type"":""dashboards"",""orgID"":""bae87e9ccb059b86""}},{""action"":""write"",""resource"":{""type"":""dashboards"",""orgID"":""bae87e9ccb059b86""}},{""action"":""read"",""resource"":{""type"":""orgs"",""id"":""bae87e9ccb059b86""}},{""action"":""write"",""resource"":{""type"":""orgs"",""id"":""bae87e9ccb059b86""}},{""action"":""read"",""resource"":{""type"":""sources"",""orgID"":""bae87e9ccb059b86""}},{""action"":""write"",""resource"":{""type"":""sources"",""orgID"":""bae87e9ccb059b86""}},{""action"":""read"",""resource"":{""type"":""tasks"",""orgID"":""bae87e9ccb059b86""}},{""action"":""write"",""resource"":{""type"":""tasks"",""orgID"":""bae87e9ccb059b86""}},{""action"":""read"",""resource"":{""type"":""telegrafs"",""orgID"":""bae87e9ccb059b86""}},{""action"":""write"",""resource"":{""type"":""telegrafs"",""orgID"":""bae87e9ccb059b86""}},{""action"":""read"",""resource"":{""type"":""users"",""orgID"":""bae87e9ccb059b86""}},{""action"":""write"",""resource"":{""type"":""users"",""orgID"":""bae87e9ccb059b86""}},{""action"":""read"",""resource"":{""type"":""variables"",""orgID"":""bae87e9ccb059b86""}},{""action"":""write"",""resource"":{""type"":""variables"",""orgID"":""bae87e9ccb059b86""}},{""action"":""read"",""resource"":{""type"":""scrapers"",""orgID"":""bae87e9ccb059b86""}},{""action"":""write"",""resource"":{""type"":""scrapers"",""orgID"":""bae87e9ccb059b86""}},{""action"":""read"",""resource"":{""type"":""secrets"",""orgID"":""bae87e9ccb059b86""}},{""action"":""write"",""resource"":{""type"":""secrets"",""orgID"":""bae87e9ccb059b86""}},{""action"":""read"",""resource"":{""type"":""labels"",""orgID"":""bae87e9ccb059b86""}},{""action"":""write"",""resource"":{""type"":""labels"",""orgID"":""bae87e9ccb059b86""}},{""action"":""read"",""resource"":{""type"":""views"",""orgID"":""bae87e9ccb059b86""}},{""action"":""write"",""resource"":{""type"":""views"",""orgID"":""bae87e9ccb059b86""}},{""action"":""read"",""resource"":{""type"":""documents"",""orgID"":""bae87e9ccb059b86""}},{""action"":""write"",""resource"":{""type"":""documents"",""orgID"":""bae87e9ccb059b86""}},{""action"":""read"",""resource"":{""type"":""notificationRules"",""orgID"":""bae87e9ccb059b86""}},{""action"":""write"",""resource"":{""type"":""notificationRules"",""orgID"":""bae87e9ccb059b86""}},{""action"":""read"",""resource"":{""type"":""notificationEndpoints"",""orgID"":""bae87e9ccb059b86""}},{""action"":""write"",""resource"":{""type"":""notificationEndpoints"",""orgID"":""bae87e9ccb059b86""}},{""action"":""read"",""resource"":{""type"":""checks"",""orgID"":""bae87e9ccb059b86""}},{""action"":""write"",""resource"":{""type"":""checks"",""orgID"":""bae87e9ccb059b86""}},{""action"":""read"",""resource"":{""type"":""dbrp"",""orgID"":""bae87e9ccb059b86""}},{""action"":""write"",""resource"":{""type"":""dbrp"",""orgID"":""bae87e9ccb059b86""}},{""action"":""read"",""resource"":{""type"":""buckets"",""orgID"":""bae87e9ccb059b86""}},{""action"":""write"",""resource"":{""type"":""buckets"",""orgID"":""bae87e9ccb059b86""}},{""action"":""read"",""resource"":{""type"":""buckets"",""id"":""e8055ded433caff2"",""orgID"":""bae87e9ccb059b86""}},{""action"":""read"",""resource"":{""type"":""users"",""id"":""06155b1cdae7c000""}},{""action"":""write"",""resource"":{""type"":""users"",""id"":""06155b1cdae7c000""}}],""createdAt"":""0001-01-01T00:00:00Z"",""updatedAt"":""0001-01-01T00:00:00Z""},""organization_id"":""bae87e9ccb059b86"",""compiler"":{""Now"":""2020-09-02T12:01:25.175728724Z"",""extern"":{""type"":""File"",""package"":null,""imports"":null,""body"":[{""type"":""OptionStatement"",""assignment"":{""type"":""VariableAssignment"",""id"":{""type"":""Identifier"",""name"":""v""},""init"":{""type"":""ObjectExpression"",""properties"":[{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""Tæki""},""value"":{""type"":""StringLiteral"",""value"":""8""}},{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""timeRangeStart""},""value"":{""type"":""DateTimeLiteral"",""value"":""2020-08-25T10:58:54.000Z""}},{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""timeRangeStop""},""value"":{""type"":""DateTimeLiteral"",""value"":""2020-08-26T11:58:54.000Z""}},{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""windowPeriod""},""value"":{""type"":""DurationLiteral"",""values"":[{""magnitude"":250000,""unit"":""ms""}]}}]}}}]},""query"":""from(bucket: \""observations\"")\r\n  |\u003e range(start: v.timeRangeStart, stop: v.timeRangeStop)\r\n  |\u003e filter(fn: (r) =\u003e r[\""_measurement\""] == \""lkx\"" or r[\""_measurement\""] == \""weather\"")\r\n  |\u003e filter(fn: (r) =\u003e r[\""Alias\""] == \""LKX201-2\"")\r\n  |\u003e filter(fn: (r) =\u003e r[\""_field\""] == \""WindDirection\"")\r\n  |\u003e filter(fn: (r) =\u003e r[\""_value\""] \u003c= 359)\r\n  |\u003e aggregateWindow(every: v.windowPeriod, fn: median, createEmpty: false)\r\n  |\u003e yield(name: \""median\"")""},""source"":""Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/84.0.4147.135 Safari/537.36"",""compiler_type"":""flux""},""dialect"":{""header"":true,""delimiter"":"","",""annotations"":[""group"",""datatype"",""default""]},""dialect_type"":""csv""}",prod01-eu-central-1,unsupported aggregate column type int; unsupported aggregate column type int,invalid,user,queryd-v1-d6b75bfbc-z5cmc,queryd-v1-d6b75bfbc-z5cmc,ip-10-153-10-33.eu-central-1.compute.internal,bae87e9ccb059b86,"rom(bucket: ""observations"")\r  |> range(start: v.timeRangeStart, stop: v.timeRangeStop)\r  |> filter(fn: (r) => r[""_measurement""] == ""lkx"" or r[""_measurement""] == ""weather"")\r  |> filter(fn: (r) => r[""Alias""] == ""LKX201-2"")\r  |> filter(fn: (r) => r[""_field""] == ""WindDirection"")\r  |> filter(fn: (r) => r[""_value""] \u003c= 359)\r  |> aggregateWindow(every: v.windowPeriod, fn: median, createEmpty: false)\r  |> yield(name: ""median"")",Chrome
,,0,request,2020-09-02T12:00:00Z,2020-09-02T13:00:00Z,2020-09-02T12:04:50.121623581Z,"{""request"":{""authorization"":{""id"":""063fa524b3194000"",""token"":"""",""status"":""active"",""description"":"""",""orgID"":""a2ec9780bc3b0e58"",""userID"":""061a208c88f50000"",""permissions"":[{""action"":""read"",""resource"":{""type"":""authorizations"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""authorizations"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""buckets"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""buckets"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""dashboards"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""dashboards"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""orgs"",""id"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""orgs"",""id"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""sources"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""sources"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""tasks"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""tasks"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""telegrafs"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""telegrafs"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""users"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""users"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""variables"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""variables"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""scrapers"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""scrapers"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""secrets"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""secrets"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""labels"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""labels"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""views"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""views"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""documents"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""documents"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""notificationRules"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""notificationRules"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""notificationEndpoints"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""notificationEndpoints"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""checks"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""checks"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""dbrp"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""dbrp"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""authorizations"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""authorizations"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""buckets"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""buckets"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""checks"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""checks"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""dashboards"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""dashboards"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""dbrp"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""dbrp"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""documents"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""documents"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""labels"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""labels"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""notificationRules"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""notificationRules"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""notificationEndpoints"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""notificationEndpoints"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""orgs"",""id"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""secrets"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""secrets"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""scrapers"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""scrapers"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""sources"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""sources"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""tasks"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""tasks"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""telegrafs"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""telegrafs"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""users"",""id"":""061a208c88f50000""}},{""action"":""write"",""resource"":{""type"":""users"",""id"":""061a208c88f50000""}},{""action"":""read"",""resource"":{""type"":""variables"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""variables"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""views"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""views"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""users"",""id"":""061a208c88f50000""}},{""action"":""write"",""resource"":{""type"":""users"",""id"":""061a208c88f50000""}}],""createdAt"":""0001-01-01T00:00:00Z"",""updatedAt"":""0001-01-01T00:00:00Z""},""organization_id"":""a2ec9780bc3b0e58"",""compiler"":{""Now"":""2020-09-02T12:04:50.098690047Z"",""extern"":{""type"":""File"",""package"":null,""imports"":null,""body"":[{""type"":""OptionStatement"",""assignment"":{""type"":""VariableAssignment"",""id"":{""type"":""Identifier"",""name"":""v""},""init"":{""type"":""ObjectExpression"",""properties"":[{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""timeRangeStart""},""value"":{""type"":""DateTimeLiteral"",""value"":""2015-02-09T10:38:00.000Z""}},{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""timeRangeStop""},""value"":{""type"":""DateTimeLiteral"",""value"":""2015-02-09T10:40:00.000Z""}},{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""windowPeriod""},""value"":{""type"":""DurationLiteral"",""values"":[{""magnitude"":333,""unit"":""ms""}]}}]}}}]},""query"":""from(bucket: \""ExtranetDumpCustomerTag\"")\n  |\u003e range(start: v.timeRangeStart, stop: v.timeRangeStop)\n  |\u003e filter(fn: (r) =\u003e r[\""_measurement\""] == \""WeatherValue\"")\n  |\u003e aggregateWindow(every: v.windowPeriod, fn: mean, createEmpty: false)\n  |\u003e yield(name: \""mean\"")""},""source"":""Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_4) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/85.0.4183.83 Safari/537.36"",""compiler_type"":""flux""},""dialect"":{""header"":true,""delimiter"":"","",""annotations"":[""group"",""datatype"",""default""]},""dialect_type"":""csv""}",prod01-eu-central-1,unsupported input type for mean aggregate: boolean; unsupported input type for mean aggregate: boolean,invalid,user,queryd-v1-d6b75bfbc-6wjbm,queryd-v1-d6b75bfbc-6wjbm,ip-10-153-10-118.eu-central-1.compute.internal,a2ec9780bc3b0e58,"from(bucket: ""ExtranetDumpCustomerTag"")  |> range(start: v.timeRangeStart, stop: v.timeRangeStop)  |> filter(fn: (r) => r[""_measurement""] == ""WeatherValue"")  |> aggregateWindow(every: v.windowPeriod, fn: mean, createEmpty: false)  |> yield(name: ""mean"")",Chrome
,,0,request,2020-09-02T12:00:00Z,2020-09-02T13:00:00Z,2020-09-02T12:05:02.640381338Z,"{""request"":{""authorization"":{""id"":""063fa524b3194000"",""token"":"""",""status"":""active"",""description"":"""",""orgID"":""a2ec9780bc3b0e58"",""userID"":""061a208c88f50000"",""permissions"":[{""action"":""read"",""resource"":{""type"":""authorizations"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""authorizations"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""buckets"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""buckets"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""dashboards"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""dashboards"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""orgs"",""id"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""orgs"",""id"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""sources"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""sources"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""tasks"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""tasks"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""telegrafs"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""telegrafs"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""users"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""users"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""variables"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""variables"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""scrapers"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""scrapers"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""secrets"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""secrets"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""labels"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""labels"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""views"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""views"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""documents"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""documents"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""notificationRules"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""notificationRules"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""notificationEndpoints"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""notificationEndpoints"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""checks"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""checks"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""dbrp"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""dbrp"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""authorizations"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""authorizations"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""buckets"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""buckets"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""checks"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""checks"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""dashboards"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""dashboards"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""dbrp"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""dbrp"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""documents"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""documents"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""labels"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""labels"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""notificationRules"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""notificationRules"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""notificationEndpoints"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""notificationEndpoints"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""orgs"",""id"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""secrets"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""secrets"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""scrapers"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""scrapers"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""sources"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""sources"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""tasks"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""tasks"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""telegrafs"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""telegrafs"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""users"",""id"":""061a208c88f50000""}},{""action"":""write"",""resource"":{""type"":""users"",""id"":""061a208c88f50000""}},{""action"":""read"",""resource"":{""type"":""variables"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""variables"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""views"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""write"",""resource"":{""type"":""views"",""orgID"":""a2ec9780bc3b0e58""}},{""action"":""read"",""resource"":{""type"":""users"",""id"":""061a208c88f50000""}},{""action"":""write"",""resource"":{""type"":""users"",""id"":""061a208c88f50000""}}],""createdAt"":""0001-01-01T00:00:00Z"",""updatedAt"":""0001-01-01T00:00:00Z""},""organization_id"":""a2ec9780bc3b0e58"",""compiler"":{""Now"":""2020-09-02T12:05:02.620536688Z"",""extern"":{""type"":""File"",""package"":null,""imports"":null,""body"":[{""type"":""OptionStatement"",""assignment"":{""type"":""VariableAssignment"",""id"":{""type"":""Identifier"",""name"":""v""},""init"":{""type"":""ObjectExpression"",""properties"":[{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""timeRangeStart""},""value"":{""type"":""DateTimeLiteral"",""value"":""2015-02-09T10:38:00.000Z""}},{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""timeRangeStop""},""value"":{""type"":""DateTimeLiteral"",""value"":""2015-02-09T10:40:00.000Z""}},{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""windowPeriod""},""value"":{""type"":""DurationLiteral"",""values"":[{""magnitude"":333,""unit"":""ms""}]}}]}}}]},""query"":""from(bucket: \""ExtranetDumpCustomerTag\"")\n  |\u003e range(start: v.timeRangeStart, stop: v.timeRangeStop)\n  |\u003e filter(fn: (r) =\u003e r[\""_measurement\""] == \""WeatherValue\"")\n  |\u003e aggregateWindow(every: v.windowPeriod, fn: mean, createEmpty: false)\n  |\u003e yield(name: \""mean\"")""},""source"":""Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_4) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/85.0.4183.83 Safari/537.36"",""compiler_type"":""flux""},""dialect"":{""header"":true,""delimiter"":"","",""annotations"":[""group"",""datatype"",""default""]},""dialect_type"":""csv""}",prod01-eu-central-1,unsupported input type for mean aggregate: boolean; unsupported input type for mean aggregate: boolean,invalid,user,queryd-v1-d6b75bfbc-kznb5,queryd-v1-d6b75bfbc-kznb5,ip-10-153-10-149.eu-central-1.compute.internal,a2ec9780bc3b0e58,"from(bucket: ""ExtranetDumpCustomerTag"")  |> range(start: v.timeRangeStart, stop: v.timeRangeStop)  |> filter(fn: (r) => r[""_measurement""] == ""WeatherValue"")  |> aggregateWindow(every: v.windowPeriod, fn: mean, createEmpty: false)  |> yield(name: ""mean"")",Chrome
,,0,request,2020-09-02T12:00:00Z,2020-09-02T13:00:00Z,2020-09-02T12:03:30.69488328Z,"{""request"":{""authorization"":{""id"":""063f9b5ac6045000"",""token"":"""",""status"":""active"",""description"":"""",""orgID"":""dc56bb3c07ec663c"",""userID"":""063f9b4954d66000"",""permissions"":[{""action"":""read"",""resource"":{""type"":""authorizations"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""authorizations"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""buckets"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""buckets"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""dashboards"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""dashboards"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""orgs"",""id"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""orgs"",""id"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""sources"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""sources"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""tasks"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""tasks"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""telegrafs"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""telegrafs"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""users"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""users"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""variables"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""variables"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""scrapers"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""scrapers"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""secrets"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""secrets"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""labels"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""labels"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""views"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""views"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""documents"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""documents"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""notificationRules"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""notificationRules"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""notificationEndpoints"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""notificationEndpoints"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""checks"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""checks"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""dbrp"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""dbrp"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""buckets"",""id"":""2d96370493e32279"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""telegrafs"",""id"":""063f9d793b166000"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""users"",""id"":""063f9b4954d66000""}},{""action"":""write"",""resource"":{""type"":""users"",""id"":""063f9b4954d66000""}}],""createdAt"":""0001-01-01T00:00:00Z"",""updatedAt"":""0001-01-01T00:00:00Z""},""organization_id"":""dc56bb3c07ec663c"",""compiler"":{""Now"":""2020-09-02T12:03:30.679008917Z"",""extern"":{""type"":""File"",""package"":null,""imports"":null,""body"":[{""type"":""OptionStatement"",""assignment"":{""type"":""VariableAssignment"",""id"":{""type"":""Identifier"",""name"":""v""},""init"":{""type"":""ObjectExpression"",""properties"":[{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""bucket""},""value"":{""type"":""StringLiteral"",""value"":""""}},{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""timeRangeStart""},""value"":{""type"":""UnaryExpression"",""operator"":""-"",""argument"":{""type"":""DurationLiteral"",""values"":[{""magnitude"":5,""unit"":""m""}]}}},{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""timeRangeStop""},""value"":{""type"":""CallExpression"",""callee"":{""type"":""Identifier"",""name"":""now""}}}]}}}]},""query"":""SELECT * from MyDB""},""source"":""Mozilla/5.0 (Windows NT 6.1; Win64; x64; rv:80.0) Gecko/20100101 Firefox/80.0"",""compiler_type"":""flux""},""dialect"":{""header"":true,""delimiter"":"","",""annotations"":[""group"",""datatype"",""default""]},""dialect_type"":""csv""}",prod01-eu-west-1,error @1:1-1:7: undefined identifier SELECT,invalid,user,queryd-v1-668bbdcf74-5m4n2,queryd-v1-668bbdcf74-5m4n2,aks-storage-23576596-vmss00000u,dc56bb3c07ec663c,SELECT * from MyDB,Firefox
,,0,request,2020-09-02T12:00:00Z,2020-09-02T13:00:00Z,2020-09-02T12:04:05.242175258Z,"{""request"":{""authorization"":{""id"":""063f9b5ac6045000"",""token"":"""",""status"":""active"",""description"":"""",""orgID"":""dc56bb3c07ec663c"",""userID"":""063f9b4954d66000"",""permissions"":[{""action"":""read"",""resource"":{""type"":""authorizations"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""authorizations"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""buckets"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""buckets"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""dashboards"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""dashboards"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""orgs"",""id"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""orgs"",""id"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""sources"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""sources"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""tasks"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""tasks"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""telegrafs"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""telegrafs"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""users"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""users"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""variables"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""variables"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""scrapers"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""scrapers"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""secrets"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""secrets"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""labels"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""labels"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""views"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""views"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""documents"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""documents"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""notificationRules"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""notificationRules"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""notificationEndpoints"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""notificationEndpoints"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""checks"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""checks"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""dbrp"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""dbrp"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""buckets"",""id"":""2d96370493e32279"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""telegrafs"",""id"":""063f9d793b166000"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""users"",""id"":""063f9b4954d66000""}},{""action"":""write"",""resource"":{""type"":""users"",""id"":""063f9b4954d66000""}}],""createdAt"":""0001-01-01T00:00:00Z"",""updatedAt"":""0001-01-01T00:00:00Z""},""organization_id"":""dc56bb3c07ec663c"",""compiler"":{""Now"":""2020-09-02T12:04:05.228838167Z"",""extern"":{""type"":""File"",""package"":null,""imports"":null,""body"":[{""type"":""OptionStatement"",""assignment"":{""type"":""VariableAssignment"",""id"":{""type"":""Identifier"",""name"":""v""},""init"":{""type"":""ObjectExpression"",""properties"":[{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""bucket""},""value"":{""type"":""StringLiteral"",""value"":""""}},{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""timeRangeStart""},""value"":{""type"":""UnaryExpression"",""operator"":""-"",""argument"":{""type"":""DurationLiteral"",""values"":[{""magnitude"":5,""unit"":""m""}]}}},{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""timeRangeStop""},""value"":{""type"":""CallExpression"",""callee"":{""type"":""Identifier"",""name"":""now""}}}]}}}]},""query"":""SELECT * from MyDB""},""source"":""Mozilla/5.0 (Windows NT 6.1; Win64; x64; rv:80.0) Gecko/20100101 Firefox/80.0"",""compiler_type"":""flux""},""dialect"":{""header"":true,""delimiter"":"","",""annotations"":[""group"",""datatype"",""default""]},""dialect_type"":""csv""}",prod01-eu-west-1,error @1:1-1:7: undefined identifier SELECT,invalid,user,queryd-v1-668bbdcf74-lszvb,queryd-v1-668bbdcf74-lszvb,aks-storage-23576596-vmss00000q,dc56bb3c07ec663c,SELECT * from MyDB,Firefox
,,0,request,2020-09-02T12:00:00Z,2020-09-02T13:00:00Z,2020-09-02T12:03:17.913129835Z,"{""request"":{""authorization"":{""id"":""063f9b5ac6045000"",""token"":"""",""status"":""active"",""description"":"""",""orgID"":""dc56bb3c07ec663c"",""userID"":""063f9b4954d66000"",""permissions"":[{""action"":""read"",""resource"":{""type"":""authorizations"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""authorizations"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""buckets"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""buckets"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""dashboards"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""dashboards"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""orgs"",""id"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""orgs"",""id"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""sources"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""sources"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""tasks"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""tasks"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""telegrafs"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""telegrafs"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""users"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""users"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""variables"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""variables"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""scrapers"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""scrapers"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""secrets"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""secrets"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""labels"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""labels"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""views"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""views"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""documents"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""documents"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""notificationRules"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""notificationRules"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""notificationEndpoints"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""notificationEndpoints"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""checks"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""checks"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""dbrp"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""dbrp"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""buckets"",""id"":""2d96370493e32279"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""telegrafs"",""id"":""063f9d793b166000"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""users"",""id"":""063f9b4954d66000""}},{""action"":""write"",""resource"":{""type"":""users"",""id"":""063f9b4954d66000""}}],""createdAt"":""0001-01-01T00:00:00Z"",""updatedAt"":""0001-01-01T00:00:00Z""},""organization_id"":""dc56bb3c07ec663c"",""compiler"":{""Now"":""2020-09-02T12:03:17.899652098Z"",""extern"":{""type"":""File"",""package"":null,""imports"":null,""body"":[{""type"":""OptionStatement"",""assignment"":{""type"":""VariableAssignment"",""id"":{""type"":""Identifier"",""name"":""v""},""init"":{""type"":""ObjectExpression"",""properties"":[{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""bucket""},""value"":{""type"":""StringLiteral"",""value"":""""}},{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""timeRangeStart""},""value"":{""type"":""UnaryExpression"",""operator"":""-"",""argument"":{""type"":""DurationLiteral"",""values"":[{""magnitude"":5,""unit"":""m""}]}}},{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""timeRangeStop""},""value"":{""type"":""CallExpression"",""callee"":{""type"":""Identifier"",""name"":""now""}}}]}}}]},""query"":""select * from MyDB""},""source"":""Mozilla/5.0 (Windows NT 6.1; Win64; x64; rv:80.0) Gecko/20100101 Firefox/80.0"",""compiler_type"":""flux""},""dialect"":{""header"":true,""delimiter"":"","",""annotations"":[""group"",""datatype"",""default""]},""dialect_type"":""csv""}",prod01-eu-west-1,error @1:1-1:7: undefined identifier select,invalid,user,queryd-v1-668bbdcf74-xdfbp,queryd-v1-668bbdcf74-xdfbp,aks-storage-23576596-vmss00000n,dc56bb3c07ec663c,select * from MyDB,Firefox
,,0,request,2020-09-02T12:00:00Z,2020-09-02T13:00:00Z,2020-09-02T12:20:23.393285634Z,"{""request"":{""authorization"":{""id"":""063f9b5ac6045000"",""token"":"""",""status"":""active"",""description"":"""",""orgID"":""dc56bb3c07ec663c"",""userID"":""063f9b4954d66000"",""permissions"":[{""action"":""read"",""resource"":{""type"":""authorizations"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""authorizations"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""buckets"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""buckets"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""dashboards"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""dashboards"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""orgs"",""id"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""orgs"",""id"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""sources"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""sources"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""tasks"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""tasks"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""telegrafs"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""telegrafs"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""users"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""users"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""variables"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""variables"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""scrapers"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""scrapers"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""secrets"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""secrets"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""labels"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""labels"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""views"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""views"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""documents"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""documents"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""notificationRules"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""notificationRules"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""notificationEndpoints"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""notificationEndpoints"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""checks"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""checks"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""dbrp"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""dbrp"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""buckets"",""id"":""2d96370493e32279"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""telegrafs"",""id"":""063f9d793b166000"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""users"",""id"":""063f9b4954d66000""}},{""action"":""write"",""resource"":{""type"":""users"",""id"":""063f9b4954d66000""}}],""createdAt"":""0001-01-01T00:00:00Z"",""updatedAt"":""0001-01-01T00:00:00Z""},""organization_id"":""dc56bb3c07ec663c"",""compiler"":{""Now"":""2020-09-02T12:20:23.375534145Z"",""extern"":{""type"":""File"",""package"":null,""imports"":null,""body"":[{""type"":""OptionStatement"",""assignment"":{""type"":""VariableAssignment"",""id"":{""type"":""Identifier"",""name"":""v""},""init"":{""type"":""ObjectExpression"",""properties"":[{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""bucket""},""value"":{""type"":""StringLiteral"",""value"":""""}},{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""timeRangeStart""},""value"":{""type"":""UnaryExpression"",""operator"":""-"",""argument"":{""type"":""DurationLiteral"",""values"":[{""magnitude"":1,""unit"":""h""}]}}},{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""timeRangeStop""},""value"":{""type"":""CallExpression"",""callee"":{""type"":""Identifier"",""name"":""now""}}},{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""windowPeriod""},""value"":{""type"":""DurationLiteral"",""values"":[{""magnitude"":10000,""unit"":""ms""}]}}]}}}]},""query"":""sensorMetrics = from(bucket: \""MyDB\"")\r\n  |\u003e range(start: -1h)\r\n  |\u003e filter(fn: (r) =\u003e r._measurement == \""mem\"")""},""source"":""Mozilla/5.0 (Windows NT 6.1; Win64; x64; rv:80.0) Gecko/20100101 Firefox/80.0"",""compiler_type"":""flux""},""dialect"":{""header"":true,""delimiter"":"","",""annotations"":[""group"",""datatype"",""default""]},""dialect_type"":""csv""}",prod01-eu-west-1,"error in query specification while starting program: this Flux script returns no streaming data. Consider adding a ""yield"" or invoking streaming functions directly, without performing an assignment",invalid,user,queryd-v1-668bbdcf74-82gb4,queryd-v1-668bbdcf74-82gb4,aks-storage-23576596-vmss00000o,dc56bb3c07ec663c,"sensorMetrics = from(bucket: ""MyDB"")\r  |> range(start: -1h)\r  |> filter(fn: (r) => r._measurement == ""mem"")",Firefox
,,0,request,2020-09-02T12:00:00Z,2020-09-02T13:00:00Z,2020-09-02T12:21:48.795361454Z,"{""request"":{""authorization"":{""id"":""063f9b5ac6045000"",""token"":"""",""status"":""active"",""description"":"""",""orgID"":""dc56bb3c07ec663c"",""userID"":""063f9b4954d66000"",""permissions"":[{""action"":""read"",""resource"":{""type"":""authorizations"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""authorizations"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""buckets"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""buckets"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""dashboards"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""dashboards"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""orgs"",""id"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""orgs"",""id"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""sources"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""sources"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""tasks"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""tasks"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""telegrafs"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""telegrafs"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""users"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""users"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""variables"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""variables"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""scrapers"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""scrapers"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""secrets"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""secrets"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""labels"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""labels"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""views"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""views"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""documents"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""documents"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""notificationRules"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""notificationRules"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""notificationEndpoints"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""notificationEndpoints"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""checks"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""checks"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""dbrp"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""dbrp"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""buckets"",""id"":""2d96370493e32279"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""telegrafs"",""id"":""063f9d793b166000"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""users"",""id"":""063f9b4954d66000""}},{""action"":""write"",""resource"":{""type"":""users"",""id"":""063f9b4954d66000""}}],""createdAt"":""0001-01-01T00:00:00Z"",""updatedAt"":""0001-01-01T00:00:00Z""},""organization_id"":""dc56bb3c07ec663c"",""compiler"":{""Now"":""2020-09-02T12:21:48.779275132Z"",""extern"":{""type"":""File"",""package"":null,""imports"":null,""body"":[{""type"":""OptionStatement"",""assignment"":{""type"":""VariableAssignment"",""id"":{""type"":""Identifier"",""name"":""v""},""init"":{""type"":""ObjectExpression"",""properties"":[{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""bucket""},""value"":{""type"":""StringLiteral"",""value"":""""}},{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""timeRangeStart""},""value"":{""type"":""UnaryExpression"",""operator"":""-"",""argument"":{""type"":""DurationLiteral"",""values"":[{""magnitude"":15,""unit"":""m""}]}}},{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""timeRangeStop""},""value"":{""type"":""CallExpression"",""callee"":{""type"":""Identifier"",""name"":""now""}}},{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""windowPeriod""},""value"":{""type"":""DurationLiteral"",""values"":[{""magnitude"":10000,""unit"":""ms""}]}}]}}}]},""query"":""sensorMetrics = from(bucket: \""MyDB\"")\r\n  |\u003e range(start: -1h)\r\n  |\u003e filter(fn: (r) =\u003e r._measurement == \""mem\"")""},""source"":""Mozilla/5.0 (Windows NT 6.1; Win64; x64; rv:80.0) Gecko/20100101 Firefox/80.0"",""compiler_type"":""flux""},""dialect"":{""header"":true,""delimiter"":"","",""annotations"":[""group"",""datatype"",""default""]},""dialect_type"":""csv""}",prod01-eu-west-1,"error in query specification while starting program: this Flux script returns no streaming data. Consider adding a ""yield"" or invoking streaming functions directly, without performing an assignment",invalid,user,queryd-v1-668bbdcf74-82gb4,queryd-v1-668bbdcf74-82gb4,aks-storage-23576596-vmss00000o,dc56bb3c07ec663c,"sensorMetrics = from(bucket: ""MyDB"")\r  |> range(start: -1h)\r  |> filter(fn: (r) => r._measurement == ""mem"")",Firefox
,,0,request,2020-09-02T12:00:00Z,2020-09-02T13:00:00Z,2020-09-02T12:15:16.019716561Z,"{""request"":{""authorization"":{""id"":""063f9b5ac6045000"",""token"":"""",""status"":""active"",""description"":"""",""orgID"":""dc56bb3c07ec663c"",""userID"":""063f9b4954d66000"",""permissions"":[{""action"":""read"",""resource"":{""type"":""authorizations"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""authorizations"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""buckets"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""buckets"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""dashboards"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""dashboards"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""orgs"",""id"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""orgs"",""id"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""sources"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""sources"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""tasks"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""tasks"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""telegrafs"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""telegrafs"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""users"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""users"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""variables"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""variables"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""scrapers"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""scrapers"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""secrets"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""secrets"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""labels"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""labels"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""views"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""views"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""documents"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""documents"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""notificationRules"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""notificationRules"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""notificationEndpoints"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""notificationEndpoints"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""checks"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""checks"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""dbrp"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""dbrp"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""buckets"",""id"":""2d96370493e32279"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""telegrafs"",""id"":""063f9d793b166000"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""users"",""id"":""063f9b4954d66000""}},{""action"":""write"",""resource"":{""type"":""users"",""id"":""063f9b4954d66000""}}],""createdAt"":""0001-01-01T00:00:00Z"",""updatedAt"":""0001-01-01T00:00:00Z""},""organization_id"":""dc56bb3c07ec663c"",""compiler"":{""Now"":""2020-09-02T12:15:16.002284529Z"",""extern"":{""type"":""File"",""package"":null,""imports"":null,""body"":[{""type"":""OptionStatement"",""assignment"":{""type"":""VariableAssignment"",""id"":{""type"":""Identifier"",""name"":""v""},""init"":{""type"":""ObjectExpression"",""properties"":[{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""bucket""},""value"":{""type"":""StringLiteral"",""value"":""""}},{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""timeRangeStart""},""value"":{""type"":""UnaryExpression"",""operator"":""-"",""argument"":{""type"":""DurationLiteral"",""values"":[{""magnitude"":1,""unit"":""h""}]}}},{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""timeRangeStop""},""value"":{""type"":""CallExpression"",""callee"":{""type"":""Identifier"",""name"":""now""}}}]}}}]},""query"":""data = from(bucket: \""db/MyDB\"")\r\n  \r\n""},""source"":""Mozilla/5.0 (Windows NT 6.1; Win64; x64; rv:80.0) Gecko/20100101 Firefox/80.0"",""compiler_type"":""flux""},""dialect"":{""header"":true,""delimiter"":"","",""annotations"":[""group"",""datatype"",""default""]},""dialect_type"":""csv""}",prod01-eu-west-1,"error in query specification while starting program: this Flux script returns no streaming data. Consider adding a ""yield"" or invoking streaming functions directly, without performing an assignment",invalid,user,queryd-v1-668bbdcf74-87t2j,queryd-v1-668bbdcf74-87t2j,aks-storage-23576596-vmss00000w,dc56bb3c07ec663c,"data = from(bucket: ""db/MyDB"")\r  \r",Firefox
,,0,request,2020-09-02T12:00:00Z,2020-09-02T13:00:00Z,2020-09-02T12:16:00.736229211Z,"{""request"":{""authorization"":{""id"":""063f9b5ac6045000"",""token"":"""",""status"":""active"",""description"":"""",""orgID"":""dc56bb3c07ec663c"",""userID"":""063f9b4954d66000"",""permissions"":[{""action"":""read"",""resource"":{""type"":""authorizations"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""authorizations"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""buckets"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""buckets"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""dashboards"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""dashboards"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""orgs"",""id"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""orgs"",""id"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""sources"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""sources"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""tasks"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""tasks"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""telegrafs"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""telegrafs"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""users"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""users"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""variables"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""variables"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""scrapers"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""scrapers"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""secrets"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""secrets"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""labels"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""labels"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""views"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""views"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""documents"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""documents"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""notificationRules"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""notificationRules"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""notificationEndpoints"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""notificationEndpoints"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""checks"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""checks"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""dbrp"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""dbrp"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""buckets"",""id"":""2d96370493e32279"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""telegrafs"",""id"":""063f9d793b166000"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""users"",""id"":""063f9b4954d66000""}},{""action"":""write"",""resource"":{""type"":""users"",""id"":""063f9b4954d66000""}}],""createdAt"":""0001-01-01T00:00:00Z"",""updatedAt"":""0001-01-01T00:00:00Z""},""organization_id"":""dc56bb3c07ec663c"",""compiler"":{""Now"":""2020-09-02T12:16:00.71821288Z"",""extern"":{""type"":""File"",""package"":null,""imports"":null,""body"":[{""type"":""OptionStatement"",""assignment"":{""type"":""VariableAssignment"",""id"":{""type"":""Identifier"",""name"":""v""},""init"":{""type"":""ObjectExpression"",""properties"":[{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""bucket""},""value"":{""type"":""StringLiteral"",""value"":""""}},{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""timeRangeStart""},""value"":{""type"":""UnaryExpression"",""operator"":""-"",""argument"":{""type"":""DurationLiteral"",""values"":[{""magnitude"":1,""unit"":""h""}]}}},{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""timeRangeStop""},""value"":{""type"":""CallExpression"",""callee"":{""type"":""Identifier"",""name"":""now""}}}]}}}]},""query"":""data = from(bucket: \""db/mydb\"")\r\n  |\u003e filter(fn: (r) =\u003e r._measurement == \""mem\"")\r\n""},""source"":""Mozilla/5.0 (Windows NT 6.1; Win64; x64; rv:80.0) Gecko/20100101 Firefox/80.0"",""compiler_type"":""flux""},""dialect"":{""header"":true,""delimiter"":"","",""annotations"":[""group"",""datatype"",""default""]},""dialect_type"":""csv""}",prod01-eu-west-1,"error in query specification while starting program: this Flux script returns no streaming data. Consider adding a ""yield"" or invoking streaming functions directly, without performing an assignment",invalid,user,queryd-v1-668bbdcf74-87t2j,queryd-v1-668bbdcf74-87t2j,aks-storage-23576596-vmss00000w,dc56bb3c07ec663c,"data = from(bucket: ""db/mydb"")\r  |> filter(fn: (r) => r._measurement == ""mem"")\r",Firefox
,,0,request,2020-09-02T12:00:00Z,2020-09-02T13:00:00Z,2020-09-02T12:13:29.371467621Z,"{""request"":{""authorization"":{""id"":""063f9b5ac6045000"",""token"":"""",""status"":""active"",""description"":"""",""orgID"":""dc56bb3c07ec663c"",""userID"":""063f9b4954d66000"",""permissions"":[{""action"":""read"",""resource"":{""type"":""authorizations"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""authorizations"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""buckets"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""buckets"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""dashboards"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""dashboards"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""orgs"",""id"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""orgs"",""id"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""sources"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""sources"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""tasks"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""tasks"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""telegrafs"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""telegrafs"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""users"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""users"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""variables"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""variables"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""scrapers"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""scrapers"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""secrets"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""secrets"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""labels"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""labels"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""views"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""views"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""documents"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""documents"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""notificationRules"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""notificationRules"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""notificationEndpoints"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""notificationEndpoints"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""checks"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""checks"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""dbrp"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""dbrp"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""buckets"",""id"":""2d96370493e32279"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""telegrafs"",""id"":""063f9d793b166000"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""users"",""id"":""063f9b4954d66000""}},{""action"":""write"",""resource"":{""type"":""users"",""id"":""063f9b4954d66000""}}],""createdAt"":""0001-01-01T00:00:00Z"",""updatedAt"":""0001-01-01T00:00:00Z""},""organization_id"":""dc56bb3c07ec663c"",""compiler"":{""Now"":""2020-09-02T12:13:29.353681386Z"",""extern"":{""type"":""File"",""package"":null,""imports"":null,""body"":[{""type"":""OptionStatement"",""assignment"":{""type"":""VariableAssignment"",""id"":{""type"":""Identifier"",""name"":""v""},""init"":{""type"":""ObjectExpression"",""properties"":[{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""bucket""},""value"":{""type"":""StringLiteral"",""value"":""""}},{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""timeRangeStart""},""value"":{""type"":""UnaryExpression"",""operator"":""-"",""argument"":{""type"":""DurationLiteral"",""values"":[{""magnitude"":1,""unit"":""h""}]}}},{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""timeRangeStop""},""value"":{""type"":""CallExpression"",""callee"":{""type"":""Identifier"",""name"":""now""}}},{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""windowPeriod""},""value"":{""type"":""DurationLiteral"",""values"":[{""magnitude"":10000,""unit"":""ms""}]}}]}}}]},""query"":""data = from(bucket: \""db/MyDB\"")\r\n  |\u003e range(start: -1h)\r\n  |\u003e filter(fn: (r) =\u003e\r\n    r._measurement == \""mem\"" \r\n  )\r\n""},""source"":""Mozilla/5.0 (Windows NT 6.1; Win64; x64; rv:80.0) Gecko/20100101 Firefox/80.0"",""compiler_type"":""flux""},""dialect"":{""header"":true,""delimiter"":"","",""annotations"":[""group"",""datatype"",""default""]},""dialect_type"":""csv""}",prod01-eu-west-1,"error in query specification while starting program: this Flux script returns no streaming data. Consider adding a ""yield"" or invoking streaming functions directly, without performing an assignment",invalid,user,queryd-v1-668bbdcf74-8rpt8,queryd-v1-668bbdcf74-8rpt8,aks-storage-23576596-vmss00000t,dc56bb3c07ec663c,"data = from(bucket: ""db/MyDB"")\r  |> range(start: -1h)\r  |> filter(fn: (r) =>\r    r._measurement == ""mem"" \r  )\r",Firefox
,,0,request,2020-09-02T12:00:00Z,2020-09-02T13:00:00Z,2020-09-02T12:13:57.770254531Z,"{""request"":{""authorization"":{""id"":""063f9b5ac6045000"",""token"":"""",""status"":""active"",""description"":"""",""orgID"":""dc56bb3c07ec663c"",""userID"":""063f9b4954d66000"",""permissions"":[{""action"":""read"",""resource"":{""type"":""authorizations"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""authorizations"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""buckets"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""buckets"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""dashboards"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""dashboards"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""orgs"",""id"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""orgs"",""id"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""sources"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""sources"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""tasks"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""tasks"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""telegrafs"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""telegrafs"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""users"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""users"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""variables"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""variables"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""scrapers"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""scrapers"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""secrets"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""secrets"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""labels"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""labels"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""views"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""views"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""documents"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""documents"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""notificationRules"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""notificationRules"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""notificationEndpoints"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""notificationEndpoints"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""checks"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""checks"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""dbrp"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""dbrp"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""buckets"",""id"":""2d96370493e32279"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""telegrafs"",""id"":""063f9d793b166000"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""users"",""id"":""063f9b4954d66000""}},{""action"":""write"",""resource"":{""type"":""users"",""id"":""063f9b4954d66000""}}],""createdAt"":""0001-01-01T00:00:00Z"",""updatedAt"":""0001-01-01T00:00:00Z""},""organization_id"":""dc56bb3c07ec663c"",""compiler"":{""Now"":""2020-09-02T12:13:57.750862748Z"",""extern"":{""type"":""File"",""package"":null,""imports"":null,""body"":[{""type"":""OptionStatement"",""assignment"":{""type"":""VariableAssignment"",""id"":{""type"":""Identifier"",""name"":""v""},""init"":{""type"":""ObjectExpression"",""properties"":[{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""bucket""},""value"":{""type"":""StringLiteral"",""value"":""""}},{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""timeRangeStart""},""value"":{""type"":""UnaryExpression"",""operator"":""-"",""argument"":{""type"":""DurationLiteral"",""values"":[{""magnitude"":1,""unit"":""h""}]}}},{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""timeRangeStop""},""value"":{""type"":""CallExpression"",""callee"":{""type"":""Identifier"",""name"":""now""}}},{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""windowPeriod""},""value"":{""type"":""DurationLiteral"",""values"":[{""magnitude"":10000,""unit"":""ms""}]}}]}}}]},""query"":""data = from(bucket: \""db/MyDB\"")\r\n  |\u003e range(start: -1h)\r\n  |\u003e filter(fn: (r) =\u003e r._measurement == \""mem\"" \r\n  )\r\n""},""source"":""Mozilla/5.0 (Windows NT 6.1; Win64; x64; rv:80.0) Gecko/20100101 Firefox/80.0"",""compiler_type"":""flux""},""dialect"":{""header"":true,""delimiter"":"","",""annotations"":[""group"",""datatype"",""default""]},""dialect_type"":""csv""}",prod01-eu-west-1,"error in query specification while starting program: this Flux script returns no streaming data. Consider adding a ""yield"" or invoking streaming functions directly, without performing an assignment",invalid,user,queryd-v1-668bbdcf74-hhnp5,queryd-v1-668bbdcf74-hhnp5,aks-storage-23576596-vmss00000v,dc56bb3c07ec663c,"data = from(bucket: ""db/MyDB"")\r  |> range(start: -1h)\r  |> filter(fn: (r) => r._measurement == ""mem"" \r  )\r",Firefox
,,0,request,2020-09-02T12:00:00Z,2020-09-02T13:00:00Z,2020-09-02T12:13:18.716771923Z,"{""request"":{""authorization"":{""id"":""063f9b5ac6045000"",""token"":"""",""status"":""active"",""description"":"""",""orgID"":""dc56bb3c07ec663c"",""userID"":""063f9b4954d66000"",""permissions"":[{""action"":""read"",""resource"":{""type"":""authorizations"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""authorizations"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""buckets"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""buckets"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""dashboards"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""dashboards"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""orgs"",""id"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""orgs"",""id"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""sources"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""sources"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""tasks"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""tasks"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""telegrafs"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""telegrafs"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""users"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""users"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""variables"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""variables"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""scrapers"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""scrapers"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""secrets"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""secrets"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""labels"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""labels"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""views"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""views"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""documents"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""documents"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""notificationRules"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""notificationRules"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""notificationEndpoints"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""notificationEndpoints"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""checks"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""checks"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""dbrp"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""dbrp"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""buckets"",""id"":""2d96370493e32279"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""telegrafs"",""id"":""063f9d793b166000"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""users"",""id"":""063f9b4954d66000""}},{""action"":""write"",""resource"":{""type"":""users"",""id"":""063f9b4954d66000""}}],""createdAt"":""0001-01-01T00:00:00Z"",""updatedAt"":""0001-01-01T00:00:00Z""},""organization_id"":""dc56bb3c07ec663c"",""compiler"":{""Now"":""2020-09-02T12:13:18.70010646Z"",""extern"":{""type"":""File"",""package"":null,""imports"":null,""body"":[{""type"":""OptionStatement"",""assignment"":{""type"":""VariableAssignment"",""id"":{""type"":""Identifier"",""name"":""v""},""init"":{""type"":""ObjectExpression"",""properties"":[{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""bucket""},""value"":{""type"":""StringLiteral"",""value"":""""}},{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""timeRangeStart""},""value"":{""type"":""UnaryExpression"",""operator"":""-"",""argument"":{""type"":""DurationLiteral"",""values"":[{""magnitude"":1,""unit"":""h""}]}}},{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""timeRangeStop""},""value"":{""type"":""CallExpression"",""callee"":{""type"":""Identifier"",""name"":""now""}}},{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""windowPeriod""},""value"":{""type"":""DurationLiteral"",""values"":[{""magnitude"":10000,""unit"":""ms""}]}}]}}}]},""query"":""data = from(bucket: \""db/MyDB\"")\r\n  |\u003e range(start: -1h)\r\n  |\u003e filter(fn: (r) =\u003e\r\n    r._measurement == \""mem\"" \r\n  )\r\n""},""source"":""Mozilla/5.0 (Windows NT 6.1; Win64; x64; rv:80.0) Gecko/20100101 Firefox/80.0"",""compiler_type"":""flux""},""dialect"":{""header"":true,""delimiter"":"","",""annotations"":[""group"",""datatype"",""default""]},""dialect_type"":""csv""}",prod01-eu-west-1,"error in query specification while starting program: this Flux script returns no streaming data. Consider adding a ""yield"" or invoking streaming functions directly, without performing an assignment",invalid,user,queryd-v1-668bbdcf74-hrqn9,queryd-v1-668bbdcf74-hrqn9,aks-storage-23576596-vmss00000s,dc56bb3c07ec663c,"data = from(bucket: ""db/MyDB"")\r  |> range(start: -1h)\r  |> filter(fn: (r) =>\r    r._measurement == ""mem"" \r  )\r",Firefox
,,0,request,2020-09-02T12:00:00Z,2020-09-02T13:00:00Z,2020-09-02T12:12:25.47329787Z,"{""request"":{""authorization"":{""id"":""063f9b5ac6045000"",""token"":"""",""status"":""active"",""description"":"""",""orgID"":""dc56bb3c07ec663c"",""userID"":""063f9b4954d66000"",""permissions"":[{""action"":""read"",""resource"":{""type"":""authorizations"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""authorizations"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""buckets"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""buckets"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""dashboards"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""dashboards"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""orgs"",""id"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""orgs"",""id"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""sources"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""sources"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""tasks"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""tasks"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""telegrafs"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""telegrafs"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""users"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""users"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""variables"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""variables"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""scrapers"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""scrapers"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""secrets"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""secrets"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""labels"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""labels"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""views"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""views"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""documents"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""documents"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""notificationRules"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""notificationRules"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""notificationEndpoints"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""notificationEndpoints"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""checks"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""checks"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""dbrp"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""dbrp"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""buckets"",""id"":""2d96370493e32279"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""telegrafs"",""id"":""063f9d793b166000"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""users"",""id"":""063f9b4954d66000""}},{""action"":""write"",""resource"":{""type"":""users"",""id"":""063f9b4954d66000""}}],""createdAt"":""0001-01-01T00:00:00Z"",""updatedAt"":""0001-01-01T00:00:00Z""},""organization_id"":""dc56bb3c07ec663c"",""compiler"":{""Now"":""2020-09-02T12:12:25.454971285Z"",""extern"":{""type"":""File"",""package"":null,""imports"":null,""body"":[{""type"":""OptionStatement"",""assignment"":{""type"":""VariableAssignment"",""id"":{""type"":""Identifier"",""name"":""v""},""init"":{""type"":""ObjectExpression"",""properties"":[{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""bucket""},""value"":{""type"":""StringLiteral"",""value"":""""}},{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""timeRangeStart""},""value"":{""type"":""UnaryExpression"",""operator"":""-"",""argument"":{""type"":""DurationLiteral"",""values"":[{""magnitude"":1,""unit"":""h""}]}}},{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""timeRangeStop""},""value"":{""type"":""CallExpression"",""callee"":{""type"":""Identifier"",""name"":""now""}}},{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""windowPeriod""},""value"":{""type"":""DurationLiteral"",""values"":[{""magnitude"":10000,""unit"":""ms""}]}}]}}}]},""query"":""data = from(bucket: \""db/rp\"")\r\n  |\u003e range(start: -1h)\r\n  |\u003e filter(fn: (r) =\u003e\r\n    r._measurement == \""mem\"" \r\n  )\r\n""},""source"":""Mozilla/5.0 (Windows NT 6.1; Win64; x64; rv:80.0) Gecko/20100101 Firefox/80.0"",""compiler_type"":""flux""},""dialect"":{""header"":true,""delimiter"":"","",""annotations"":[""group"",""datatype"",""default""]},""dialect_type"":""csv""}",prod01-eu-west-1,"error in query specification while starting program: this Flux script returns no streaming data. Consider adding a ""yield"" or invoking streaming functions directly, without performing an assignment",invalid,user,queryd-v1-668bbdcf74-krft8,queryd-v1-668bbdcf74-krft8,aks-storage-23576596-vmss00000m,dc56bb3c07ec663c,"data = from(bucket: ""db/rp"")\r  |> range(start: -1h)\r  |> filter(fn: (r) =>\r    r._measurement == ""mem"" \r  )\r",Firefox
,,0,request,2020-09-02T12:00:00Z,2020-09-02T13:00:00Z,2020-09-02T12:13:44.62902407Z,"{""request"":{""authorization"":{""id"":""063f9b5ac6045000"",""token"":"""",""status"":""active"",""description"":"""",""orgID"":""dc56bb3c07ec663c"",""userID"":""063f9b4954d66000"",""permissions"":[{""action"":""read"",""resource"":{""type"":""authorizations"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""authorizations"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""buckets"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""buckets"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""dashboards"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""dashboards"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""orgs"",""id"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""orgs"",""id"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""sources"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""sources"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""tasks"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""tasks"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""telegrafs"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""telegrafs"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""users"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""users"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""variables"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""variables"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""scrapers"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""scrapers"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""secrets"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""secrets"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""labels"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""labels"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""views"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""views"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""documents"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""documents"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""notificationRules"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""notificationRules"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""notificationEndpoints"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""notificationEndpoints"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""checks"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""checks"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""dbrp"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""dbrp"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""buckets"",""id"":""2d96370493e32279"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""telegrafs"",""id"":""063f9d793b166000"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""users"",""id"":""063f9b4954d66000""}},{""action"":""write"",""resource"":{""type"":""users"",""id"":""063f9b4954d66000""}}],""createdAt"":""0001-01-01T00:00:00Z"",""updatedAt"":""0001-01-01T00:00:00Z""},""organization_id"":""dc56bb3c07ec663c"",""compiler"":{""Now"":""2020-09-02T12:13:44.610314757Z"",""extern"":{""type"":""File"",""package"":null,""imports"":null,""body"":[{""type"":""OptionStatement"",""assignment"":{""type"":""VariableAssignment"",""id"":{""type"":""Identifier"",""name"":""v""},""init"":{""type"":""ObjectExpression"",""properties"":[{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""bucket""},""value"":{""type"":""StringLiteral"",""value"":""""}},{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""timeRangeStart""},""value"":{""type"":""UnaryExpression"",""operator"":""-"",""argument"":{""type"":""DurationLiteral"",""values"":[{""magnitude"":1,""unit"":""h""}]}}},{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""timeRangeStop""},""value"":{""type"":""CallExpression"",""callee"":{""type"":""Identifier"",""name"":""now""}}},{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""windowPeriod""},""value"":{""type"":""DurationLiteral"",""values"":[{""magnitude"":10000,""unit"":""ms""}]}}]}}}]},""query"":""data = from(bucket: \""db/MyDB\"")\r\n  |\u003e range(start: -1h)\r\n  |\u003e filter(fn: (r) =\u003e\r\n    r._measurement == \""mem\"" \r\n  )\r\n""},""source"":""Mozilla/5.0 (Windows NT 6.1; Win64; x64; rv:80.0) Gecko/20100101 Firefox/80.0"",""compiler_type"":""flux""},""dialect"":{""header"":true,""delimiter"":"","",""annotations"":[""group"",""datatype"",""default""]},""dialect_type"":""csv""}",prod01-eu-west-1,"error in query specification while starting program: this Flux script returns no streaming data. Consider adding a ""yield"" or invoking streaming functions directly, without performing an assignment",invalid,user,queryd-v1-668bbdcf74-m4pqw,queryd-v1-668bbdcf74-m4pqw,aks-storage-23576596-vmss00000p,dc56bb3c07ec663c,"data = from(bucket: ""db/MyDB"")\r  |> range(start: -1h)\r  |> filter(fn: (r) =>\r    r._measurement == ""mem"" \r  )\r",Firefox
,,0,request,2020-09-02T12:00:00Z,2020-09-02T13:00:00Z,2020-09-02T12:12:17.073621426Z,"{""request"":{""authorization"":{""id"":""063f9b5ac6045000"",""token"":"""",""status"":""active"",""description"":"""",""orgID"":""dc56bb3c07ec663c"",""userID"":""063f9b4954d66000"",""permissions"":[{""action"":""read"",""resource"":{""type"":""authorizations"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""authorizations"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""buckets"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""buckets"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""dashboards"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""dashboards"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""orgs"",""id"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""orgs"",""id"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""sources"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""sources"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""tasks"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""tasks"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""telegrafs"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""telegrafs"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""users"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""users"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""variables"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""variables"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""scrapers"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""scrapers"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""secrets"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""secrets"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""labels"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""labels"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""views"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""views"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""documents"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""documents"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""notificationRules"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""notificationRules"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""notificationEndpoints"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""notificationEndpoints"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""checks"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""checks"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""dbrp"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""dbrp"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""buckets"",""id"":""2d96370493e32279"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""telegrafs"",""id"":""063f9d793b166000"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""users"",""id"":""063f9b4954d66000""}},{""action"":""write"",""resource"":{""type"":""users"",""id"":""063f9b4954d66000""}}],""createdAt"":""0001-01-01T00:00:00Z"",""updatedAt"":""0001-01-01T00:00:00Z""},""organization_id"":""dc56bb3c07ec663c"",""compiler"":{""Now"":""2020-09-02T12:12:17.057108159Z"",""extern"":{""type"":""File"",""package"":null,""imports"":null,""body"":[{""type"":""OptionStatement"",""assignment"":{""type"":""VariableAssignment"",""id"":{""type"":""Identifier"",""name"":""v""},""init"":{""type"":""ObjectExpression"",""properties"":[{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""bucket""},""value"":{""type"":""StringLiteral"",""value"":""""}},{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""timeRangeStart""},""value"":{""type"":""UnaryExpression"",""operator"":""-"",""argument"":{""type"":""DurationLiteral"",""values"":[{""magnitude"":1,""unit"":""h""}]}}},{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""timeRangeStop""},""value"":{""type"":""CallExpression"",""callee"":{""type"":""Identifier"",""name"":""now""}}},{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""windowPeriod""},""value"":{""type"":""DurationLiteral"",""values"":[{""magnitude"":10000,""unit"":""ms""}]}}]}}}]},""query"":""data = from(bucket: \""Mydb/mem\"")\r\n  |\u003e range(start: -1h)\r\n  |\u003e filter(fn: (r) =\u003e\r\n    r._measurement == \""mem\"" \r\n  )\r\n""},""source"":""Mozilla/5.0 (Windows NT 6.1; Win64; x64; rv:80.0) Gecko/20100101 Firefox/80.0"",""compiler_type"":""flux""},""dialect"":{""header"":true,""delimiter"":"","",""annotations"":[""group"",""datatype"",""default""]},""dialect_type"":""csv""}",prod01-eu-west-1,"error in query specification while starting program: this Flux script returns no streaming data. Consider adding a ""yield"" or invoking streaming functions directly, without performing an assignment",invalid,user,queryd-v1-668bbdcf74-qs7rp,queryd-v1-668bbdcf74-qs7rp,aks-storage-23576596-vmss00000u,dc56bb3c07ec663c,"data = from(bucket: ""Mydb/mem"")\r  |> range(start: -1h)\r  |> filter(fn: (r) =>\r    r._measurement == ""mem"" \r  )\r",Firefox
,,0,request,2020-09-02T12:00:00Z,2020-09-02T13:00:00Z,2020-09-02T12:15:48.905930446Z,"{""request"":{""authorization"":{""id"":""063f9b5ac6045000"",""token"":"""",""status"":""active"",""description"":"""",""orgID"":""dc56bb3c07ec663c"",""userID"":""063f9b4954d66000"",""permissions"":[{""action"":""read"",""resource"":{""type"":""authorizations"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""authorizations"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""buckets"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""buckets"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""dashboards"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""dashboards"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""orgs"",""id"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""orgs"",""id"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""sources"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""sources"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""tasks"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""tasks"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""telegrafs"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""telegrafs"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""users"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""users"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""variables"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""variables"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""scrapers"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""scrapers"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""secrets"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""secrets"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""labels"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""labels"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""views"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""views"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""documents"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""documents"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""notificationRules"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""notificationRules"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""notificationEndpoints"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""notificationEndpoints"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""checks"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""checks"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""dbrp"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""dbrp"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""buckets"",""id"":""2d96370493e32279"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""telegrafs"",""id"":""063f9d793b166000"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""users"",""id"":""063f9b4954d66000""}},{""action"":""write"",""resource"":{""type"":""users"",""id"":""063f9b4954d66000""}}],""createdAt"":""0001-01-01T00:00:00Z"",""updatedAt"":""0001-01-01T00:00:00Z""},""organization_id"":""dc56bb3c07ec663c"",""compiler"":{""Now"":""2020-09-02T12:15:48.889804822Z"",""extern"":{""type"":""File"",""package"":null,""imports"":null,""body"":[{""type"":""OptionStatement"",""assignment"":{""type"":""VariableAssignment"",""id"":{""type"":""Identifier"",""name"":""v""},""init"":{""type"":""ObjectExpression"",""properties"":[{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""bucket""},""value"":{""type"":""StringLiteral"",""value"":""""}},{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""timeRangeStart""},""value"":{""type"":""UnaryExpression"",""operator"":""-"",""argument"":{""type"":""DurationLiteral"",""values"":[{""magnitude"":1,""unit"":""h""}]}}},{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""timeRangeStop""},""value"":{""type"":""CallExpression"",""callee"":{""type"":""Identifier"",""name"":""now""}}}]}}}]},""query"":""data = from(bucket: \""db/MyDB\"")\r\n  |\u003e filter(fn: (r) =\u003e r._measurement == \""mem\"")\r\n""},""source"":""Mozilla/5.0 (Windows NT 6.1; Win64; x64; rv:80.0) Gecko/20100101 Firefox/80.0"",""compiler_type"":""flux""},""dialect"":{""header"":true,""delimiter"":"","",""annotations"":[""group"",""datatype"",""default""]},""dialect_type"":""csv""}",prod01-eu-west-1,"error in query specification while starting program: this Flux script returns no streaming data. Consider adding a ""yield"" or invoking streaming functions directly, without performing an assignment",invalid,user,queryd-v1-668bbdcf74-tscn5,queryd-v1-668bbdcf74-tscn5,aks-storage-23576596-vmss00000p,dc56bb3c07ec663c,"data = from(bucket: ""db/MyDB"")\r  |> filter(fn: (r) => r._measurement == ""mem"")\r",Firefox
,,0,request,2020-09-02T12:00:00Z,2020-09-02T13:00:00Z,2020-09-02T12:11:54.023053163Z,"{""request"":{""authorization"":{""id"":""063f9b5ac6045000"",""token"":"""",""status"":""active"",""description"":"""",""orgID"":""dc56bb3c07ec663c"",""userID"":""063f9b4954d66000"",""permissions"":[{""action"":""read"",""resource"":{""type"":""authorizations"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""authorizations"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""buckets"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""buckets"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""dashboards"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""dashboards"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""orgs"",""id"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""orgs"",""id"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""sources"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""sources"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""tasks"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""tasks"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""telegrafs"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""telegrafs"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""users"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""users"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""variables"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""variables"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""scrapers"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""scrapers"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""secrets"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""secrets"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""labels"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""labels"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""views"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""views"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""documents"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""documents"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""notificationRules"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""notificationRules"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""notificationEndpoints"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""notificationEndpoints"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""checks"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""checks"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""dbrp"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""dbrp"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""buckets"",""id"":""2d96370493e32279"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""telegrafs"",""id"":""063f9d793b166000"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""users"",""id"":""063f9b4954d66000""}},{""action"":""write"",""resource"":{""type"":""users"",""id"":""063f9b4954d66000""}}],""createdAt"":""0001-01-01T00:00:00Z"",""updatedAt"":""0001-01-01T00:00:00Z""},""organization_id"":""dc56bb3c07ec663c"",""compiler"":{""Now"":""2020-09-02T12:11:54.002991404Z"",""extern"":{""type"":""File"",""package"":null,""imports"":null,""body"":[{""type"":""OptionStatement"",""assignment"":{""type"":""VariableAssignment"",""id"":{""type"":""Identifier"",""name"":""v""},""init"":{""type"":""ObjectExpression"",""properties"":[{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""bucket""},""value"":{""type"":""StringLiteral"",""value"":""""}},{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""timeRangeStart""},""value"":{""type"":""UnaryExpression"",""operator"":""-"",""argument"":{""type"":""DurationLiteral"",""values"":[{""magnitude"":1,""unit"":""h""}]}}},{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""timeRangeStop""},""value"":{""type"":""CallExpression"",""callee"":{""type"":""Identifier"",""name"":""now""}}},{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""windowPeriod""},""value"":{""type"":""DurationLiteral"",""values"":[{""magnitude"":10000,""unit"":""ms""}]}}]}}}]},""query"":""data = from(bucket: \""Mydb/mem\"")\r\n  |\u003e range(start: -1h)\r\n  |\u003e filter(fn: (r) =\u003e\r\n    r._measurement == \""example-measurement\"" and\r\n    r._field == \""example-field\""\r\n  )\r\n""},""source"":""Mozilla/5.0 (Windows NT 6.1; Win64; x64; rv:80.0) Gecko/20100101 Firefox/80.0"",""compiler_type"":""flux""},""dialect"":{""header"":true,""delimiter"":"","",""annotations"":[""group"",""datatype"",""default""]},""dialect_type"":""csv""}",prod01-eu-west-1,"error in query specification while starting program: this Flux script returns no streaming data. Consider adding a ""yield"" or invoking streaming functions directly, without performing an assignment",invalid,user,queryd-v1-668bbdcf74-xdfbp,queryd-v1-668bbdcf74-xdfbp,aks-storage-23576596-vmss00000n,dc56bb3c07ec663c,"data = from(bucket: ""Mydb/mem"")\r  |> range(start: -1h)\r  |> filter(fn: (r) =>\r    r._measurement == ""example-measurement"" and\r    r._field == ""example-field""\r  )\r",Firefox
,,0,request,2020-09-02T12:00:00Z,2020-09-02T13:00:00Z,2020-09-02T12:15:37.717994087Z,"{""request"":{""authorization"":{""id"":""063f9b5ac6045000"",""token"":"""",""status"":""active"",""description"":"""",""orgID"":""dc56bb3c07ec663c"",""userID"":""063f9b4954d66000"",""permissions"":[{""action"":""read"",""resource"":{""type"":""authorizations"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""authorizations"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""buckets"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""buckets"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""dashboards"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""dashboards"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""orgs"",""id"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""orgs"",""id"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""sources"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""sources"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""tasks"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""tasks"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""telegrafs"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""telegrafs"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""users"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""users"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""variables"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""variables"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""scrapers"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""scrapers"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""secrets"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""secrets"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""labels"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""labels"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""views"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""views"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""documents"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""documents"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""notificationRules"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""notificationRules"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""notificationEndpoints"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""notificationEndpoints"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""checks"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""checks"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""dbrp"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""dbrp"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""write"",""resource"":{""type"":""buckets"",""id"":""2d96370493e32279"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""telegrafs"",""id"":""063f9d793b166000"",""orgID"":""dc56bb3c07ec663c""}},{""action"":""read"",""resource"":{""type"":""users"",""id"":""063f9b4954d66000""}},{""action"":""write"",""resource"":{""type"":""users"",""id"":""063f9b4954d66000""}}],""createdAt"":""0001-01-01T00:00:00Z"",""updatedAt"":""0001-01-01T00:00:00Z""},""organization_id"":""dc56bb3c07ec663c"",""compiler"":{""Now"":""2020-09-02T12:15:37.700873475Z"",""extern"":{""type"":""File"",""package"":null,""imports"":null,""body"":[{""type"":""OptionStatement"",""assignment"":{""type"":""VariableAssignment"",""id"":{""type"":""Identifier"",""name"":""v""},""init"":{""type"":""ObjectExpression"",""properties"":[{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""bucket""},""value"":{""type"":""StringLiteral"",""value"":""""}},{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""timeRangeStart""},""value"":{""type"":""UnaryExpression"",""operator"":""-"",""argument"":{""type"":""DurationLiteral"",""values"":[{""magnitude"":1,""unit"":""h""}]}}},{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""timeRangeStop""},""value"":{""type"":""CallExpression"",""callee"":{""type"":""Identifier"",""name"":""now""}}}]}}}]},""query"":""data = from(bucket: \""db/MyDB\"")\r\n  |\u003e filter(fn: (r) =\u003e r._measurement == \""mem\"" \r\n  )\r\n""},""source"":""Mozilla/5.0 (Windows NT 6.1; Win64; x64; rv:80.0) Gecko/20100101 Firefox/80.0"",""compiler_type"":""flux""},""dialect"":{""header"":true,""delimiter"":"","",""annotations"":[""group"",""datatype"",""default""]},""dialect_type"":""csv""}",prod01-eu-west-1,"error in query specification while starting program: this Flux script returns no streaming data. Consider adding a ""yield"" or invoking streaming functions directly, without performing an assignment",invalid,user,queryd-v1-668bbdcf74-zwvjc,queryd-v1-668bbdcf74-zwvjc,aks-storage-23576596-vmss00000q,dc56bb3c07ec663c,"data = from(bucket: ""db/MyDB"")\r  |> filter(fn: (r) => r._measurement == ""mem"" \r  )\r",Firefox
,,0,request,2020-09-02T12:00:00Z,2020-09-02T13:00:00Z,2020-09-02T12:29:49.06643776Z,"{""request"":{""authorization"":{""id"":""063fb21b8c0f5000"",""token"":"""",""status"":""active"",""description"":"""",""orgID"":""c86c6a73dbfd08b8"",""userID"":""05275d2dffbc2000"",""permissions"":[{""action"":""read"",""resource"":{""type"":""authorizations"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""authorizations"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""buckets"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""buckets"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""dashboards"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""dashboards"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""orgs"",""id"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""orgs"",""id"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""sources"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""sources"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""tasks"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""tasks"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""telegrafs"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""telegrafs"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""users"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""users"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""variables"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""variables"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""scrapers"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""scrapers"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""secrets"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""secrets"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""labels"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""labels"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""views"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""views"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""documents"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""documents"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""notificationRules"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""notificationRules"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""notificationEndpoints"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""notificationEndpoints"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""checks"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""checks"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""dbrp"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""dbrp"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""buckets"",""id"":""b1216af140aadc17""}},{""action"":""read"",""resource"":{""type"":""buckets"",""id"":""b1216af140aadc17""}},{""action"":""read"",""resource"":{""type"":""orgs"",""id"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""authorizations"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""authorizations"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""buckets"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""buckets"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""dashboards"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""dashboards"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""sources"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""sources"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""tasks"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""tasks"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""telegrafs"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""telegrafs"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""users"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""users"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""variables"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""variables"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""scrapers"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""scrapers"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""secrets"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""secrets"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""labels"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""labels"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""views"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""views"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""documents"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""documents"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""authorizations"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""authorizations"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""buckets"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""buckets"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""checks"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""checks"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""dashboards"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""dashboards"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""documents"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""documents"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""labels"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""labels"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""notificationRules"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""notificationRules"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""notificationEndpoints"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""notificationEndpoints"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""orgs"",""id"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""secrets"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""secrets"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""scrapers"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""scrapers"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""sources"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""sources"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""tasks"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""tasks"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""telegrafs"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""telegrafs"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""users"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""users"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""variables"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""variables"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""views"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""views"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""authorizations"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""authorizations"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""buckets"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""buckets"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""checks"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""checks"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""dashboards"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""dashboards"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""documents"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""documents"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""labels"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""labels"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""notificationRules"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""notificationRules"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""notificationEndpoints"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""notificationEndpoints"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""orgs"",""id"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""secrets"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""secrets"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""scrapers"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""scrapers"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""sources"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""sources"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""tasks"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""tasks"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""telegrafs"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""telegrafs"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""users"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""users"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""variables"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""variables"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""views"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""views"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""users"",""id"":""05275d2dffbc2000""}},{""action"":""write"",""resource"":{""type"":""users"",""id"":""05275d2dffbc2000""}}],""createdAt"":""0001-01-01T00:00:00Z"",""updatedAt"":""0001-01-01T00:00:00Z""},""organization_id"":""c86c6a73dbfd08b8"",""compiler"":{""Now"":""2020-09-02T12:29:49.041696404Z"",""extern"":{""type"":""File"",""package"":null,""imports"":null,""body"":[{""type"":""OptionStatement"",""assignment"":{""type"":""VariableAssignment"",""id"":{""type"":""Identifier"",""name"":""v""},""init"":{""type"":""ObjectExpression"",""properties"":[{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""timeRangeStart""},""value"":{""type"":""UnaryExpression"",""operator"":""-"",""argument"":{""type"":""DurationLiteral"",""values"":[{""magnitude"":15,""unit"":""m""}]}}},{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""timeRangeStop""},""value"":{""type"":""CallExpression"",""callee"":{""type"":""Identifier"",""name"":""now""}}},{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""windowPeriod""},""value"":{""type"":""DurationLiteral"",""values"":[{""magnitude"":10000,""unit"":""ms""}]}}]}}}]},""query"":""from(bucket: \""cronjobs\"")\n  |\u003e range(start: v.timeRangeStart, stop: v.timeRangeStop)\n  |\u003e filter(fn: (r) =\u003e r[\""_measurement\""] == \""iconico-cronjobs-5ed512ac09c51c0bea12ec55\"")\n  |\u003e filter(fn: (r) =\u003e r[\""_field\""] == \""msg\"")\n  |\u003e aggregateWindow(every: v.windowPeriod, fn: mean, createEmpty: false)\n  |\u003e yield(name: \""mean\"")""},""source"":""Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/84.0.4147.89 Safari/537.36"",""compiler_type"":""flux""},""dialect"":{""header"":true,""delimiter"":"","",""annotations"":[""group"",""datatype"",""default""]},""dialect_type"":""csv""}",prod01-us-central-1,unsupported input type for mean aggregate: string; unsupported input type for mean aggregate: string,invalid,user,queryd-v1-dc8994845-67z87,queryd-v1-dc8994845-67z87,gke-prod01-us-central-1-highmem1-4bc74378-dbm8,c86c6a73dbfd08b8,"from(bucket: ""cronjobs"")  |> range(start: v.timeRangeStart, stop: v.timeRangeStop)  |> filter(fn: (r) => r[""_measurement""] == ""iconico-cronjobs-5ed512ac09c51c0bea12ec55"")  |> filter(fn: (r) => r[""_field""] == ""msg"")  |> aggregateWindow(every: v.windowPeriod, fn: mean, createEmpty: false)  |> yield(name: ""mean"")",Chrome
,,0,request,2020-09-02T12:00:00Z,2020-09-02T13:00:00Z,2020-09-02T12:29:44.679013171Z,"{""request"":{""authorization"":{""id"":""063fb21b8c0f5000"",""token"":"""",""status"":""active"",""description"":"""",""orgID"":""c86c6a73dbfd08b8"",""userID"":""05275d2dffbc2000"",""permissions"":[{""action"":""read"",""resource"":{""type"":""authorizations"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""authorizations"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""buckets"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""buckets"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""dashboards"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""dashboards"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""orgs"",""id"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""orgs"",""id"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""sources"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""sources"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""tasks"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""tasks"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""telegrafs"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""telegrafs"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""users"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""users"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""variables"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""variables"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""scrapers"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""scrapers"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""secrets"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""secrets"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""labels"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""labels"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""views"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""views"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""documents"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""documents"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""notificationRules"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""notificationRules"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""notificationEndpoints"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""notificationEndpoints"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""checks"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""checks"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""dbrp"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""dbrp"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""buckets"",""id"":""b1216af140aadc17""}},{""action"":""read"",""resource"":{""type"":""buckets"",""id"":""b1216af140aadc17""}},{""action"":""read"",""resource"":{""type"":""orgs"",""id"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""authorizations"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""authorizations"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""buckets"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""buckets"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""dashboards"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""dashboards"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""sources"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""sources"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""tasks"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""tasks"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""telegrafs"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""telegrafs"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""users"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""users"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""variables"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""variables"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""scrapers"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""scrapers"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""secrets"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""secrets"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""labels"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""labels"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""views"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""views"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""documents"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""documents"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""authorizations"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""authorizations"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""buckets"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""buckets"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""checks"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""checks"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""dashboards"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""dashboards"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""documents"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""documents"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""labels"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""labels"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""notificationRules"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""notificationRules"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""notificationEndpoints"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""notificationEndpoints"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""orgs"",""id"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""secrets"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""secrets"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""scrapers"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""scrapers"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""sources"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""sources"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""tasks"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""tasks"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""telegrafs"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""telegrafs"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""users"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""users"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""variables"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""variables"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""views"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""views"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""authorizations"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""authorizations"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""buckets"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""buckets"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""checks"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""checks"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""dashboards"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""dashboards"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""documents"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""documents"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""labels"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""labels"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""notificationRules"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""notificationRules"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""notificationEndpoints"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""notificationEndpoints"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""orgs"",""id"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""secrets"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""secrets"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""scrapers"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""scrapers"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""sources"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""sources"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""tasks"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""tasks"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""telegrafs"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""telegrafs"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""users"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""users"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""variables"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""variables"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""views"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""write"",""resource"":{""type"":""views"",""orgID"":""c86c6a73dbfd08b8""}},{""action"":""read"",""resource"":{""type"":""users"",""id"":""05275d2dffbc2000""}},{""action"":""write"",""resource"":{""type"":""users"",""id"":""05275d2dffbc2000""}}],""createdAt"":""0001-01-01T00:00:00Z"",""updatedAt"":""0001-01-01T00:00:00Z""},""organization_id"":""c86c6a73dbfd08b8"",""compiler"":{""Now"":""2020-09-02T12:29:44.64391534Z"",""extern"":{""type"":""File"",""package"":null,""imports"":null,""body"":[{""type"":""OptionStatement"",""assignment"":{""type"":""VariableAssignment"",""id"":{""type"":""Identifier"",""name"":""v""},""init"":{""type"":""ObjectExpression"",""properties"":[{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""timeRangeStart""},""value"":{""type"":""UnaryExpression"",""operator"":""-"",""argument"":{""type"":""DurationLiteral"",""values"":[{""magnitude"":5,""unit"":""m""}]}}},{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""timeRangeStop""},""value"":{""type"":""CallExpression"",""callee"":{""type"":""Identifier"",""name"":""now""}}},{""type"":""Property"",""key"":{""type"":""Identifier"",""name"":""windowPeriod""},""value"":{""type"":""DurationLiteral"",""values"":[{""magnitude"":10000,""unit"":""ms""}]}}]}}}]},""query"":""from(bucket: \""cronjobs\"")\n  |\u003e range(start: v.timeRangeStart, stop: v.timeRangeStop)\n  |\u003e filter(fn: (r) =\u003e r[\""_measurement\""] == \""iconico-cronjobs-5ed512ac09c51c0bea12ec55\"")\n  |\u003e filter(fn: (r) =\u003e r[\""_field\""] == \""msg\"")\n  |\u003e aggregateWindow(every: v.windowPeriod, fn: mean, createEmpty: false)\n  |\u003e yield(name: \""mean\"")""},""source"":""Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/84.0.4147.89 Safari/537.36"",""compiler_type"":""flux""},""dialect"":{""header"":true,""delimiter"":"","",""annotations"":[""group"",""datatype"",""default""]},""dialect_type"":""csv""}",prod01-us-central-1,unsupported input type for mean aggregate: string; unsupported input type for mean aggregate: string,invalid,user,queryd-v1-dc8994845-kxp6b,queryd-v1-dc8994845-kxp6b,gke-prod01-us-central-1-highmem1-4bc74378-dbm8,c86c6a73dbfd08b8,"from(bucket: ""cronjobs"")  |> range(start: v.timeRangeStart, stop: v.timeRangeStop)  |> filter(fn: (r) => r[""_measurement""] == ""iconico-cronjobs-5ed512ac09c51c0bea12ec55"")  |> filter(fn: (r) => r[""_field""] == ""msg"")  |> aggregateWindow(every: v.windowPeriod, fn: mean, createEmpty: false)  |> yield(name: ""mean"")",Chrome

#group,false,false,true,false,false,false,false,false,false,false,false,false,false,false,false,false
#datatype,string,long,string,long,long,long,long,long,long,long,long,long,string,string,long,long
#default,_profiler,,,,,,,,,,,,,,,
,result,table,_measurement,TotalDuration,CompileDuration,QueueDuration,PlanDuration,RequeueDuration,ExecuteDuration,Concurrency,MaxAllocated,TotalAllocated,RuntimeErrors,flux/query-plan,influxdb/scanned-bytes,influxdb/scanned-values
,,0,profiler/query,41826554659,2390321,41026,0,0,41824071714,0,326272,0,,"digraph {
  merged_ReadRange_filter2
  map3
  map4
  map5
  map6
  map7
  filter8
  // r.request !~ <semantic format error, unknown node *semantic.RegexpLiteral>
  drop9
  // DualImplProcedureSpec, UseDeprecated = false
  group10
  yield11

  merged_ReadRange_filter2 -> map3
  map3 -> map4
  map4 -> map5
  map5 -> map6
  map6 -> map7
  map7 -> filter8
  filter8 -> drop9
  drop9 -> group10
  group10 -> yield11
}
",156253,27`))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			return r.Tables()
		},
		IsDone: func(tbl flux.Table) bool {
			return tbl.(interface{ IsDone() bool }).IsDone()
		},
	})
}

var crlfPattern = regexp.MustCompile(`\r?\n`)

func toCRLF(data string) []byte {
	return []byte(crlfPattern.ReplaceAllString(data, "\r\n"))
}
