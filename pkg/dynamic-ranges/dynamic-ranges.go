package dynamicranges

import (
	"context"
	"fmt"
	"strings"

	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift-kni/commatrix/pkg/client"
	"github.com/openshift-kni/commatrix/pkg/consts"
	"github.com/openshift-kni/commatrix/pkg/endpointslices"
	"github.com/openshift-kni/commatrix/pkg/types"
	"github.com/openshift-kni/commatrix/pkg/utils"
	configv1 "github.com/openshift/api/config/v1"
	clientOptions "sigs.k8s.io/controller-runtime/pkg/client"
)

func GetDynamicRanges(exporter *endpointslices.EndpointSlicesExporter, utilsHelpers utils.UtilsInterface, cs *client.ClientSet) ([]types.DynamicRange, error) {
	log.Debug("Getting dynamic ranges")

	dynamicRanges := []types.DynamicRange{}

	nodePortDynamicRange, err := getNodePortDynamicRange(exporter)
	if err != nil {
		log.Errorf("Failed to get node port dynamic range: %v", err)
		return nil, fmt.Errorf("failed to get node port dynamic range: %w", err)
	}
	dynamicRanges = append(dynamicRanges, nodePortDynamicRange...)

	linuxDynamicPrivateRange, err := getLinuxDynamicPrivateRange(exporter, utilsHelpers)
	if err != nil {
		log.Errorf("Failed to get Linux dynamic private range: %v", err)
		return nil, fmt.Errorf("failed to get Linux dynamic private range: %w", err)
	}
	dynamicRanges = append(dynamicRanges, linuxDynamicPrivateRange...)

	return dynamicRanges, nil
}

// GetNodePortDynamicRange returns the cluster's Service NodePort range as dynamic ranges.
// If the cluster does not define a custom range, it falls back to the Kubernetes default (30000-32767).
func getNodePortDynamicRange(exporter *endpointslices.EndpointSlicesExporter) ([]types.DynamicRange, error) {
	log.Debug("Getting node port dynamic range")
	network := &configv1.Network{}
	if err := exporter.Get(context.TODO(), clientOptions.ObjectKey{Name: "cluster"}, network); err != nil {
		log.Errorf("Failed to get Network config: %v", err)
		return nil, fmt.Errorf("failed to get Network config: %w", err)
	}

	dr := types.KubeletNodePortDefaultDynamicRange
	rangeStr := strings.TrimSpace(network.Spec.ServiceNodePortRange)
	if rangeStr == "" {
		log.Debug("ServiceNodePortRange not set; using default")
		return dr, nil
	}

	minPort, maxPort, err := types.ParsePortRangeHyphen(rangeStr)
	if err != nil {
		log.Errorf("Invalid ServiceNodePortRange format %q: %v", rangeStr, err)
		return nil, fmt.Errorf("invalid ServiceNodePortRange format %q: %w", rangeStr, err)
	}

	return []types.DynamicRange{
		{
			Direction:   "Ingress",
			Protocol:    "TCP",
			MinPort:     minPort,
			MaxPort:     maxPort,
			Description: "Kubelet node ports",
			Optional:    false,
		},
		{
			Direction:   "Ingress",
			Protocol:    "UDP",
			MinPort:     minPort,
			MaxPort:     maxPort,
			Description: "Kubelet node ports",
			Optional:    false,
		},
	}, nil
}

// getLinuxDynamicPrivateRange retrieves the Linux dynamic/private port range from a cluster node
// by reading the host sysctl:
//   - /proc/sys/net/ipv4/ip_local_port_range
func getLinuxDynamicPrivateRange(exporter *endpointslices.EndpointSlicesExporter, utilsHelpers utils.UtilsInterface) ([]types.DynamicRange, error) {
	log.Debug("Getting Linux dynamic/private port range from cluster")

	// Pick an arbitrary node to query (ranges are expected to be consistent across nodes).
	nodes, err := exporter.CoreV1Interface.Nodes().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		log.Errorf("Failed to list nodes: %v", err)
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}
	if len(nodes.Items) == 0 {
		return nil, fmt.Errorf("no nodes found in the cluster")
	}
	nodeName := nodes.Items[0].Name

	// Ensure namespace exists.
	if err := utilsHelpers.CreateNamespace(consts.DefaultDebugNamespace); err != nil {
		log.Errorf("Failed to create debug namespace: %v", err)
		return nil, fmt.Errorf("failed to create debug namespace: %w", err)
	}
	defer func() {
		if delErr := utilsHelpers.DeleteNamespace(consts.DefaultDebugNamespace); delErr != nil {
			log.Warnf("failed to delete namespace %s: %v", consts.DefaultDebugNamespace, delErr)
		}
	}()

	// Create a privileged pod on the selected node.
	pod, err := utilsHelpers.CreatePodOnNode(nodeName, consts.DefaultDebugNamespace, consts.DefaultDebugPodImage, []string{})
	if err != nil {
		log.Errorf("Failed to create debug pod: %v", err)
		return nil, fmt.Errorf("failed to create debug pod: %w", err)
	}
	defer func() {
		if delErr := utilsHelpers.DeletePod(pod); delErr != nil {
			log.Warnf("failed to delete debug pod %s: %v", pod.Name, delErr)
		}
	}()

	// Wait for the pod to be running.
	if err := utilsHelpers.WaitForPodStatus(consts.DefaultDebugNamespace, pod, corev1.PodRunning); err != nil {
		log.Errorf("Debug pod did not reach Running state: %v", err)
		return nil, fmt.Errorf("debug pod did not reach Running state: %w", err)
	}

	// Read IPv4 range (applies to both IPv4 and IPv6 ephemeral ports).
	out, err := utilsHelpers.RunCommandOnPod(pod, []string{"/bin/sh", "-c", "cat /host/proc/sys/net/ipv4/ip_local_port_range"})
	if err != nil {
		log.Errorf("Failed to read IPv4 ip_local_port_range: %v", err)
		return nil, fmt.Errorf("failed to read IPv4 ip_local_port_range: %w", err)
	}
	// If not set or empty, fall back to the Linux default dynamic/private range.
	if strings.TrimSpace(string(out)) == "" {
		log.Debug("ip_local_port_range not set; using Linux default dynamic/private range")
		return types.LinuxDynamicPrivateDefaultDynamicRange, nil
	}
	minPort, maxPort, err := types.ParsePortRangeSpace(string(out))
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
