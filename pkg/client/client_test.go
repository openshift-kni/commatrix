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

	DescribeTable("registers OpenShift API types",
		func(group string, kinds []string) {
			for _, kind := range kinds {
				gvk := schema.GroupVersionKind{Group: group, Version: "v1", Kind: kind}
				Expect(testScheme.Recognizes(gvk)).To(BeTrue(), "expected %s to be registered", gvk)
			}
		},
		Entry("operator.openshift.io", "operator.openshift.io", []string{"MachineConfiguration"}),
		Entry("machineconfiguration.openshift.io", "machineconfiguration.openshift.io", []string{"MachineConfigPool", "MachineConfig"}),
		Entry("config.openshift.io", "config.openshift.io", []string{"Infrastructure", "ClusterVersion", "Network"}),
	)
})
