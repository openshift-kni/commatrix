package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/openshift-kni/commatrix/commatrix"
)

var (
	destDir             string
	format              string
	envStr              string
	deploymentStr       string
	customEntriesPath   string
	customEntriesFormat string
)

func init() {
	flag.StringVar(&destDir, "destDir", "communication-matrix", "Output files dir")
	flag.StringVar(&format, "format", "csv", "Desired format (json,yaml,csv,nft)")
	flag.StringVar(&envStr, "env", "baremetal", "Cluster environment (baremetal/cloud)")
	flag.StringVar(&deploymentStr, "deployment", "mno", "Deployment type (mno/sno)")
	flag.StringVar(&customEntriesPath, "customEntriesPath", "", "Add custom entries from a file to the matrix")
	flag.StringVar(&customEntriesFormat, "customEntriesFormat", "", "Set the format of the custom entries file (json,yaml,csv)")
	flag.Parse()
}

func main() {
	kubeconfig, ok := os.LookupEnv("KUBECONFIG")
	if !ok {
		panic("must set the KUBECONFIG environment variable")
	}

	var env commatrix.Env
	switch envStr {
	case "baremetal":
		env = commatrix.Baremetal
	case "cloud":
		env = commatrix.Cloud
	default:
		panic(fmt.Sprintf("invalid cluster environment: %s", envStr))
	}

	var deployment commatrix.Deployment
	switch deploymentStr {
	case "mno":
		deployment = commatrix.MNO
	case "sno":
		deployment = commatrix.SNO
	default:
		panic(fmt.Sprintf("invalid deployment type: %s", deploymentStr))
	}

	if customEntriesPath != "" && customEntriesFormat == "" {
		panic("error, variable customEntriesFormat is not set")
	}

	mat, ssMat, err := commatrix.GenerateMatrix(kubeconfig, customEntriesPath, customEntriesFormat, format, env, deployment, destDir)
	if err != nil {
		panic(fmt.Sprintf("Error while generating matrix :%v", err))
	}

	err = commatrix.WriteMatsToFiles(mat, ssMat, format, env, deployment, destDir)
	if err != nil {
		panic(fmt.Sprintf("Error while writing matrix to file: %v", err))
	}
}
