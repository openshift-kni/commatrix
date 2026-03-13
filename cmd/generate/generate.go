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
	configv1 "github.com/openshift/api/config/v1"
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

			 # Generate Butane configs with nftables firewall rules (per node pool) and a NodeDisruptionPolicy patch:
			 oc commatrix generate --format butane

			 # Generate MachineConfig CRs with nftables firewall rules (per node pool) and a NodeDisruptionPolicy patch:
			 oc commatrix generate --format mc
	`)
)

var (
	validFormats = []string{
		types.FormatCSV,
		types.FormatJSON,
		types.FormatYAML,
		types.FormatNFT,
		types.FormatButane,
		types.FormatMC,
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
	cmd.Flags().StringVar(&o.format, "format", consts.FilesDefaultFormat, "Desired format (json,yaml,csv,nft,butane,mc)")
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
	controlPlaneTopology, err := o.utilsHelpers.GetControlPlaneTopology()
	if err != nil {
		return fmt.Errorf("failed to get control plane topology %s", err)
	}

	platformType, err := o.utilsHelpers.GetPlatformType()
	if err != nil {
		return fmt.Errorf("failed to get platform type %s", err)
	}

	if !slices.Contains(types.SupportedPlatforms, platformType) {
		return fmt.Errorf("unsupported platform type: %s. Supported platform types are: %v", platformType, types.SupportedPlatforms)
	}

	// Validate control plane topology (supports: HA, SNO, HyperShift External)
	if !types.IsSupportedTopology(controlPlaneTopology) {
		return fmt.Errorf("unsupported control plane topology: %s. Supported topologies are: %v", controlPlaneTopology, types.SupportedTopologiesList())
	}

	ipv6Enabled, err := o.utilsHelpers.IsIPv6Enabled()
	if err != nil {
		return fmt.Errorf("failed to detect IPv6: %v", err)
	}

	// DHCP is only supported on BareMetal and None platforms
	var dhcpEnabled bool
	if platformType == configv1.BareMetalPlatformType || platformType == configv1.NonePlatformType {
		dhcpEnabled, err = o.utilsHelpers.IsDHCPEnabled()
		if err != nil {
			return fmt.Errorf("failed to detect DHCP: %v", err)
		}
		if dhcpEnabled {
			log.Debug("DHCP enabled")
		}
	}

	// Generate the comm matrix, but do not write it, yet.
	matrix, err := generateMatrix(o, controlPlaneTopology, platformType, ipv6Enabled, dhcpEnabled)
	if err != nil {
		return fmt.Errorf("failed to generate endpoint slice matrix: %w", err)
	}

	// Generate the flow comm matrix and other info, but do not write it, yet.
	var ssResult *listeningsockets.SSResult
	if o.openPorts {
		if ssResult, err = generateSS(o); err != nil {
			return fmt.Errorf("failed to generate SS matrix: %w", err)
		}
	}

	// If format is all in one, merge the SS matrix and the normal matrix and write the result.
	if formatRequiresMerge(o) {
		return writeMergedMatrix(o, matrix, ssResult)
	}

	// Otherwise, write the matrix and ss result files individually.
	return writeMatrix(o, matrix, ssResult)
}

// writeMergedMatrix merges the communication matrix with the SS (listening sockets) matrix
// and writes the combined result to a single file. Used for formats that require all-in-one
// output (NFT, Butane, MachineConfig).
func writeMergedMatrix(o *GenerateOptions, matrix *types.ComMatrix, ssResult *listeningsockets.SSResult) error {
	if o == nil {
		return fmt.Errorf("writeMergedMatrix called with nil GenerateOptions")
	}
	if matrix == nil {
		return fmt.Errorf("writeMergedMatrix called with nil ComMatrix")
	}

	if ssResult != nil {
		log.Debug("Merging matrix and ss matrix")
		matrix = matrix.Merge(ssResult.SSCommMatrix)
	}

	log.Debug("Writing endpoint matrix to file")
	if err := matrix.WriteMatrixToFileByType(o.utilsHelpers, fileNamePrefix(o.format, consts.CommatrixFileNamePrefix),
		o.format, o.destDir); err != nil {
		return fmt.Errorf("failed to write endpoint matrix to file: %w", err)
	}
	return nil
}

// writeMatrix writes the communication matrix and optionally the SS (listening sockets) results
// as separate files. This includes the main matrix file, SS raw files, SS matrix file, and
// a diff file comparing the two matrices (when ssResult is provided).
func writeMatrix(o *GenerateOptions, matrix *types.ComMatrix, ssResult *listeningsockets.SSResult) error {
	if o == nil {
		return fmt.Errorf("writeMatrix called with nil GenerateOptions")
	}
	if matrix == nil {
		return fmt.Errorf("writeMatrix called with nil ComMatrix")
	}

	log.Debug("Writing endpoint matrix to file")
	if err := matrix.WriteMatrixToFileByType(o.utilsHelpers, fileNamePrefix(o.format, consts.CommatrixFileNamePrefix),
		o.format, o.destDir); err != nil {
		return fmt.Errorf("failed to write endpoint matrix to file: %w", err)
	}

	if ssResult == nil {
		return nil
	}

	log.Debug("Writing SS raw files")
	if err := ssResult.WriteSSRawFiles(o.utilsHelpers, o.destDir); err != nil {
		return fmt.Errorf("error while writing the SS raw files: %w", err)
	}

	log.Debug("Writing SS matrix to file")
	if err := ssResult.SSCommMatrix.WriteMatrixToFileByType(
		o.utilsHelpers, fileNamePrefix(o.format, consts.SSMatrixFileNamePrefix), o.format, o.destDir); err != nil {
		return fmt.Errorf("error while writing SS matrix to file: %w", err)
	}

	log.Debug("Generating diff between the endpoint slice and SS matrix")
	diff := matrixdiff.Generate(matrix, ssResult.SSCommMatrix)
	diffStr, err := diff.String()
	if err != nil {
		return fmt.Errorf("error while generating matrix diff string: %w", err)
	}

	log.Debug("Writing the matrix diff to file")
	if err := o.utilsHelpers.WriteFile(filepath.Join(o.destDir, consts.MatrixDiffSSfileName),
		[]byte(diffStr)); err != nil {
		return fmt.Errorf("error writing the diff matrix file: %w", err)
	}

	log.Debug("Matrix diff successfully written to file")
	return nil
}

func generateMatrix(o *GenerateOptions, controlPlaneTopology configv1.TopologyMode, platformType configv1.PlatformType, ipv6Enabled bool, dhcpEnabled bool) (*types.ComMatrix, error) {
	if o.debug {
		log.SetLevel(log.DebugLevel)
	}

	epExporter, err := endpointslices.New(o.cs)
	if err != nil {
		return nil, fmt.Errorf("failed creating the endpointslices exporter %s", err)
	}

	log.Debug("Creating communication matrix")
	opts := []commatrixcreator.Option{
		commatrixcreator.WithExporter(epExporter),
		commatrixcreator.WithUtilsHelpers(o.utilsHelpers),
	}
	if o.customEntriesPath != "" {
		opts = append(opts,
			commatrixcreator.WithCustomEntries(
				o.customEntriesPath, o.customEntriesFormat,
			),
		)
	}
	if ipv6Enabled {
		opts = append(opts, commatrixcreator.WithIPv6())
	}
	if dhcpEnabled {
		opts = append(opts, commatrixcreator.WithDHCP())
	}
	commMatrix, err := commatrixcreator.New(
		platformType, controlPlaneTopology, opts...,
	)
	if err != nil {
		return nil, err
	}

	matrix, err := commMatrix.CreateEndpointMatrix()
	if err != nil {
		return nil, err
	}

	return matrix, nil
}

func generateSS(o *GenerateOptions) (*listeningsockets.SSResult, error) {
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
		}
	}()

	log.Debug("Generating SS matrix and raw files")
	result, err := listeningCheck.GenerateSS(consts.DefaultDebugNamespace)
	if err != nil {
		return nil, fmt.Errorf("error while generating the listening check matrix and ss raws: %v", err)
	}
	return result, nil
}

func fileNamePrefix(format, defaultPrefix string) string {
	switch format {
	case types.FormatButane:
		return consts.ButaneFileNamePrefix
	case types.FormatMC:
		return consts.MCFileNamePrefix
	default:
		return defaultPrefix
	}
}

// formatRequiresMerge returns true for formats that combine both static and open ports
// into a single output (Butane and MachineConfig formats).
func formatRequiresMerge(o *GenerateOptions) bool {
	return o.format == types.FormatButane || o.format == types.FormatMC
}
