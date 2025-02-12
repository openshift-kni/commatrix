package generate

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/openshift-kni/commatrix/pkg/client"
	commatrixcreator "github.com/openshift-kni/commatrix/pkg/commatrix-creator"
	"github.com/openshift-kni/commatrix/pkg/consts"
	"github.com/openshift-kni/commatrix/pkg/endpointslices"
	listeningsockets "github.com/openshift-kni/commatrix/pkg/listening-sockets"
	matrixdiff "github.com/openshift-kni/commatrix/pkg/matrix-diff"
	"github.com/openshift-kni/commatrix/pkg/utils"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/kubectl/pkg/util/templates"

	"github.com/openshift-kni/commatrix/pkg/types"
)

var (
	commatrixLong = templates.LongDesc(`
              Generate an up-to-date communication flows matrix for all ingress flows of openshift (multi-node and single-node in OpenShift) and Operators.

              Optionally, generate a host open ports matrix and the difference with the communication matrix.
			  
              For additional details, please refer to the communication matrix documentation(https://github.com/openshift-kni/commatrix/blob/main/README.md).

	`)
	CommatrixExample = templates.Examples(`
			 # Generate the communication matrix in default format (csv):
			 oc commatrix generate  
			 
			 # Generate the communication matrix in nft format:
			 oc commatrix generate --format nft 
			 
			 # Generate the communication matrix in json format with debug logs:
			 oc commatrix generate --format json --debug
			 
			 # Generate communication matrix, host open ports matrix, and their difference in yaml format:
			 oc commatrix generate --host-open-ports --format yaml 
			 
			 # Generate the communication matrix in json format with custom entries:
			 oc commatrix generate --format json --customEntriesPath /path/to/customEntriesFile --customEntriesFormat json
			 
	`)
)

var (
	validFormats = []string{
		types.FormatCSV,
		types.FormatJSON,
		types.FormatYAML,
		types.FormatNFT,
	}

	validCustomEntriesFormats = []string{
		types.FormatCSV,
		types.FormatJSON,
		types.FormatYAML,
	}
)

type GenerateOptions struct {
	destDir             string
	format              string
	customEntriesPath   string
	customEntriesFormat string
	debug               bool
	openPorts           bool
	cs                  *client.ClientSet
	utilsHelpers        utils.UtilsInterface
	configFlags         *genericclioptions.ConfigFlags
	genericiooptions.IOStreams
}

func NewCmd(cs *client.ClientSet, streams genericiooptions.IOStreams) *cobra.Command {
	// Parent command to which all subcommands are added.
	cmds := &cobra.Command{
		Use:   "commatrix",
		Short: "Run communication matrix commands.",
		Long:  commatrixLong,
	}
	cmds.AddCommand(NewCmdCommatrixGenerate(cs, streams))

	return cmds
}

func NewCommatrixOptions(streams genericiooptions.IOStreams, cs *client.ClientSet) *GenerateOptions {
	return &GenerateOptions{
		configFlags:  genericclioptions.NewConfigFlags(true),
		IOStreams:    streams,
		cs:           cs,
		utilsHelpers: utils.New(cs),
	}
}

