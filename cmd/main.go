package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

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
	// generate the endpointslice matrix
	mat, err := commatrix.New(kubeconfig, customEntriesPath, customEntriesFormat, env, deployment)
	if err != nil {
		panic(fmt.Errorf("failed to create the communication matrix: %s", err))
	}
	// write the endpoint slice matrix to file
	err = commatrix.WriteMatrixToFileByType(*mat, "communication-matrix", format, deployment, destDir)
	if err != nil {
		panic(fmt.Sprintf("Error while writing the endpoint slice matrix to file :%v", err))
	}
	// generate the ss matrix and ss raws
	ssMat, ssOutTCP, ssOutUDP, err := commatrix.GenerateSS(kubeconfig, customEntriesPath, customEntriesFormat, format, env, deployment, destDir)
	if err != nil {
		panic(fmt.Sprintf("Error while generating the ss matrix and ss raws :%v", err))
	}
	// write ss raw files
	err = commatrix.WriteSSRawFiles(destDir, ssOutTCP, ssOutUDP)
	if err != nil {
		panic(fmt.Sprintf("Error while writing the ss raw files :%v", err))
	}
	// write the ss matrix to file
	err = commatrix.WriteMatrixToFileByType(*ssMat, "ss-generated-matrix", format, deployment, destDir)
	if err != nil {
		panic(fmt.Sprintf("Error while writing ss matrix to file :%v", err))
	}
	// generate the diff matrix between the ss and the enpointslice matrix
	diff, err := commatrix.GenerateMatrixDiff(*mat, *ssMat)
	if err != nil {
		panic(fmt.Sprintf("Error while writing matrix diff file :%v", err))
	}

	// write the diff matrix between the ss and the enpointslice matrix to a csv file
	err = os.WriteFile(filepath.Join(destDir, "matrix-diff-ss"), []byte(diff), 0644)
	if err != nil {
		panic(fmt.Sprintf("Error writing the diff matrix :%v", err))
	}
}
