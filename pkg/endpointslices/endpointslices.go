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
	nodesutil "github.com/liornoy/node-comm-lib/pkg/nodes"
	"github.com/liornoy/node-comm-lib/pkg/types"
)

type EndpointSlicesInfo struct {
	endpointSlice discoveryv1.EndpointSlice
	serivce       corev1.Service
	pods          []corev1.Pod
}

func GetIngressEndpointSlicesInfo(cs *client.ClientSet) ([]EndpointSlicesInfo, error) {
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

	epsliceInfos, err := createServicecInfos(&epSlicesList, &servicesList, &podsList)
	if err != nil {
		return nil, fmt.Errorf("failed to bundle resources: %w", err)
	}

	res := FilterForIngressTraffic(epsliceInfos)

	return res, nil
}

func ToComDetails(cs *client.ClientSet, epSlicesInfo []EndpointSlicesInfo) ([]types.ComDetails, error) {
	comDetails := make([]types.ComDetails, 0)
	nodeList, err := cs.Nodes().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	for _, epSliceInfo := range epSlicesInfo {
		cds, err := epSliceInfo.toComDetails(nodeList.Items)
		if err != nil {
			return nil, err
		}

		comDetails = append(comDetails, cds...)
	}

	cleanedComDetails := removeDups(comDetails)
	return cleanedComDetails, nil
}

// createServiceInfos retrieves lists of EndpointSlices, Services, and Pods from the cluster and generates a slice of EndpointSlicesInfo, each representing a distinct service.
func createServicecInfos(epSlicesList *discoveryv1.EndpointSliceList, servicesList *corev1.ServiceList, podsList *corev1.PodList) ([]EndpointSlicesInfo, error) {
	var (
		service corev1.Service
		pod     corev1.Pod
		found   bool
		res     []EndpointSlicesInfo
	)
	res = make([]EndpointSlicesInfo, len(epSlicesList.Items))

	for _, epSlice := range epSlicesList.Items {
		// Fetch info about the service behind the endpointslice.
		for _, ownerRef := range epSlice.OwnerReferences {
			name := ownerRef.Name
			namespace := epSlice.Namespace
			if service, found = getService(name, namespace, servicesList); !found {
				return nil, fmt.Errorf("failed to get service for endpoint %s/%s", epSlice.Namespace, epSlice.Name)
			}
		}

		// Fetch info about the pods behind the endpointslice.
		pods := make([]corev1.Pod, 0)
		for _, endpoint := range epSlice.Endpoints {
			if endpoint.TargetRef == nil {
				continue
			}
			name := endpoint.TargetRef.Name
			namespace := endpoint.TargetRef.Namespace

			if pod, found = getPod(name, namespace, podsList); !found {
				log.Printf("warning: failed to get service for endpoint %s/%s", epSlice.Namespace, epSlice.Name)
				continue
			}
			pods = append(pods, pod)
		}

		res = append(res, EndpointSlicesInfo{
			endpointSlice: epSlice,
			serivce:       service,
			pods:          pods,
		})
	}

	return res, nil
}

func getPod(name, namespace string, podsList *corev1.PodList) (corev1.Pod, bool) {
	for _, pod := range podsList.Items {
		if pod.Name == name && pod.Namespace == namespace {
			return pod, true
		}
	}
	return corev1.Pod{}, false
}

func getService(name, namespace string, serviceList *corev1.ServiceList) (corev1.Service, bool) {
	for _, service := range serviceList.Items {
		if service.Name == name && service.Namespace == namespace {
			return service, true
		}
	}

	return corev1.Service{}, false
}

// getEndpointSliceNodeRoles gets endpointslice Info struct and returns which node roles the services are on.
func getEndpointSliceNodeRoles(epSliceInfo *EndpointSlicesInfo, nodes []corev1.Node) []string {
	// map to prevent duplications
	rolesMap := make(map[string]bool)
	for _, endpoint := range epSliceInfo.endpointSlice.Endpoints {
		nodeName := endpoint.NodeName
		for _, node := range nodes {
			if node.Name == *nodeName {
				role := nodesutil.GetRoles(&node)
				rolesMap[role] = true
			}
		}
	}

	roles := []string{}
	for k, _ := range rolesMap {
		roles = append(roles, k)
	}

	return roles
}

