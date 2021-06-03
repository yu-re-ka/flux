package csv


import c "csv"
import "experimental/http"

// csv.from is a function that retrieves annotated CSV from a URL.
//
// - `url` is the URL to retrieve annotated CSV from.
//
from = (url) => c.from(csv: string(v: http.get(url: url).body))
