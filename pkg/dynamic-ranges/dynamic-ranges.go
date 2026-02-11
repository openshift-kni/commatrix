package dynamicranges

import (
	"context"
	"fmt"
	"strings"

	log "github.com/sirupsen/logrus"

	"github.com/openshift-kni/commatrix/pkg/client"
	"github.com/openshift-kni/commatrix/pkg/endpointslices"
	"github.com/openshift-kni/commatrix/pkg/types"
	"github.com/openshift-kni/commatrix/pkg/utils"
	configv1 "github.com/openshift/api/config/v1"
	clientOptions "sigs.k8s.io/controller-runtime/pkg/client"
)

func GetDynamicRanges(exporter *endpointslices.EndpointSlicesExporter, utilsHelpers utils.UtilsInterface, cs *client.ClientSet) ([]types.DynamicRange, error) {
	log.Debug("Getting dynamic ranges")

	nodePortDynamicRange, err := getNodePortDynamicRange(exporter)
	if err != nil {
		log.Errorf("Failed to get node port dynamic range: %v", err)
		return nil, fmt.Errorf("failed to get node port dynamic range: %w", err)
	}

	return nodePortDynamicRange, nil
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
