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
			name: "CRI-O host-local TCP listener in the ephemeral range",
			listener: types.ComDetails{
				Protocol: "TCP",
				Port:     40000,
				Service:  "crio",
			},
			filtered: true,
		},
		{
			name: "non-CRI-O fixed listener",
			listener: types.ComDetails{
				Protocol: "TCP",
				Port:     8080,
				Service:  "custom-daemon",
			},
		},
		{
			name: "fixed CRI-O listener",
			listener: types.ComDetails{
				Protocol: "TCP",
				Port:     10000,
				Service:  "crio",
			},
		},
		{
			name: "CRI-O listener on an arbitrary high port",
			listener: types.ComDetails{
				Protocol: "TCP",
				Port:     61000,
				Service:  "crio",
			},
		},
		{
			name: "non-CRI-O listener in the ephemeral range",
			listener: types.ComDetails{
				Protocol: "TCP",
				Port:     40000,
				Service:  "custom-daemon",
			},
		},
		{
			name: "CRI-O UDP listener in the ephemeral range",
			listener: types.ComDetails{
				Protocol: "UDP",
				Port:     40000,
				Service:  "crio",
			},
		},
		{
			name: "CRI-O ephemeral listener with Kubernetes identity",
			listener: types.ComDetails{
				Protocol:  "TCP",
				Port:      40000,
				Service:   "crio",
				Namespace: "test-namespace",
				Pod:       "test-pod",
				Container: "test-container",
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

func TestEndpointSliceBackedListenerIsNotMissing(t *testing.T) {
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
	missing := filterOutPortsOfKnownServices(diff.GetUniqueSecondary())
	if len(missing.Ports) != 0 {
		t.Fatalf("expected EndpointSlice-backed listener not to be missing, got %#v", missing.Ports)
	}
}
