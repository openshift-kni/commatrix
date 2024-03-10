package commatrix

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/liornoy/node-comm-lib/pkg/client"
	"github.com/liornoy/node-comm-lib/pkg/endpointslices"
	"github.com/liornoy/node-comm-lib/pkg/types"
)

// New initializes a ComMatrix using Kubernetes cluster data.
// It takes kubeconfigPath for cluster access to  fetch EndpointSlice objects,
// detailing open ports for ingress traffic.
// customEntriesPath allows adding custom entries from a JSON file to the matrix.
// Returns a pointer to ComMatrix and error. Entries include traffic direction, protocol,
// port number, namespace, service name, pod, container, node role, and flow optionality for OpenShift.
func New(kubeconfigPath string, customEntriesPath string) (*types.ComMatrix, error) {
	res := make([]types.ComDetails, 0)

	cs, err := client.New(kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed creating the k8s client: %w", err)
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

	staticEntries, err := getStaticEntries()
	if err != nil {
		return nil, err
	}

	res = append(res, staticEntries...)

	if customEntriesPath != "" {
		customComDetails, err := addFromFile(customEntriesPath)
		if err != nil {
			return nil, fmt.Errorf("failed fetching custom entries from file %s err: %w", customEntriesPath, err)
		}

		res = append(res, customComDetails...)
	}

	return &types.ComMatrix{Matrix: res}, nil
}

func addFromFile(fp string) ([]types.ComDetails, error) {
	var res []types.ComDetails
	f, err := os.Open(filepath.Clean(fp))
	if err != nil {
		return nil, fmt.Errorf("failed to open file %s: %v", fp, err)
	}
	defer f.Close()
	raw, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %v", fp, err)
	}

	err = json.Unmarshal(raw, &res)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal custom entries file: %v", err)
	}

	return res, nil
}

func getStaticEntries() ([]types.ComDetails, error) {
	var res []types.ComDetails

	err := json.Unmarshal([]byte(staticEntries), &res)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal static entries: %v", err)
	}

	return res, nil
}
