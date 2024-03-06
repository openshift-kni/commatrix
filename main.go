package main

import (
	"flag"
	"fmt"

	"github.com/liornoy/node-comm-lib/commatrix"
)

var (
	customEntriesPath = flag.String("custom-entries-path", "", "specifies the path to user-defined custom entries to be added to the communication matrix, formatted as per module specifications.")
)

func main() {
	flag.Parse()

	res, err := commatrix.New("/home/lnoy/Documents/ssh-kubeconfig", *customEntriesPath)
	if err != nil {
		panic(err)
	}

	fmt.Print(res)
}
