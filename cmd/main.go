package main

import (
	"flag"
	"fmt"
	"path/filepath"

	log "github.com/sirupsen/logrus"

	"github.com/openshift-kni/commatrix/pkg/client"
	commatrixcreator "github.com/openshift-kni/commatrix/pkg/commatrix-creator"
	"github.com/openshift-kni/commatrix/pkg/endpointslices"
	listeningsockets "github.com/openshift-kni/commatrix/pkg/listening-sockets"
	"github.com/openshift-kni/commatrix/pkg/types"
	"github.com/openshift-kni/commatrix/pkg/utils"
)

var (
	destDir             string
	format              string
	envStr              string
	deploymentStr       string
	customEntriesPath   string
	customEntriesFormat string
	debug               bool
)

func init() {
	flag.StringVar(&destDir, "destDir", "communication-matrix", "Output files dir")
	flag.StringVar(&format, "format", "csv", "Desired format (json,yaml,csv,nft)")
	flag.StringVar(&envStr, "env", "baremetal", "Cluster environment (baremetal/cloud)")
	flag.StringVar(&deploymentStr, "deployment", "mno", "Deployment type (mno/sno)")
	flag.StringVar(&customEntriesPath, "customEntriesPath", "", "Add custom entries from a file to the matrix")
	flag.StringVar(&customEntriesFormat, "customEntriesFormat", "", "Set the format of the custom entries file (json,yaml,csv)")
	flag.BoolVar(&debug, "debug", false, "Debug logs")
	flag.Parse()
}

func main() {
	env, err := types.GetEnv(envStr)
	if err != nil {
		panic(err)
	}

	deployment, err := types.GetDeployment(deploymentStr)
	if err != nil {
		panic(err)
	}

	if debug {
		log.SetLevel(log.DebugLevel)
	}

	if customEntriesPath != "" && customEntriesFormat == "" {
		panic("error, variable customEntriesFormat is not set")
	}

	cs, err := client.New()
	if err != nil {
		panic(fmt.Errorf("failed creating the k8s client: %w", err))
	}

	utilsHelpers := utils.New(cs)

	epExporter, err := endpointslices.New(cs)
	if err != nil {
		panic(fmt.Errorf("failed creating the endpointslices exporter: %w", err))
	}

	commMatrix, err := commatrixcreator.New(epExporter, customEntriesPath, customEntriesFormat, env, deployment)
	if err != nil {
		panic(fmt.Errorf("failed creating comm matrix creator: %w", err))
	}

	matrix, err := commMatrix.CreateEndpointMatrix()
	if err != nil {
		panic(fmt.Errorf("failed creating endpoint matrix: %w", err))
	}

	err = matrix.WriteMatrixToFileByType(utilsHelpers, "communication-matrix", format, deployment, destDir)
	if err != nil {
		panic(fmt.Errorf("failed to write endpoint matrix to file: %w", err))
	}

	listeningCheck, err := listeningsockets.NewCheck(cs, utilsHelpers, destDir)
	if err != nil {
		panic(fmt.Errorf("failed creating listening socket check: %w", err))
	}

	// generate the ss matrix and ss raws
	ssMat, ssOutTCP, ssOutUDP, err := listeningCheck.GenerateSS()
	if err != nil {
		panic(fmt.Sprintf("Error while generating the listening check matrix and ss raws :%v", err))
	}

	err = listeningCheck.WriteSSRawFiles(ssOutTCP, ssOutUDP)
	if err != nil {
		panic(fmt.Sprintf("Error while writing the ss raw files :%v", err))
	}

	err = ssMat.WriteMatrixToFileByType(utilsHelpers, "ss-generated-matrix", format, deployment, destDir)
	if err != nil {
		panic(fmt.Sprintf("Error while writing ss matrix to file :%v", err))
	}

	// generate the diff between the enpointslice and the ss matrix
	diff, err := matrix.GenerateDiff(ssMat)
	if err != nil {
		panic(fmt.Sprintf("Error while generating matrix diff :%v", err))
	}
	err = utilsHelpers.WriteFile(filepath.Join(destDir, "matrix-diff-ss"), []byte(diff))
	if err != nil {
		panic(fmt.Sprintf("Error writing the diff matrix file: %v", err))
	}
}
