package customendpointslices

import (
	"context"
	"fmt"
	"log"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/liornoy/node-comm-lib/internal/client"
	"github.com/liornoy/node-comm-lib/internal/consts"
	"github.com/liornoy/node-comm-lib/internal/endpointslices"
	"github.com/liornoy/node-comm-lib/internal/nodes"
	"github.com/liornoy/node-comm-lib/internal/ss"
	"github.com/liornoy/node-comm-lib/internal/types"
)

const defaultNamespace = "default"

// Create creates the custom endpoint slices that don't exist already in the cluster.
func Create(cs *client.ClientSet) error {
	endpointSlices, err := getEndpointSlices(cs)
	if err != nil {
		return err
	}

	existingComDetails, err := endpointslices.ToComDetails(cs, endpointSlices)
	if err != nil {
		return err
	}

	cds, err := getComDetailsFromSS(cs)
	if err != nil {
		return err
	}
	knownComDetails, _ := ss.FilterPorts(cds)

	diffComDetails := getDiffComDetails(knownComDetails, existingComDetails)

	err = createEpSlices(cs, diffComDetails)
	if err != nil {
		return err
	}
	return nil
}

func createEpSlices(cs *client.ClientSet, cds []types.ComDetails) error {
	nodeNameToNodeRoles, err := getNodeNamesToRolesMap(cs)
	if err != nil {
		return err
	}
	nodeRolesToNodeNames := reverseMap(nodeNameToNodeRoles)

	for _, cd := range cds {
		endpointSlice, err := comDetailsToEPSlice(&cd, nodeRolesToNodeNames)
		if err != nil {
			log.Printf("ERROR: failed transating ComDetails to EndpointSlice: %v", err)
			continue
		}

		_, err = cs.EndpointSlices("default").Create(context.TODO(), &endpointSlice, metav1.CreateOptions{})
		if err != nil && !errors.IsAlreadyExists(err) {
			log.Printf("ERROR: failed creating EndpointSlice %s: %v", endpointSlice.Name, err)
			continue
		}

		log.Printf("INFO: successfully created EndpointSlice %s\n", endpointSlice.Name)
	}

	return nil
}

func getNodeNamesToRolesMap(cs *client.ClientSet) (map[string]string, error) {
	res := make(map[string]string)

	nodesList, err := cs.Nodes().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed listing nodes: %v", err)
	}

	for _, n := range nodesList.Items {
		role := nodes.GetRoles(&n)
		res[n.Name] = role
	}

	return res, nil
}

func comDetailsToEPSlice(cd *types.ComDetails, nodeRolesToNodeNames map[string]string) (discoveryv1.EndpointSlice, error) {
	port, err := strconv.ParseInt(cd.Port, 10, 32)
	if err != nil {
		return discoveryv1.EndpointSlice{}, err
	}
	name := fmt.Sprintf("commatrix-test-%s-%s-%s", cd.ServiceName, cd.NodeRole, cd.Port)

	nodeName := nodeRolesToNodeNames[cd.NodeRole]

	labels := map[string]string{
		consts.IngressLabel:          "",
		"kubernetes.io/service-name": cd.ServiceName,
	}
	if !cd.Required {
		labels[consts.OptionalLabel] = consts.OptionalTrue
	}

	endpointSlice := cd.ToEndpointSlice(name, defaultNamespace, nodeName, labels, int(port))

	return endpointSlice, nil
}

func getComDetailsFromSS(cs *client.ClientSet) ([]types.ComDetails, error) {
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

	return cleanedComDetails, nil
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

func removeDups(comDetails []types.ComDetails) []types.ComDetails {
	set := sets.New[types.ComDetails](comDetails...)
	res := set.UnsortedList()

	return res
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

func reverseMap(m map[string]string) map[string]string {
	n := make(map[string]string, len(m))
	for k, v := range m {
		n[v] = k
	}
	return n
}
