package commatrixcreator

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"

	log "github.com/sirupsen/logrus"

	dynamicranges "github.com/openshift-kni/commatrix/pkg/dynamic-ranges"
	"github.com/openshift-kni/commatrix/pkg/endpointslices"
	"github.com/openshift-kni/commatrix/pkg/mcp"
	"github.com/openshift-kni/commatrix/pkg/types"
	"github.com/openshift-kni/commatrix/pkg/utils"
	configv1 "github.com/openshift/api/config/v1"
)

type CommunicationMatrixCreator struct {
	exporter             *endpointslices.EndpointSlicesExporter
	customEntriesPath    string
	customEntriesFormat  string
	platformType         configv1.PlatformType
	controlPlaneTopology configv1.TopologyMode
	ipv6Enabled          bool
	dhcpEnabled          bool
	utilsHelpers         utils.UtilsInterface
}

type Option func(*CommunicationMatrixCreator)

func WithExporter(
	e *endpointslices.EndpointSlicesExporter,
) Option {
	return func(c *CommunicationMatrixCreator) {
		c.exporter = e
	}
}

func WithCustomEntries(path, format string) Option {
	return func(c *CommunicationMatrixCreator) {
		c.customEntriesPath = path
		c.customEntriesFormat = format
	}
}

func WithIPv6() Option {
	return func(c *CommunicationMatrixCreator) {
		c.ipv6Enabled = true
	}
}

func WithDHCP() Option {
	return func(c *CommunicationMatrixCreator) {
		c.dhcpEnabled = true
	}
}

func WithUtilsHelpers(u utils.UtilsInterface) Option {
	return func(c *CommunicationMatrixCreator) {
		c.utilsHelpers = u
	}
}

func New(
	platformType configv1.PlatformType,
	topology configv1.TopologyMode,
	opts ...Option,
) *CommunicationMatrixCreator {
	cm := &CommunicationMatrixCreator{
		platformType:         platformType,
		controlPlaneTopology: topology,
	}
	for _, o := range opts {
		o(cm)
	}
	return cm
}

// CreateEndpointMatrix initializes a ComMatrix using Kubernetes cluster data.
// It takes kubeconfigPath for cluster access to  fetch EndpointSlice objects,
// detailing open ports for ingress traffic.
// Custom entries from a JSON file can be added to the matrix by setting `customEntriesPath`.
// Returns a pointer to ComMatrix and error. Entries include traffic direction, protocol,
// port number, namespace, service name, pod, container, node role, and flow optionality for OpenShift.
func (cm *CommunicationMatrixCreator) CreateEndpointMatrix() (*types.ComMatrix, error) {
	log.Debug("Loading EndpointSlices information")
	err := cm.exporter.LoadExposedEndpointSlicesInfo()
	if err != nil {
		log.Errorf("Failed loading endpointslices: %v", err)
		return nil, fmt.Errorf("failed loading endpointslices: %w", err)
	}

	log.Debug("Converting EndpointSlices to ComDetails")
	epSliceComDetails, err := cm.exporter.ToComDetails()
	if err != nil {
		log.Errorf("Failed to convert endpoint slices: %v", err)
		return nil, err
	}

	log.Debug("Getting static entries")
	staticEntries, err := cm.getStaticEntries()
	if err != nil {
		log.Errorf("Failed adding static entries: %s", err)
		return nil, fmt.Errorf("failed adding static entries: %w", err)
	}

	// List of [master, worker] roles per pool for static entries expansion
	nodes, err := cm.utilsHelpers.ListNodes()
	if err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}

	PoolRolesForStaticEntriesExpansion := mcp.GetPoolRolesForStaticEntriesExpansion(nodes, cm.exporter.NodeToGroup())

	// Expand static entries for all MCPs based on their roles
	staticEntries = ExpandStaticEntriesByPool(staticEntries, PoolRolesForStaticEntriesExpansion)
	epSliceComDetails = append(epSliceComDetails, staticEntries...)

	dynamicRanges, err := dynamicranges.GetDynamicRanges(cm.exporter)
	if err != nil {
		log.Errorf("Failed to get dynamic ranges: %v", err)
		return nil, fmt.Errorf("failed to get dynamic ranges: %w", err)
	}

	if cm.customEntriesPath != "" {
		log.Debug("Loading custom entries from file")
		customMatrix, err := cm.GetComMatrixFromFile()
		if err != nil {
			log.Errorf("Failed adding custom entries: %s", err)
			return nil, fmt.Errorf("failed adding custom entries: %w", err)
		}
		epSliceComDetails = append(epSliceComDetails, customMatrix.Ports...)
		dynamicRanges = append(dynamicRanges, customMatrix.DynamicRanges...)
	}

	commMatrix := &types.ComMatrix{Ports: epSliceComDetails, DynamicRanges: dynamicRanges}
	log.Debug("Sorting ComMatrix and removing duplicates")
	commMatrix.SortAndRemoveDuplicates()
	return commMatrix, nil
}

func (cm *CommunicationMatrixCreator) GetComMatrixFromFile() (*types.ComMatrix, error) {
	log.Debugf("Opening file %s", cm.customEntriesPath)
	f, err := os.Open(filepath.Clean(cm.customEntriesPath))
	if err != nil {
		log.Errorf("Failed to open file %s: %v", cm.customEntriesPath, err)
		return nil, fmt.Errorf("failed to open file %s: %w", cm.customEntriesPath, err)
	}
	defer f.Close()

	log.Debugf("Reading file %s", cm.customEntriesPath)
	raw, err := io.ReadAll(f)
	if err != nil {
		log.Errorf("Failed to read file %s: %v", cm.customEntriesPath, err)
		return nil, fmt.Errorf("failed to read file %s: %w", cm.customEntriesPath, err)
	}

	log.Debugf("Unmarshalling file content with format %s", cm.customEntriesFormat)
	res, err := types.ParseToComMatrix(raw, cm.customEntriesFormat)
	if err != nil {
		log.Errorf("Failed to unmarshal %s file: %v", cm.customEntriesFormat, err)
		return nil, fmt.Errorf("failed to unmarshal custom entries file: %w", err)
	}

	log.Debug("Successfully unmarshalled custom entries")
	return res, nil
}

// getStaticEntries is a convenience wrapper around types.GetStaticEntries
// that forwards the creator's platform configuration.
func (cm *CommunicationMatrixCreator) getStaticEntries() ([]types.ComDetails, error) {
	return types.GetStaticEntries(cm.platformType, cm.controlPlaneTopology, cm.ipv6Enabled, cm.dhcpEnabled)
}

// expandStaticEntriesByPool uses MCP-derived role per pool.
func ExpandStaticEntriesByPool(staticEntries []types.ComDetails, poolToRoles map[string][]string) []types.ComDetails {
	if len(poolToRoles) == 0 {
		return staticEntries
	}
	out := make([]types.ComDetails, 0, len(staticEntries))
	for _, se := range staticEntries {
		for poolName, roles := range poolToRoles {
			// check membership in slice
			if slices.Contains(roles, se.NodeGroup) {
				dup := se
				dup.NodeGroup = poolName
				out = append(out, dup)
			}
		}
	}
	return out
}
