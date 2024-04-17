package main

import (
	"log"

	"github.com/liornoy/node-comm-lib/pkg/client"
	"github.com/liornoy/node-comm-lib/pkg/commatrix"
	"github.com/liornoy/node-comm-lib/pkg/consts"
	"github.com/liornoy/node-comm-lib/pkg/endpointslices"
	corev1 "k8s.io/api/core/v1"
)

func main() {
	// IMPORTANT: Set the `KUBECONFIG` enviourment variable.
	cs, err := client.New("")
	if err != nil {
		log.Fatalf("Failed creating client: %v", err)
	}

	epSliceQuery, err := endpointslices.NewQuery(cs)
	if err != nil {
		log.Fatalf("Failed creating EndpointSlices query: %v", err)
	}

	ingressSlice := epSliceQuery.
		WithHostNetwork().
		WithLabels(map[string]string{consts.IngressLabel: ""}).
		WithServiceType(corev1.ServiceTypeNodePort).
		WithServiceType(corev1.ServiceTypeLoadBalancer).
		Query()

	comMatrix, err := commatrix.CreateComMatrix(cs, ingressSlice)
	if err != nil {
		log.Fatalf("Failed creating Communication Matrix: %v", err)
	}

	log.Printf("Created the following Communication Matrix:\n%+v", comMatrix)
}
