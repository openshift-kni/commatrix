package commatrix

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"

	"github.com/liornoy/node-comm-lib/internal/client"
	"github.com/liornoy/node-comm-lib/internal/consts"
	"github.com/liornoy/node-comm-lib/internal/customendpointslices"
	"github.com/liornoy/node-comm-lib/internal/endpointslices"
	"github.com/liornoy/node-comm-lib/internal/types"
)

// New gets the kubeconfig path or consumes the KUBECONFIG env var
// and creates Communication Matrix for given cluster.
func New(kubeconfigPath string) (*types.ComMatrix, error) {
	cs, err := client.New(kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed creating the client: %w", err)
	}

	// Temporary step: Manually creating missing endpointslices.
	err = customendpointslices.Create(cs)
	if err != nil {
		return nil, fmt.Errorf("failed creating custom services: %w", err)
	}

	endpointSlices, err := getEndpointSlices(cs)
	if err != nil {
		return nil, fmt.Errorf("failed getting endpointslices: %w", err)
	}

	comDetailsFromEndpointSlices, err := endpointslices.ToComDetails(cs, endpointSlices)
	if err != nil {
		return nil, err
	}

	res := &types.ComMatrix{Matrix: comDetailsFromEndpointSlices}

	return res, nil
}

func getEndpointSlices(cs *client.ClientSet) ([]discoveryv1.EndpointSlice, error) {
	query, err := endpointslices.NewQuery(cs)
	if err != nil {
		return nil, fmt.Errorf("failed creating new query: %v", err)
	}

	res := query.
		WithHostNetwork().
		WithLabels(map[string]string{consts.IngressLabel: ""}).
		WithServiceType(corev1.ServiceTypeNodePort).
		WithServiceType(corev1.ServiceTypeLoadBalancer).
		Query()

	return res, nil
}
