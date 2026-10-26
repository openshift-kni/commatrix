package e2e

import (
	"testing"

	matrixdiff "github.com/openshift-kni/commatrix/pkg/matrix-diff"
	"github.com/openshift-kni/commatrix/pkg/types"
)

func TestFilterOutPortsOfKnownServices(t *testing.T) {
	tests := []struct {
		name     string
		listener types.ComDetails
		filtered bool
	}{
		{
			name: "unverified CRI-O listener remains reportable",
			listener: types.ComDetails{
				Protocol: "TCP",
				Port:     40000,
				Service:  "crio",
			},
		},
		{
			name: "rpc.statd listener is ignored",
			listener: types.ComDetails{
				Protocol: "TCP",
				Port:     40000,
				Service:  "rpc.statd",
			},
			filtered: true,
		},
		{
			name: "dnsmasq listener is ignored",
			listener: types.ComDetails{
				Protocol: "UDP",
				Port:     53,
				Service:  "dnsmasq",
			},
			filtered: true,
		},
		{
			name: "dig listener is ignored",
			listener: types.ComDetails{
				Protocol: "UDP",
				Port:     53,
				Service:  "dig",
			},
			filtered: true,
		},
		{
			name: "custom listener remains reportable",
			listener: types.ComDetails{
				Protocol: "TCP",
				Port:     8080,
				Service:  "custom-daemon",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matrix := &types.ComMatrix{Ports: []types.ComDetails{tt.listener}}
			got := filterOutPortsOfKnownServices(matrix)
			if tt.filtered && len(got.Ports) != 0 {
				t.Fatalf("expected listener to be filtered, got %#v", got.Ports)
			}
			if !tt.filtered && len(got.Ports) != 1 {
				t.Fatalf("expected listener to remain a mismatch, got %#v", got.Ports)
			}
		})
	}
}

func TestEndpointSliceBackedListenerMatchesByTuple(t *testing.T) {
	endpointSlicePort := types.ComDetails{
		Direction: "Ingress",
		Protocol:  "TCP",
		Port:      40000,
		Namespace: "test-namespace",
		Service:   "test-service",
		Pod:       "test-pod",
		Container: "test-container",
		NodeGroup: "worker",
	}
	ssListener := types.ComDetails{
		Direction: "Ingress",
		Protocol:  "TCP",
		Port:      40000,
		Service:   "crio",
		NodeGroup: "worker",
	}

	diff := matrixdiff.Generate(
		&types.ComMatrix{Ports: []types.ComDetails{endpointSlicePort}},
		&types.ComMatrix{Ports: []types.ComDetails{ssListener}},
	)
	missing := diff.GetUniqueSecondary()
	if len(missing.Ports) != 0 {
		t.Fatalf("expected EndpointSlice-backed listener not to be missing, got %#v", missing.Ports)
	}
}
