package main

import (
	"log"
	"os"

	"github.com/openshift-kni/commatrix/cmd/generate"
	"github.com/openshift-kni/commatrix/pkg/client"

	"k8s.io/cli-runtime/pkg/genericiooptions"
)

func main() {
	ioStreams := genericiooptions.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}

	cs, err := client.New()
	if err != nil {
		log.Panicf("Error: %v\n", err)
		os.Exit(1)
	}
	root := generate.NewCmd(cs, ioStreams)
	if err := root.Execute(); err != nil {
		log.Panicf("Error: %v\n", err)
		os.Exit(1)
	}
}
