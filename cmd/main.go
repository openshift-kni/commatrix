package main

import (
	"flag"
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
	if debug {
		log.SetLevel(log.DebugLevel)
	}

	env, err := types.GetEnv(envStr)
	if err != nil {
		log.Panicf("Failed to get environment: %v", err)
	}

	deployment, err := types.GetDeployment(deploymentStr)
	if err != nil {
		log.Panicf("Failed to get deployment: %v", err)
	}

	if customEntriesPath != "" && customEntriesFormat == "" {
		log.Panic("Error: variable customEntriesFormat is not set")
	}

	cs, err := client.New()
	if err != nil {
		log.Panicf("Failed creating the k8s client: %v", err)
	}
	log.Debug("K8s client created successfully")

	utilsHelpers := utils.New(cs)
	log.Debug("Utils helpers initialized")

	epExporter, err := endpointslices.New(cs)
	if err != nil {
		log.Panicf("Failed creating the endpointslices exporter: %v", err)
	}
	log.Debug("EndpointSlices exporter created")

	log.Debug("Creating communication matrix")
	commMatrix, err := commatrixcreator.New(epExporter, customEntriesPath, customEntriesFormat, env, deployment)
	if err != nil {
		log.Panicf("Failed creating comm matrix creator: %v", err)
	}

	log.Debug("Generating endpoint matrix")
	matrix, err := commMatrix.CreateEndpointMatrix()
	if err != nil {
		log.Panicf("Failed creating endpoint matrix: %v", err)
	}

	log.Debug("Writing endpoint matrix to file")
	err = matrix.WriteMatrixToFileByType(utilsHelpers, "communication-matrix", format, deployment, destDir)
	if err != nil {
		log.Panicf("Failed to write endpoint matrix to file: %v", err)
	}

	log.Debug("Creating listening socket check")
	listeningCheck, err := listeningsockets.NewCheck(cs, utilsHelpers, destDir)
	if err != nil {
		log.Panicf("Failed creating listening socket check: %v", err)
	}

	log.Debug("Generating SS matrix and raw files")
	ssMat, ssOutTCP, ssOutUDP, err := listeningCheck.GenerateSS()
	if err != nil {
		log.Panicf("Error while generating the listening check matrix and ss raws: %v", err)
	}

	log.Debug("Writing SS raw files")
	err = listeningCheck.WriteSSRawFiles(ssOutTCP, ssOutUDP)
	if err != nil {
		log.Panicf("Error while writing the SS raw files: %v", err)
	}

	log.Debug("Writing SS matrix to file")
	err = ssMat.WriteMatrixToFileByType(utilsHelpers, "ss-generated-matrix", format, deployment, destDir)
	if err != nil {
		log.Panicf("Error while writing SS matrix to file: %v", err)
	}

	log.Debug("Generating diff between the endpoint slice and SS matrix")
	diff, err := matrix.GenerateDiff(ssMat)
	if err != nil {
		log.Panicf("Error while generating matrix diff: %v", err)
	}

	log.Debug("Writing the matrix diff to file")
	err = utilsHelpers.WriteFile(filepath.Join(destDir, "matrix-diff-ss"), []byte(diff))
	if err != nil {
		log.Panicf("Error writing the diff matrix file: %v", err)
	}

	log.Debug("Matrix diff successfully written to file")
}
