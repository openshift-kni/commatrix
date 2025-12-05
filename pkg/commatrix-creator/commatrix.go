package commatrixcreator

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift-kni/commatrix/pkg/consts"
	"github.com/openshift-kni/commatrix/pkg/endpointslices"
	"github.com/openshift-kni/commatrix/pkg/mcp"
	"github.com/openshift-kni/commatrix/pkg/types"
	"github.com/openshift-kni/commatrix/pkg/utils"
	configv1 "github.com/openshift/api/config/v1"
	clientOptions "sigs.k8s.io/controller-runtime/pkg/client"
)

type CommunicationMatrixCreator struct {
	exporter            *endpointslices.EndpointSlicesExporter
	customEntriesPath   string
	customEntriesFormat string
	platformType        configv1.PlatformType
	deployment          types.Deployment
	ipv6Enabled         bool
	utilsHelpers        utils.UtilsInterface
}

func New(exporter *endpointslices.EndpointSlicesExporter, customEntriesPath string, customEntriesFormat string, platformType configv1.PlatformType, deployment types.Deployment, ipv6Enabled bool, utilsHelpers utils.UtilsInterface) (*CommunicationMatrixCreator, error) {
	return &CommunicationMatrixCreator{
		exporter:            exporter,
		customEntriesPath:   customEntriesPath,
		customEntriesFormat: customEntriesFormat,
		platformType:        platformType,
		deployment:          deployment,
		ipv6Enabled:         ipv6Enabled,
		utilsHelpers:        utilsHelpers,
	}, nil
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
	staticEntries, err := cm.GetStaticEntries()
	if err != nil {
		log.Errorf("Failed adding static entries: %s", err)
		return nil, fmt.Errorf("failed adding static entries: %s", err)
	}

	// List of [master, worker] roles per pool for static entries expansion
	PoolRolesForStaticEntriesExpansion, err := mcp.GetPoolRolesForStaticEntriesExpansion(cm.exporter.ClientSet, cm.exporter.NodeToGroup())
	if err != nil {
		log.Errorf("Failed to extract pool to roles: %v", err)
		return nil, err
	}

	// Expand static entries for all MCPs based on their roles
	staticEntries = expandStaticEntriesByPool(staticEntries, PoolRolesForStaticEntriesExpansion)
	epSliceComDetails = append(epSliceComDetails, staticEntries...)

	var customMatrix *types.ComMatrix
	if cm.customEntriesPath != "" {
		log.Debug("Loading custom entries from file")
		customMatrix, err = cm.GetComMatrixFromFile()
		if err != nil {
			log.Errorf("Failed adding custom entries: %s", err)
			return nil, fmt.Errorf("failed adding custom entries: %s", err)
		}
		epSliceComDetails = append(epSliceComDetails, customMatrix.Matrix...)
	}

	dynamicRanges, err := cm.getDynamicRanges()
	if err != nil {
		log.Errorf("Failed to get dynamic ranges: %v", err)
		return nil, fmt.Errorf("failed to get dynamic ranges: %w", err)
	}
	if customMatrix != nil && len(customMatrix.DynamicRanges) > 0 {
		dynamicRanges = append(dynamicRanges, customMatrix.DynamicRanges...)
	}

	commMatrix := &types.ComMatrix{Matrix: epSliceComDetails, DynamicRanges: dynamicRanges}
	log.Debug("Sorting ComMatrix and removing duplicates")
	commMatrix.SortAndRemoveDuplicates()
	return commMatrix, nil
}

func (cm *CommunicationMatrixCreator) GetComMatrixFromFile() (*types.ComMatrix, error) {
	log.Debugf("Opening file %s", cm.customEntriesPath)
	f, err := os.Open(filepath.Clean(cm.customEntriesPath))
	if err != nil {
		log.Errorf("Failed to open file %s: %v", cm.customEntriesPath, err)
		return nil, fmt.Errorf("failed to open file %s: %v", cm.customEntriesPath, err)
	}
	defer f.Close()

	log.Debugf("Reading file %s", cm.customEntriesPath)
	raw, err := io.ReadAll(f)
	if err != nil {
		log.Errorf("Failed to read file %s: %v", cm.customEntriesPath, err)
		return nil, fmt.Errorf("failed to read file %s: %v", cm.customEntriesPath, err)
	}

	log.Debugf("Unmarshalling file content with format %s", cm.customEntriesFormat)
	res, err := types.ParseToComMatrix(raw, cm.customEntriesFormat)
	if err != nil {
		log.Errorf("Failed to unmarshal %s file: %v", cm.customEntriesFormat, err)
		return nil, fmt.Errorf("failed to unmarshal custom entries file: %v", err)
	}

	log.Debug("Successfully unmarshalled custom entries")
	return res, nil
}

