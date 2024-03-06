package endpointslices

import (
	corev1 "k8s.io/api/core/v1"
)

type Filter func(EndpointSlicesInfo) bool

func ApplyFilters(endpointSlicesInfo []EndpointSlicesInfo, filters ...Filter) []EndpointSlicesInfo {
	if len(filters) == 0 {
		return endpointSlicesInfo
	}
	if endpointSlicesInfo == nil {
		return nil
	}

	filteredEndpointsSlices := make([]EndpointSlicesInfo, 0, len(endpointSlicesInfo))

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

func FilterForIngressTraffic(epslicesInfo []EndpointSlicesInfo) []EndpointSlicesInfo {
	return ApplyFilters(epslicesInfo,
		FilterHostNetwork,
		FilterServiceTypes)
}

// FilterHostNetwork checks if the pods behind the endpointSlice are host network.
func FilterHostNetwork(epInfo EndpointSlicesInfo) bool {
	if len(epInfo.pods) == 0 {
		return false
	}
	// Assuming all pods in an EndpointSlice are uniformly on host network or not, we only check the first one.
	return epInfo.pods[0].Spec.HostNetwork
}

// FilterServiceTypes checks if the service behind the endpointSlice is of type LoadBalancer or NodePort.
func FilterServiceTypes(epInfo EndpointSlicesInfo) bool {
	if epInfo.serivce.Spec.Type != corev1.ServiceTypeLoadBalancer &&
		epInfo.serivce.Spec.Type != corev1.ServiceTypeNodePort {
		return false
	}

	return true
}
