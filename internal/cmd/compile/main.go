package main

import (
	"io/ioutil"
	"os"

	"github.com/jsternberg/flux-lang/parser"
)

func main() {
	for _, arg := range os.Args[1:] {
		buf, err := ioutil.ReadFile(arg)
		if err != nil {
			panic(err)
		}

		content := string(buf)
		parser.Parse(content)
	}
}