func (cm *CommunicationMatrixCreator) GetStaticEntries() ([]types.ComDetails, error) {
	log.Debug("Determining static entries based on environment and deployment")
	comDetails := []types.ComDetails{}

	switch cm.platformType {
	case configv1.BareMetalPlatformType:
		log.Debug("Adding Baremetal static entries")
		comDetails = append(comDetails, types.BaremetalStaticEntriesMaster...)
		if cm.deployment == types.SNO {
			break
		}
		comDetails = append(comDetails, types.BaremetalStaticEntriesWorker...)
	case configv1.AWSPlatformType:
		log.Debug("There are no Cloud static entries to be added")
	case configv1.NonePlatformType:
		break
	default:
		log.Errorf("Invalid value for cluster environment: %v", cm.platformType)
		return nil, fmt.Errorf("invalid value for cluster environment")
	}

	log.Debug("Adding general static entries")
	comDetails = append(comDetails, types.GeneralStaticEntriesMaster...)
	if cm.ipv6Enabled {
		comDetails = append(comDetails, types.GeneralIPv6StaticEntriesMaster...)
	}
	if cm.deployment == types.SNO {
		return comDetails, nil
	}

	comDetails = append(comDetails, types.StandardStaticEntries...)
	comDetails = append(comDetails, types.GeneralStaticEntriesWorker...)
	if cm.ipv6Enabled {
		comDetails = append(comDetails, types.GeneralIPv6StaticEntriesWorker...)
	}
	log.Debug("Successfully determined static entries")
	return comDetails, nil
}

