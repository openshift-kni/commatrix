package main

import (
	"context"
	"fmt"
	"log"
	"strconv"

	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/liornoy/node-comm-lib/pkg/client"
	"github.com/liornoy/node-comm-lib/pkg/commatrix"
	"github.com/liornoy/node-comm-lib/pkg/consts"
	"github.com/liornoy/node-comm-lib/pkg/ss"
)

const defaultNamespace = "default"

func main() {
	// IMPORTANT: Set the `KUBECONFIG` enviourment variable.
	cs, err := client.New("")
	if err != nil {
		log.Fatalf("Failed creating client: %v", err)
	}

	nodes, err := cs.Nodes().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		log.Fatalf("Failed listing nodes: %v", err)
	}
	nodeNameToNodeRoles := commatrix.GetNodesRoles(nodes)
	nodeRolesToNodeNames := reverseMap(nodeNameToNodeRoles)

	// Create ComDetails from the ss output
	ssComDetails := make([]commatrix.ComDetails, 0)
	for _, n := range nodes.Items {
		nodeRole := nodeNameToNodeRoles[n.Name]
		tcpOutput := "..." // output of `ss -anplt` on the node
		udpOutput := "..." // output of `ss -anplu` on the node

		tcpComDetails := ss.ToComDetails(tcpOutput, nodeRole, "TCP")
		ssComDetails = append(ssComDetails, tcpComDetails...)

		udpComDetails := ss.ToComDetails(udpOutput, nodeRole, "UDP")
		ssComDetails = append(ssComDetails, udpComDetails...)
	}

	// Remove duplications because some services repeat on each worker/master node.
	res := commatrix.RemoveDups(ssComDetails)

	// Create custom EndpointSlices
	for _, cd := range res {
		endpointSlice, err := comDetailsToEPSlice(&cd, nodeRolesToNodeNames)
		if err != nil {
			log.Fatalf("Failed transating ComDetails to EndpointSlice: %v", err)
		}

		_, err = cs.EndpointSlices("default").Create(context.TODO(), &endpointSlice, metav1.CreateOptions{})
		if err != nil && !errors.IsAlreadyExists(err) {
			log.Fatalf("Failed creating EndpointSlice %s: %v", endpointSlice.Name, err)
		}
	}
}

func comDetailsToEPSlice(cd *commatrix.ComDetails, nodeRolesToNodeNames map[string]string) (discoveryv1.EndpointSlice, error) {
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

func reverseMap(m map[string]string) map[string]string {
	n := make(map[string]string, len(m))
	for k, v := range m {
		n[v] = k
	}
	return n
}
