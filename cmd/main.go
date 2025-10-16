package main

import (
	"os"

	"github.com/openshift-kni/commatrix/cmd/generate"
	"github.com/openshift-kni/commatrix/pkg/client"
	"github.com/openshift-kni/commatrix/pkg/errhandler"

	"k8s.io/cli-runtime/pkg/genericiooptions"
)

func main() {
	defer errhandler.RecoverAndExit()
	ioStreams := genericiooptions.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}

	cs, err := client.New()
	if err != nil {
		errhandler.HandleAndExit(err)
	}
	root := generate.NewCmd(cs, ioStreams)
	// Silence Cobra's automatic error/usage output to avoid leaking details.
	root.SilenceErrors = true
	root.SilenceUsage = true
	if err := root.Execute(); err != nil {
		errhandler.HandleAndExit(err)
	}
}
