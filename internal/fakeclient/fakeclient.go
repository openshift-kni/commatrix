package fakeclient

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type ClusterResources struct {
	Pods     []corev1.Pod
	EpSlices []discoveryv1.EndpointSlice
	Services []corev1.Service
}

func New(initObjects []client.Object) (client.Client, error) {
	scheme := runtime.NewScheme()

	if err := corev1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("corev1: add to scheme failed: %v", err)
	}

	if err := discoveryv1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("discoveryv1: add to scheme failed: %v", err)
	}

	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(initObjects...).
		Build(), nil
}

func ObjectsFromResources(r ClusterResources) []client.Object {
	objects := make([]client.Object, 0)
	for _, pod := range r.Pods {
		objects = append(objects, pod.DeepCopy())
	}

	for _, epSlice := range r.EpSlices {
		objects = append(objects, epSlice.DeepCopy())
	}

	for _, service := range r.Services {
		objects = append(objects, service.DeepCopy())
	}

	return objects
}
