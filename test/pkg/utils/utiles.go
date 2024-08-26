package utils

import (
	"context"

	"github.com/openshift-kni/commatrix/pkg/client"

	configv1 "github.com/openshift/api/config/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func IsSNOCluster(cs *client.ClientSet) (bool, error) {
	infra, err := cs.ConfigV1Interface.Infrastructures().Get(context.Background(), "cluster", metav1.GetOptions{})
	if err != nil {
		return false, err
	}

	return infra.Status.ControlPlaneTopology == configv1.SingleReplicaTopologyMode, nil
}

func IsBMInfra(cs *client.ClientSet) (bool, error) {
	infra, err := cs.ConfigV1Interface.Infrastructures().Get(context.Background(), "cluster", metav1.GetOptions{})
	if err != nil {
		return false, err
	}

	return infra.Status.PlatformStatus.Type == configv1.BareMetalPlatformType, nil
}
