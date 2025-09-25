package generate

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"context"

	"github.com/openshift-kni/commatrix/pkg/client"
	commatrixcreator "github.com/openshift-kni/commatrix/pkg/commatrix-creator"
	"github.com/openshift-kni/commatrix/pkg/consts"
	"github.com/openshift-kni/commatrix/pkg/endpointslices"
	listeningsockets "github.com/openshift-kni/commatrix/pkg/listening-sockets"
	matrixdiff "github.com/openshift-kni/commatrix/pkg/matrix-diff"
	"github.com/openshift-kni/commatrix/pkg/utils"
	configv1 "github.com/openshift/api/config/v1"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/kubectl/pkg/util/templates"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

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
	cmd.Flags().StringVar(&o.format, "format", consts.FilesDefaultFormat, "Desired format (json,yaml,csv,nft)")
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

	if err := validateCustomEntries(o.customEntriesPath, o.customEntriesFormat, validCustomEntriesFormats); err != nil {
		return err
	}

	return nil
}

func validateCustomEntries(path, format string, validFormats []string) error {
	if path == "" && format == "" { // dont need to validate
		return nil
	}

	if path != "" && format == "" { // if one of them are missing
		return fmt.Errorf("you must specify the --customEntriesFormat when using --customEntriesPath")
	}

	if path == "" && format != "" { // if one of them are missing
		return fmt.Errorf("you must specify the --customEntriesPath when using --customEntriesFormat")
	}

	if !slices.Contains(validFormats, format) { // poth are set with wrong format
		return fmt.Errorf("invalid custom entries format '%s', valid options are: %s",
			format, strings.Join(validFormats, ", "))
	}

	return nil
}

func Complete(o *GenerateOptions) error {
	if o.debug {
		log.SetLevel(log.DebugLevel)
	}

	if o.destDir == "" {
		o.destDir = consts.CommatrixDefaultDir
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

	isSNO, err := o.utilsHelpers.IsSNOCluster()
	if err != nil {
		return fmt.Errorf("failed to check is sno cluster %s", err)
	}

	if isSNO {
		deployment = types.SNO
	}

	platformType, err := o.utilsHelpers.GetPlatformType()
	if err != nil {
		return fmt.Errorf("failed to get platform type %s", err)
	}

	if !slices.Contains(types.SupportedPlatforms, platformType) {
		return fmt.Errorf("unsupported platform type: %s. Supported platform types are: %v", platformType, types.SupportedPlatforms)
	}

	matrix, err := generateMatrix(o, deployment, platformType)
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
		err = o.utilsHelpers.WriteFile(filepath.Join(o.destDir, consts.MatrixDiffSSfileName), []byte(diffStr))
		if err != nil {
			return fmt.Errorf("error writing the diff matrix file: %v", err)
		}

		log.Debug("Matrix diff successfully written to file")
	}
	return nil
}

func generateMatrix(o *GenerateOptions, deployment types.Deployment, platformType configv1.PlatformType) (*types.ComMatrix, error) {
	if o.debug {
		log.SetLevel(log.DebugLevel)
	}

	epExporter, err := endpointslices.New(o.cs)
	if err != nil {
		return nil, fmt.Errorf("failed creating the endpointslices exporter %s", err)
	}

	log.Debug("Creating communication matrix")
	commMatrix, err := commatrixcreator.New(epExporter, o.customEntriesPath, o.customEntriesFormat, platformType, deployment)
	if err != nil {
		return nil, err
	}

	matrix, err := commMatrix.CreateEndpointMatrix()
	if err != nil {
		return nil, err
	}

	log.Debug("Writing endpoint matrix to file")
	err = matrix.WriteMatrixToFileByType(o.utilsHelpers, consts.CommatrixFileNamePrefix, o.format, deployment, o.destDir)
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
	// Always delete namespace and wait for full removal to avoid Terminating races on reruns
	defer func() {
		if delErr := o.utilsHelpers.DeleteNamespace(consts.DefaultDebugNamespace); delErr != nil {
			log.Warnf("failed to delete namespace %s: %v", consts.DefaultDebugNamespace, delErr)
			return
		}
		if pollErr := wait.PollUntilContextTimeout(context.TODO(), time.Second, 2*time.Minute, true, func(ctx context.Context) (bool, error) {
			ns := &corev1.Namespace{}
			err := o.cs.Get(context.TODO(), ctrlclient.ObjectKey{Name: consts.DefaultDebugNamespace}, ns)
			if apierrors.IsNotFound(err) {
				return true, nil
			}
			if err != nil {
				log.Warningf("retrying due to error: %v", err)
				return false, nil // keep retrying
			}
			return false, nil
		}); pollErr != nil {
			log.Errorf("error while waiting for namespace %s deletion: %v", consts.DefaultDebugNamespace, pollErr)
		}
	}()

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
	err = ssMat.WriteMatrixToFileByType(o.utilsHelpers, consts.SSMatrixFileNamePrefix, o.format, deployment, o.destDir)
	if err != nil {
		return nil, fmt.Errorf("error while writing SS matrix to file: %v", err)
	}

	return ssMat, nil
}