// NewCmdAddRoleToUser implements the OpenShift cli add-role-to-user command.
func NewCmdCommatrixGenerate(cs *client.ClientSet, streams genericiooptions.IOStreams) *cobra.Command {
	o := NewCommatrixOptions(streams, cs)
	cmd := &cobra.Command{
		Use:     "generate",
		Short:   "Generate an up-to-date communication flows matrix for all ingress flows.",
		Long:    commatrixLong,
		Example: CommatrixExample,
		RunE: func(c *cobra.Command, args []string) (err error) {
			if err := Validate(o); err != nil {
				return err
			}

			if err := Complete(o); err != nil {
				return err
			}

			if err := Run(o); err != nil {
				return fmt.Errorf("failed to generate matrix: %v", err)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&o.destDir, "destDir", "", "Output files dir (default communication-matrix)")
	cmd.Flags().StringVar(&o.format, "format", "csv", "Desired format (json,yaml,csv,nft)")
	cmd.Flags().BoolVar(&o.debug, "debug", false, "Debug logs")
	cmd.Flags().StringVar(&o.customEntriesPath, "customEntriesPath", "", "Add custom entries from a file to the matrix")
	cmd.Flags().StringVar(&o.customEntriesFormat, "customEntriesFormat", "", "Set the format of the custom entries file (json,yaml,csv)")
	cmd.Flags().BoolVar(&o.openPorts, "host-open-ports", false, "Generate communication matrix, host open ports matrix, and their difference")

	return cmd
}

func Validate(o *GenerateOptions) error {
	if !slices.Contains(validFormats, o.format) {
		return fmt.Errorf("invalid format '%s', valid options are: %s",
			o.format, strings.Join(validFormats, ", "))
	}

	if o.customEntriesPath == "" {
		return nil
	}

	if o.customEntriesFormat == "" {
		return fmt.Errorf("you must specify the --customEntriesFormat when using --customEntriesPath")
	}

	if !slices.Contains(validCustomEntriesFormats, o.customEntriesFormat) {
		return fmt.Errorf("invalid custom entries format '%s', valid options are: %s",
			o.customEntriesFormat, strings.Join(validCustomEntriesFormats, ", "))
	}

	return nil
}

func Complete(o *GenerateOptions) error {
	if o.debug {
		log.SetLevel(log.DebugLevel)
	}

	if o.destDir == "" {
		o.destDir = "communication-matrix"
		log.Debugf("Creating communication-matrix default path: %s", o.destDir)
		if err := os.MkdirAll(o.destDir, 0755); err != nil {
			return fmt.Errorf("failed to create destination directory '%s': %v", o.destDir, err)
		}
	}

	return nil
}

func Run(o *GenerateOptions) (err error) {
	if o.debug {
		log.SetLevel(log.DebugLevel)
	}

	log.Debug("Detecting deployment and infra types")
	deployment := types.Standard
	infra := types.Cloud

	isSNO, err := o.utilsHelpers.IsSNOCluster()
	if err != nil {
		return fmt.Errorf("failed to check is sno cluster %s", err)
	}

	if isSNO {
		deployment = types.SNO
	}

	isBM, err := o.utilsHelpers.IsBMInfra()
	if err != nil {
		return fmt.Errorf("failed to check is bm cluster %s", err)
	}

	if isBM {
		infra = types.Baremetal
	}

	matrix, err := generateMatrix(o, deployment, infra)
	if err != nil {
		return fmt.Errorf("failed to generate endpoint slice matrix: %v", err)
	}

	if o.openPorts {
		ssMat, err := generateSS(o, deployment)
		if err != nil {
			return fmt.Errorf("failed to generate SS matrix: %v", err)
		}

		log.Debug("Generating diff between the endpoint slice and SS matrix")
		diff := matrixdiff.Generate(matrix, ssMat)
		diffStr, err := diff.String()
		if err != nil {
			return fmt.Errorf("error while generating matrix diff string: %v", err)
		}

		log.Debug("Writing the matrix diff to file")
		err = o.utilsHelpers.WriteFile(filepath.Join(o.destDir, "matrix-diff-ss"), []byte(diffStr))
		if err != nil {
			return fmt.Errorf("error writing the diff matrix file: %v", err)
		}

		log.Debug("Matrix diff successfully written to file")
	}
	return nil
}

func generateMatrix(o *GenerateOptions, deployment types.Deployment, infra types.Env) (*types.ComMatrix, error) {
	if o.debug {
		log.SetLevel(log.DebugLevel)
	}

	epExporter, err := endpointslices.New(o.cs)
	if err != nil {
		return nil, fmt.Errorf("failed creating the endpointslices exporter %s", err)
	}

	log.Debug("Creating communication matrix")
	commMatrix, err := commatrixcreator.New(epExporter, o.customEntriesPath, o.customEntriesFormat, infra, deployment)
	if err != nil {
		return nil, err
	}

	matrix, err := commMatrix.CreateEndpointMatrix()
	if err != nil {
		return nil, err
	}

	log.Debug("Writing endpoint matrix to file")
	err = matrix.WriteMatrixToFileByType(o.utilsHelpers, "communication-matrix", o.format, deployment, o.destDir)
	if err != nil {
		return nil, fmt.Errorf("failed to write endpoint matrix to file: %v", err)
	}

	return matrix, nil
}

func generateSS(o *GenerateOptions, deployment types.Deployment) (*types.ComMatrix, error) {
	if o.debug {
		log.SetLevel(log.DebugLevel)
	}

	log.Debug("Creating listening socket check")
	listeningCheck, err := listeningsockets.NewCheck(o.cs, o.utilsHelpers, o.destDir)
	if err != nil {
		return nil, fmt.Errorf("failed creating listening socket check: %v", err)
	}

	log.Debug("Creating namespace")
	err = o.utilsHelpers.CreateNamespace(consts.DefaultDebugNamespace)
	if err != nil {
		return nil, fmt.Errorf("failed to create namespace: %v", err)
	}

	log.Debug("Generating SS matrix and raw files")
	ssMat, ssOutTCP, ssOutUDP, err := listeningCheck.GenerateSS(consts.DefaultDebugNamespace)
	if err != nil {
		return nil, fmt.Errorf("error while generating the listening check matrix and ss raws: %v", err)
	}

	log.Debug("Writing SS raw files")
	err = listeningCheck.WriteSSRawFiles(ssOutTCP, ssOutUDP)
	if err != nil {
		return nil, fmt.Errorf("error while writing the SS raw files: %v", err)
	}

	log.Debug("Writing SS matrix to file")
	err = ssMat.WriteMatrixToFileByType(o.utilsHelpers, "ss-generated-matrix", o.format, deployment, o.destDir)
	if err != nil {
		return nil, fmt.Errorf("error while writing SS matrix to file: %v", err)
	}

	return ssMat, nil
}
