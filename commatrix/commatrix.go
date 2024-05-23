package commatrix

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/openshift-kni/commatrix/client"
	"github.com/openshift-kni/commatrix/endpointslices"
	"github.com/openshift-kni/commatrix/types"
)

// TODO: add integration tests.

type Env int

const (
	Baremetal Env = iota
	Cloud
)

type Deployment int

const (
	SNO Deployment = iota
	MNO
)

// New initializes a ComMatrix using Kubernetes cluster data.
// It takes kubeconfigPath for cluster access to  fetch EndpointSlice objects,
// detailing open ports for ingress traffic.
// Custom entries from a JSON file can be added to the matrix by setting `customEntriesPath`.
// Returns a pointer to ComMatrix and error. Entries include traffic direction, protocol,
// port number, namespace, service name, pod, container, node role, and flow optionality for OpenShift.
func New(kubeconfigPath string, customEntriesPath string, e Env, d Deployment) (*types.ComMatrix, error) {
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

	staticEntries, err := getStaticEntries(e, d)
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

	cleanedComDetails := types.CleanComDetails(res)

	return &types.ComMatrix{Matrix: cleanedComDetails}, nil
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

func getStaticEntries(e Env, d Deployment) ([]types.ComDetails, error) {
	comDetails := []types.ComDetails{}

	switch e {
	case Baremetal:
		comDetails = append(comDetails, baremetalStaticEntriesMaster...)
		if d == SNO {
			break
		}
		comDetails = append(comDetails, baremetalStaticEntriesWorker...)
	case Cloud:
		comDetails = append(comDetails, cloudStaticEntriesMaster...)
		if d == SNO {
			break
		}
		comDetails = append(comDetails, cloudStaticEntriesWorker...)
	default:
		return nil, fmt.Errorf("invalid value for cluster environment")
	}

	comDetails = append(comDetails, generalStaticEntriesMaster...)
	if d == SNO {
		return comDetails, nil
	}

	comDetails = append(comDetails, MNOStaticEntries...)
	comDetails = append(comDetails, generalStaticEntriesWorker...)

	return comDetails, nil
}
