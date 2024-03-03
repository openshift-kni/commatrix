package endpointslices

import (
	"context"
	"fmt"
	"log"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	rtclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/liornoy/node-comm-lib/pkg/client"
	"github.com/liornoy/node-comm-lib/pkg/consts"
	"github.com/liornoy/node-comm-lib/pkg/nodes"
	"github.com/liornoy/node-comm-lib/pkg/types"
)

type SvcInfo struct {
	endpointSlice discoveryv1.EndpointSlice
	serivce       corev1.Service
	pods          []corev1.Pod
}

func GetIngressEndpointSlices(cs *client.ClientSet) ([]SvcInfo, error) {
	var (
		epSlicesList discoveryv1.EndpointSliceList
		servicesList corev1.ServiceList
		podsList     corev1.PodList
	)

	err := cs.List(context.TODO(), &epSlicesList, &rtclient.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list endpointslices: %w", err)
	}

	err = cs.List(context.TODO(), &servicesList, &rtclient.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list services: %w", err)
	}

	err = cs.List(context.TODO(), &podsList, &rtclient.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}

	epsliceInfo, err := createEndpointSliceInfo(epSlicesList, servicesList, podsList)
	if err != nil {
		return nil, fmt.Errorf("failed to bundle resources: %w", err)
	}

	res := FilterForIngressTraffic(epsliceInfo)

	return res, nil
}
func createEndpointSliceInfo(epSlicesList discoveryv1.EndpointSliceList, servicesList corev1.ServiceList, podsList corev1.PodList) ([]SvcInfo, error) {
	var service corev1.Service
	var pod corev1.Pod
	var found bool
	res := make([]SvcInfo, len(epSlicesList.Items))

	for _, epSlice := range epSlicesList.Items {

		pods := make([]corev1.Pod, 0)

		// Fetch info about the service behind the endpointslice.
		for _, ownerRef := range epSlice.OwnerReferences {
			name := ownerRef.Name
			namespace := epSlice.Namespace
			if service, found = getService(name, namespace, servicesList); !found {
				return nil, fmt.Errorf("failed to get service for endpoint %s/%s", epSlice.Namespace, epSlice.Name)
			}
		}

		// Fetch info about the pods behind the endpointslice.
		for _, endpoint := range epSlice.Endpoints {
			if endpoint.TargetRef == nil {
				continue
			}
			name := endpoint.TargetRef.Name
			namespace := endpoint.TargetRef.Namespace

			if pod, found = getPod(name, namespace, podsList); !found {
				log.Printf("warning: failed to get service for endpoint %s/%s", epSlice.Namespace, epSlice.Name)
				continue
				//return nil, fmt.Errorf("failed to get service for endpoint %s/%s", epSlice.Namespace, epSlice.Name)
			}
			pods = append(pods, pod)
		}

		res = append(res, SvcInfo{
			endpointSlice: epSlice,
			serivce:       service,
			pods:          pods,
		})
	}

	return res, nil
}
func getPod(name, namespace string, podsList corev1.PodList) (corev1.Pod, bool) {
	for _, pod := range podsList.Items {
		if pod.Name == name && pod.Namespace == namespace {
			return pod, true
		}
	}
	return corev1.Pod{}, false
}

func getService(name, namespace string, serviceList corev1.ServiceList) (corev1.Service, bool) {
	for _, service := range serviceList.Items {
		if service.Name == name && service.Namespace == namespace {
			return service, true
		}
	}

	return corev1.Service{}, false
}

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

func ToComDetails(cs *client.ClientSet, epSlicesInfo []SvcInfo) ([]types.ComDetails, error) {
	ComDetails := make([]types.ComDetails, 0)
	for _, epSliceInfo := range epSlicesInfo {
		cds, err := toComDetails(cs, epSliceInfo.endpointSlice)
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
