package commatrix

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/liornoy/node-comm-lib/internal/client"
	"github.com/liornoy/node-comm-lib/internal/consts"
	"github.com/liornoy/node-comm-lib/internal/endpointslices"
	"github.com/liornoy/node-comm-lib/internal/nodes"
	"github.com/liornoy/node-comm-lib/internal/ss"
	"github.com/liornoy/node-comm-lib/internal/types"
)

// New gets the kubeconfig path or consumes the KUBECONFIG env var
// and creates Communication Matrix for given cluster.
func New(kubeconfigPath string) (*types.ComMatrix, error) {
	cs, err := client.New(kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed creating the client: %w", err)
	}

	endpointSlices, err := getEndpointSlices(cs)
	if err != nil {
		return nil, fmt.Errorf("failed getting endpointslices: %w", err)
	}

	comDetailsFromEndpointSlices, err := epSlicesToComDetails(cs, endpointSlices)
	if err != nil {
		return nil, err
	}

	ssComDetails, err := getComDetailsFromSS(cs, comDetailsFromEndpointSlices)
	if err != nil {
		return nil, fmt.Errorf("failed getting comDetails from `ss` command: %w", err)
	}

	res := &types.ComMatrix{Matrix: comDetailsFromEndpointSlices}
	res.Matrix = append(res.Matrix, ssComDetails...)

	return res, nil
}

func getComDetailsFromSS(cs *client.ClientSet, existingComDetails []types.ComDetails) ([]types.ComDetails, error) {
	nodes, err := cs.Nodes().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	allComDetails := make([]types.ComDetails, 0)
	for _, n := range nodes.Items {
		cds, err := ss.CreateComDetailsFromNode(cs, &n)
		if err != nil {
			return nil, err
		}
		allComDetails = append(allComDetails, cds...)
	}
	cleanedComDetails := removeDups(allComDetails)
	knownComDetails, _ := ss.FilterPorts(cleanedComDetails)

	diffComDetails := getDiffComDetails(knownComDetails, existingComDetails)

	return diffComDetails, nil
}

// getDiffComDetails returns all the ComDetails that present in the slice cd1
// and not in cd2.
func getDiffComDetails(comDetails1 []types.ComDetails, comDetails2 []types.ComDetails) []types.ComDetails {
	res := make([]types.ComDetails, 0)
	for _, cd1 := range comDetails1 {
		found := false
		for _, cd2 := range comDetails2 {
			if isComDetailsEqual(cd1, cd2) {
				found = true
				break
			}
		}
		if !found {
			res = append(res, cd1)
		}
	}

	return res
}

func isComDetailsEqual(a types.ComDetails, b types.ComDetails) bool {
	return a.NodeRole == b.NodeRole && a.Port == b.Port && a.Protocol == b.Protocol
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

func epSlicesToComDetails(cs *client.ClientSet, slices []discoveryv1.EndpointSlice) ([]types.ComDetails, error) {
	ComDetails := make([]types.ComDetails, 0)
	for _, epSlice := range slices {
		cds, err := epSliceToComDetails(cs, epSlice)
		if err != nil {
			return nil, err
		}
		ComDetails = append(ComDetails, cds...)
	}

	cleanedComDetails := removeDups(ComDetails)
	return cleanedComDetails, nil
}

func epSliceToComDetails(cs *client.ClientSet, epSlice discoveryv1.EndpointSlice) ([]types.ComDetails, error) {
	res := make([]types.ComDetails, 0)
	roles := make(map[string]bool)

	nodeList, err := cs.Nodes().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	for _, endpoint := range epSlice.Endpoints {
		node := corev1.Node{}
		found := false
		for _, n := range nodeList.Items {
			if n.Name == *endpoint.NodeName {
				node = n
				found = true
			}
		}
		if !found {
			return nil, fmt.Errorf("failed creating ComDetails: node %s not found", *endpoint.NodeName)
		}
		nodeRole := nodes.GetRoles(&node)
		roles[nodeRole] = true
	}

	required := isRequired(epSlice)
	service := epSlice.Labels["kubernetes.io/service-name"]

	for role := range roles {
		for _, p := range epSlice.Ports {
			res = append(res, types.ComDetails{
				Direction:   consts.IngressLabel,
				Protocol:    fmt.Sprint(*p.Protocol),
				Port:        fmt.Sprint(*p.Port),
				NodeRole:    role,
				ServiceName: service,
				Required:    required,
			})
		}
	}

	return res, nil
}

func isRequired(epSlice discoveryv1.EndpointSlice) bool {
	required := true
	if _, ok := epSlice.Labels[consts.OptionalLabel]; ok {
		required = false
	}

	return required
}

func removeDups(comDetails []types.ComDetails) []types.ComDetails {
	set := sets.New[types.ComDetails](comDetails...)
	res := set.UnsortedList()

	return res
}
