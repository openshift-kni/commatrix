package main

import (
	"flag"
	"fmt"
	"os"

	log "github.com/sirupsen/logrus"

	"github.com/liornoy/node-comm-lib/commatrix"
)

var (
	customEntriesPath = flag.String("custom-entries-path", "", "specifies the path to user-defined custom entries to be added to the communication matrix, formatted as per module specifications.")
	logLevel          = flag.String("loglevel", "info", "set the log level (debug, info, warn, error, fatal, panic)")
)

func main() {
	flag.Parse()

	level, err := log.ParseLevel(*logLevel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: invalid log level '%s'\n", *logLevel)
		os.Exit(1)
	}
	log.SetLevel(level)

	kubeconfig, ok := os.LookupEnv("KUBECONFIG")
	if !ok {
		panic("must set the KUBECONFIG environment variable")
	}

	res, err := commatrix.New(kubeconfig, *customEntriesPath, commatrix.Baremetal)
	if err != nil {
		panic(err)
	}

	fmt.Print(res)
}
