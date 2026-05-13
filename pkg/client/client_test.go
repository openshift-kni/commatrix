package client

import (
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("NewScheme", func() {
	It("registers corev1 types", func() {
		Expect(testScheme.IsGroupRegistered(corev1.SchemeGroupVersion.Group)).To(BeTrue())

		for _, kind := range []string{"Pod", "Service", "Node", "Namespace"} {
			Expect(testScheme.Recognizes(schema.GroupVersionKind{
				Group:   corev1.SchemeGroupVersion.Group,
				Version: corev1.SchemeGroupVersion.Version,
				Kind:    kind,
			})).To(BeTrue(), "expected corev1.%s to be registered", kind)
		}
	})

	It("registers discoveryv1 EndpointSlice", func() {
		Expect(testScheme.Recognizes(schema.GroupVersionKind{
			Group:   discoveryv1.SchemeGroupVersion.Group,
			Version: discoveryv1.SchemeGroupVersion.Version,
			Kind:    "EndpointSlice",
		})).To(BeTrue())
	})

	It("registers OpenShift operator API group", func() {
		Expect(testScheme.IsGroupRegistered("operator.openshift.io")).To(BeTrue())
	})

	It("registers machineconfiguration API group", func() {
		Expect(testScheme.IsGroupRegistered("machineconfiguration.openshift.io")).To(BeTrue())
	})

	It("registers OpenShift config API group", func() {
		Expect(testScheme.IsGroupRegistered("config.openshift.io")).To(BeTrue())
	})
})
