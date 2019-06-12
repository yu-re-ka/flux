package ledger_test

import "testing"
import "ledger"

inData = "
; A sample journal file.
;
; Sets up this account tree:
; assets
;   bank
;     checking
;     saving
;   cash
; expenses
;   food
;   supplies
; income
;   gifts
;   salary
; liabilities
;   debts

2008/01/01 income
    assets:bank:checking  $1
    income:salary

2008/06/01 gift
    assets:bank:checking  $2
    income:gifts

2008/06/02 ! save
    assets:bank:saving  $1
    assets:bank:checking

2008/06/03 * eat & shop
    expenses:food      $1
    expenses:supplies  $3
    assets:cash

2008/12/31 * pay off
    liabilities:debts  $1
    assets:bank:checking


;final comment
"

outData = "
#datatype,string,long,dateTime:RFC3339,string,string,boolean,boolean,string,string,string,double
#group,false,false,true,true,true,false,false,false,false,false,false
#default,_result,,,,,,,,,,
,result,table,_time,commodity,tx,cleared,pending,l0,l1,l2,_value
,,0,2008-01-01T00:00:00Z,$,income,false,false,assets,bank,checking,1
,,0,2008-01-01T00:00:00Z,$,income,false,false,income,salary,,-1
,,1,2008-06-01T00:00:00Z,$,gift,false,false,assets,bank,checking,2
,,1,2008-06-01T00:00:00Z,$,gift,false,false,income,gifts,,-2
,,2,2008-06-02T00:00:00Z,$,save,false,true,assets,bank,saving,1
,,2,2008-06-02T00:00:00Z,$,save,false,true,assets,bank,checking,-1
,,3,2008-06-03T00:00:00Z,$,eat & shop,true,false,expenses,food,,1
,,3,2008-06-03T00:00:00Z,$,eat & shop,true,false,expenses,supplies,,3
,,3,2008-06-03T00:00:00Z,$,eat & shop,true,false,assets,cash,,-4
,,4,2008-12-31T00:00:00Z,$,pay off,true,false,liabilities,debts,,1
,,4,2008-12-31T00:00:00Z,$,pay off,true,false,assets,bank,checking,-1
"

test from = () => ({
    input: ledger.from(raw: inData),
    want: testing.loadMem(csv: outData),
    fn: (tables=<-) => tables
})

