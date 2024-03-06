package commatrix

import (
	"fmt"

	"github.com/liornoy/node-comm-lib/pkg/client"
	"github.com/liornoy/node-comm-lib/pkg/endpointslices"
	"github.com/liornoy/node-comm-lib/pkg/matrixcustomizer"
	"github.com/liornoy/node-comm-lib/pkg/types"
)

// New gets the kubeconfig path or consumes the KUBECONFIG env var
// and creates Communication Matrix for given cluster.
func New(kubeconfigPath string, customEntriesPath string) (*types.ComMatrix, error) {
	res := make([]types.ComDetails, 0)

	cs, err := client.New(kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed creating the client: %w", err)
	}

	epSlicesInfo, err := endpointslices.GetIngressEndpointSlicesInfo(cs)
	if err != nil {
		return nil, fmt.Errorf("failed getting endpointslices: %w", err)
	}

	epSliceComDetails, err := endpointslices.ToComDetails(cs, epSlicesInfo)
	if err != nil {
		return nil, err
	}
	res = append(res, epSliceComDetails...)

	staticCustomComDetails, err := matrixcustomizer.GetStaticCustomEntries()
	if err != nil {
		return nil, err
	}
	res = append(res, staticCustomComDetails...)

	if customEntriesPath != "" {
		customComDetails, err := matrixcustomizer.AddFromFile(customEntriesPath)
		if err != nil {
			return nil, fmt.Errorf("failed fetching custom entries from file %s err: %w", customEntriesPath, err)
		}

		res = append(res, customComDetails...)
	}

	return &types.ComMatrix{Matrix: res}, nil
}
