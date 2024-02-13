package endpointslices

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/liornoy/node-comm-lib/internal/client"
	"github.com/liornoy/node-comm-lib/internal/consts"
	"github.com/liornoy/node-comm-lib/internal/nodes"
	"github.com/liornoy/node-comm-lib/internal/types"
)

func toComDetails(cs *client.ClientSet, epSlice discoveryv1.EndpointSlice) ([]types.ComDetails, error) {
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

func ToComDetails(cs *client.ClientSet, slices []discoveryv1.EndpointSlice) ([]types.ComDetails, error) {
	ComDetails := make([]types.ComDetails, 0)
	for _, epSlice := range slices {
		cds, err := toComDetails(cs, epSlice)
		if err != nil {
			return nil, err
		}
		ComDetails = append(ComDetails, cds...)
	}

	cleanedComDetails := removeDups(ComDetails)
	return cleanedComDetails, nil
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