func getContainerName(portNum int, pods []corev1.Pod) (string, error) {
	res := ""
	pod := pods[0]
	found := false

	if len(pods) == 0 {
		return "", fmt.Errorf("got empty pods slice")
	}

	for i := 0; i <= len(pod.Spec.Containers); i++ {
		container := pod.Spec.Containers[i]

		if found {
			break
		}

		for _, port := range container.Ports {
			if port.ContainerPort == int32(portNum) {
				res = container.Name
				found = true
				break
			}
		}
	}

	if !found {
		return "", fmt.Errorf("couldn't find port %d in pods: %+v", portNum, pods)
	}

	return res, nil
}

func (epSliceinfo *EndpointSlicesInfo) toComDetails(nodes []corev1.Node) ([]types.ComDetails, error) {
	// Each EndpointSlice:
	// endpoints: get the roles (master / worker or both.)
	// ownerReferences = services. (could be >1)
	// Each service: - under the service metadata - pod's namespace (same as pods)
	//                                              pod's name: labels: app: <name>
	// Ports - (could be >1)
	// Need to fetch container name from pods.

	// Get roles (from endpointslice.Endpoints.addresses.nodeName)
	// Get pod name (service.metadata.labels: app)
	// Get pod namespace (service.metadata.namespace)

	// Role(s)
	// (service) Namespace
	// (pod) Name
	// (pod) Container(s)
	// (pod) port(s)

	// 1 endpointslice can have 4 ComDetails:
	// 2 roles. 2 ports. for example:

	// port num ||role || serviceName || namespace || podname	||	 containername	||
	// ==============================================================================
	//  9103, 	master, ovn-kubernetes-node, openshift-ovn-kubernetes,ovnkube-node, kube-rbac-proxy-node
	//  9105, 	worker, ovn-kubernetes-node, openshift-ovn-kubernetes,ovnkube-node, kube-rbac-proxy-ovn-metrics
	//  9103, 	master, ovn-kubernetes-node, openshift-ovn-kubernetes,ovnkube-node, kube-rbac-proxy-node
	//  9105, 	worker, ovn-kubernetes-node, openshift-ovn-kubernetes,ovnkube-node, kube-rbac-proxy-ovn-metrics

	res := make([]types.ComDetails, 0)

	// Get the Namespace and Pod's name from the service.
	namespace := epSliceinfo.serivce.Namespace
	name := epSliceinfo.serivce.Labels["app"]

	// Get the node roles of this endpointslice. (master or worker or both).
	roles := getEndpointSliceNodeRoles(epSliceinfo, nodes)

	epSlice := epSliceinfo.endpointSlice

	optional := isOptional(epSlice)
	service := epSlice.Labels["kubernetes.io/service-name"]

	for _, role := range roles {
		for _, port := range epSlice.Ports {
			containerName, err := getContainerName(int(*port.Port), epSliceinfo.pods)
			if err != nil {
				return nil, fmt.Errorf("failed to get container name: %w", err)
			}

			res = append(res, types.ComDetails{
				Direction: consts.IngressLabel,
				Protocol:  fmt.Sprint(port.Protocol),
				Port:      fmt.Sprint(port.Port),
				Namespace: namespace,
				Pod:       name,
				Container: containerName,
				NodeRole:  role,
				Service:   service,
				Optional:  optional,
			})
		}
	}

	return res, nil
}

func isOptional(epSlice discoveryv1.EndpointSlice) bool {
	optional := false
	if _, ok := epSlice.Labels[consts.OptionalLabel]; ok {
		optional = true
	}

	return optional
}

func removeDups(comDetails []types.ComDetails) []types.ComDetails {
	set := sets.New[types.ComDetails](comDetails...)
	res := set.UnsortedList()

	return res
}
