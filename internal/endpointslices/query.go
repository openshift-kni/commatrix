package endpointslices

import (
	"context"
	"fmt"

	clinetutil "github.com/liornoy/node-comm-lib/internal/client"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type QueryBuilder interface {
	Query() []discoveryv1.EndpointSlice
	WithLabels(labels map[string]string) QueryBuilder
	WithHostNetwork() QueryBuilder
	WithServiceType(serviceType corev1.ServiceType) QueryBuilder
}

type QueryParams struct {
	pods     []corev1.Pod
	filter   []bool
	epSlices []discoveryv1.EndpointSlice
	services []corev1.Service
}

type EndpointSliceInfo struct {
	endpointSlice discoveryv1.EndpointSlice
	serivce       corev1.Service
	pods          []corev1.Pod
}

func GetEndpointSliceInfo(cs *clinetutil.ClientSet) ([]EndpointSliceInfo, error) {
	var (
		epSlicesList discoveryv1.EndpointSliceList
		servicesList corev1.ServiceList
		podsList     corev1.PodList
	)

	err := cs.List(context.TODO(), &epSlicesList, &client.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list endpointslices: %w", err)
	}

	err = cs.List(context.TODO(), &servicesList, &client.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list services: %w", err)
	}

	err = cs.List(context.TODO(), &podsList, &client.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}

	res, err := createEndpointSliceInfo(epSlicesList, servicesList, podsList)
	if err != nil {
		return nil, fmt.Errorf("failed to bundle resources: %w", err)
	}

	return res, nil
}

func createEndpointSliceInfo(epSlicesList discoveryv1.EndpointSliceList, servicesList corev1.ServiceList, podsList corev1.PodList) ([]EndpointSliceInfo, error) {
	var service corev1.Service
	var pod corev1.Pod
	var found bool
	res := make([]EndpointSliceInfo, len(epSlicesList.Items))

	for _, epSlice := range epSlicesList.Items {

		pods := make([]corev1.Pod, 1)

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
			name := endpoint.TargetRef.Name
			namespace := endpoint.TargetRef.Namespace

			if pod, found = getPod(name, namespace, podsList); !found {
				return nil, fmt.Errorf("failed to get service for endpoint %s/%s", epSlice.Namespace, epSlice.Name)
			}
			pods = append(pods, pod)
		}

		res = append(res, EndpointSliceInfo{
			endpointSlice: epSlice,
			serivce:       service,
			pods:          pods,
		})
	}

	return res, nil
}

func (q *QueryParams) withLabels(epSlice discoveryv1.EndpointSlice, labels map[string]string) bool {
	for key, value := range labels {
		if mValue, ok := epSlice.Labels[key]; !ok || mValue != value {
			return false
		}
	}

	return true
}

func (q *QueryParams) withServiceType(epSlice discoveryv1.EndpointSlice, serviceType corev1.ServiceType) bool {
	if len(epSlice.OwnerReferences) == 0 {
		return false
	}

	for _, ownerRef := range epSlice.OwnerReferences {
		name := ownerRef.Name
		namespace := epSlice.Namespace
		service := getService(name, namespace, q.services)
		if service == nil {
			continue
		}
		if service.Spec.Type == serviceType {
			return true
		}
	}

	return false
}

func (q *QueryParams) withHostNetwork(epSlice discoveryv1.EndpointSlice) bool {
	if len(epSlice.Endpoints) == 0 {
		return false
	}

	for _, endpoint := range epSlice.Endpoints {
		if endpoint.TargetRef == nil {
			continue
		}
		name := endpoint.TargetRef.Name
		namespace := endpoint.TargetRef.Namespace
		pod := getPod(name, namespace, q.pods)
		if pod == nil {
			continue
		}
		if pod.Spec.HostNetwork {
			return true
		}
	}

	return false
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
