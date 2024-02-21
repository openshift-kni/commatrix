package endpointslices

import (
	corev1 "k8s.io/api/core/v1"
)

type Filter func(SvcInfo) bool

func ApplyFilters(endpointSlicesInfo []SvcInfo, filters ...Filter) []SvcInfo {
	if len(filters) == 0 {
		return endpointSlicesInfo
	}
	if endpointSlicesInfo == nil {
		return nil
	}

	filteredEndpointsSlices := make([]SvcInfo, 0, len(endpointSlicesInfo))

	for _, epInfo := range endpointSlicesInfo {
		keep := true

		for _, f := range filters {
			ret := f(epInfo)
			if !ret {
				keep = false
				break
			}
		}

		if keep {
			filteredEndpointsSlices = append(filteredEndpointsSlices, epInfo)
		}
	}

	return filteredEndpointsSlices
}

func FilterForIngressTraffic(endpointslices []SvcInfo) []SvcInfo {
	return ApplyFilters(endpointslices,
		FilterHostNetwork,
		FilterServiceTypes)
}

// FilterHostNetwork checks if the pods behind the endpointSlice are host network.
func FilterHostNetwork(epInfo SvcInfo) bool {
	if len(epInfo.pods) == 0 {
		return false
	}
	// Assuming all pods in an EndpointSlice are uniformly on host network or not, we only check the first one.
	return epInfo.pods[0].Spec.HostNetwork
}

// FilterServiceTypes checks if the service behind the endpointSlice is of type LoadBalancer or NodePort.
func FilterServiceTypes(epInfo SvcInfo) bool {
	if epInfo.serivce.Spec.Type != corev1.ServiceTypeLoadBalancer &&
		epInfo.serivce.Spec.Type != corev1.ServiceTypeNodePort {
		return false
	}

	return true
}