// expandStaticEntriesByPool uses MCP-derived role per pool.
func expandStaticEntriesByPool(staticEntries []types.ComDetails, poolToRoles map[string][]string) []types.ComDetails {
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

func (cm *CommunicationMatrixCreator) getDynamicRanges() ([]types.DynamicRange, error) {
	log.Debug("Getting dynamic ranges")
	dynamicRanges := types.HostLevelServicesDynamicRange

	nodePortDynamicRange, err := cm.getNodePortDynamicRange()
	if err != nil {
		log.Errorf("Failed to get node port dynamic range: %v", err)
		return nil, fmt.Errorf("failed to get node port dynamic range: %w", err)
	}
	dynamicRanges = append(dynamicRanges, nodePortDynamicRange...)

	linuxDynamicPrivateRange, err := cm.getLinuxDynamicPrivateRange()
	if err != nil {
		log.Errorf("Failed to get Linux dynamic private range: %v", err)
		return nil, fmt.Errorf("failed to get Linux dynamic private range: %w", err)
	}
	dynamicRanges = append(dynamicRanges, linuxDynamicPrivateRange...)

	return dynamicRanges, nil
}

// GetNodePortDynamicRange returns the cluster's Service NodePort range as dynamic ranges.
// If the cluster does not define a custom range, it falls back to the Kubernetes default (30000-32767).
func (cm *CommunicationMatrixCreator) getNodePortDynamicRange() ([]types.DynamicRange, error) {
	log.Debug("Getting node port dynamic range")
	network := &configv1.Network{}
	if err := cm.exporter.Get(context.TODO(), clientOptions.ObjectKey{Name: "cluster"}, network); err != nil {
		log.Errorf("Failed to get Network config: %v", err)
		return nil, fmt.Errorf("failed to get Network config: %w", err)
	}

	dr := types.KubeletNodePortDefaultDynamicRange
	rangeStr := strings.TrimSpace(network.Spec.ServiceNodePortRange)
	if rangeStr == "" {
		log.Debug("ServiceNodePortRange not set; using default")
		return dr, nil
	}

	minPort, maxPort, err := parsePortRange(rangeStr)
	if err != nil {
		log.Errorf("Invalid ServiceNodePortRange format %q: %v", rangeStr, err)
		return nil, fmt.Errorf("invalid ServiceNodePortRange format %q: %w", rangeStr, err)
	}

	for i := range dr {
		dr[i].MinPort = minPort
		dr[i].MaxPort = maxPort
	}
	return dr, nil
}

// getLinuxDynamicPrivateRange retrieves the Linux dynamic/private port range from a cluster node
// by reading the host sysctl:
//   - /proc/sys/net/ipv4/ip_local_port_range
func (cm *CommunicationMatrixCreator) getLinuxDynamicPrivateRange() ([]types.DynamicRange, error) {
	log.Debug("Getting Linux dynamic/private port range from cluster")

	// Pick an arbitrary node to query (ranges are expected to be consistent across nodes).
	nodes, err := cm.exporter.CoreV1Interface.Nodes().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		log.Errorf("Failed to list nodes: %v", err)
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}
	if len(nodes.Items) == 0 {
		return nil, fmt.Errorf("no nodes found in the cluster")
	}
	nodeName := nodes.Items[0].Name

	// Ensure namespace exists.
	if err := cm.utilsHelpers.CreateNamespace(consts.DefaultDebugNamespace); err != nil {
		log.Errorf("Failed to create debug namespace: %v", err)
		return nil, fmt.Errorf("failed to create debug namespace: %w", err)
	}
	defer func() {
		if delErr := cm.utilsHelpers.DeleteNamespace(consts.DefaultDebugNamespace); delErr != nil {
			log.Warnf("failed to delete namespace %s: %v", consts.DefaultDebugNamespace, delErr)
		}
	}()

	// Create a privileged pod on the selected node.
	pod, err := cm.utilsHelpers.CreatePodOnNode(nodeName, consts.DefaultDebugNamespace, consts.DefaultDebugPodImage, []string{})
	if err != nil {
		log.Errorf("Failed to create debug pod: %v", err)
		return nil, fmt.Errorf("failed to create debug pod: %w", err)
	}
	defer func() {
		if delErr := cm.utilsHelpers.DeletePod(pod); delErr != nil {
			log.Warnf("failed to delete debug pod %s: %v", pod.Name, delErr)
		}
	}()

	// Wait for the pod to be running.
	if err := cm.utilsHelpers.WaitForPodStatus(consts.DefaultDebugNamespace, pod, corev1.PodRunning); err != nil {
		log.Errorf("Debug pod did not reach Running state: %v", err)
		return nil, fmt.Errorf("debug pod did not reach Running state: %w", err)
	}

	// Read IPv4 range (applies to both IPv4 and IPv6 ephemeral ports).
	out, err := cm.utilsHelpers.RunCommandOnPod(pod, []string{"/bin/sh", "-c", "cat /host/proc/sys/net/ipv4/ip_local_port_range"})
	if err != nil {
		log.Errorf("Failed to read IPv4 ip_local_port_range: %v", err)
		return nil, fmt.Errorf("failed to read IPv4 ip_local_port_range: %w", err)
	}
	minPort, maxPort, err := parsePortRange(string(out))
	if err != nil {
		log.Errorf("Failed to parse IPv4 ip_local_port_range output: %v", err)
		return nil, fmt.Errorf("failed to parse IPv4 ip_local_port_range: %w", err)
	}

	return []types.DynamicRange{
		{
			Direction:   "Ingress",
			Protocol:    "TCP",
			MinPort:     minPort,
			MaxPort:     maxPort,
			Description: "Linux dynamic/private ports",
			Optional:    true,
		},
		{
			Direction:   "Ingress",
			Protocol:    "UDP",
			MinPort:     minPort,
			MaxPort:     maxPort,
			Description: "Linux dynamic/private ports",
			Optional:    true,
		},
	}, nil
}

// parsePortRange parses "MIN MAX" or "MIN-MAX" formatted ranges.
func parsePortRange(s string) (int, int, error) {
	normalized := strings.TrimSpace(strings.ReplaceAll(s, "-", " "))
	fields := strings.Fields(normalized)
	if len(fields) != 2 {
		return 0, 0, fmt.Errorf("unexpected format %q", s)
	}
	minPort, err := strconv.Atoi(fields[0])
	if err != nil {
		return 0, 0, err
	}
	maxPort, err := strconv.Atoi(fields[1])
	if err != nil {
		return 0, 0, err
	}
	return minPort, maxPort, nil
}
