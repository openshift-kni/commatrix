package endpointslices

import (
	"fmt"
)

type Filter func(EndpointSliceInfo) (bool, error)

func ApplyFilters(endpointSlicesInfo []EndpointSliceInfo, filters ...Filter) ([]EndpointSliceInfo, error) {
	if len(filters) == 0 {
		return endpointSlicesInfo, nil
	}
	if endpointSlicesInfo == nil {
		return nil, nil
	}

	filteredEndpointsSlices := make([]EndpointSliceInfo, 0, len(endpointSlicesInfo))

	for _, epInfo := range endpointSlicesInfo {
		keep := true

		for _, f := range filters {
			ret, err := f(epInfo)
			if err != nil {
				return nil, fmt.Errorf("failed to filter endpointslice %s/%s, err: %w", epInfo.endpointSlice.Namespace, epInfo.endpointSlice.Name, err)
			}
			if !ret {
				keep = false
				break
			}
		}

		if keep {
			filteredEndpointsSlices = append(filteredEndpointsSlices, epInfo)
		}
	}

	return filteredEndpointsSlices, nil
}

func FilterForIngressTrafic(endpointslices []EndpointSliceInfo) ([]EndpointSliceInfo, error) {
	filteredEndpointsSlices, err := ApplyFilters(endpointslices,
		FilterHostNetwork,
		FilterLabels)
	if err != nil {
		return nil, err
	}

	return filteredEndpointsSlices, nil
}

// FilterHostNetwork checks if the pods behind the endpointSlice are host network.
func FilterHostNetwork(epInfo EndpointSliceInfo) (bool, error) {
	// We assume all pods for given endpointSlice are either host network or not, so check only the first.
	if !epInfo.pods[0].Spec.HostNetwork {
		return false, nil
	}

	return true, nil
}
