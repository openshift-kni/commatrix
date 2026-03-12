package dynamicranges

import (
	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	configv1 "github.com/openshift/api/config/v1"
	machineconfigurationv1 "github.com/openshift/api/machineconfiguration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	fakek "k8s.io/client-go/kubernetes/fake"
	rtclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/openshift-kni/commatrix/pkg/client"
	"github.com/openshift-kni/commatrix/pkg/endpointslices"
	"github.com/openshift-kni/commatrix/pkg/types"
)

func newExporterWithObjects(objs ...rtclient.Object) *endpointslices.EndpointSlicesExporter {
	sch := runtime.NewScheme()
	o.Expect(corev1.AddToScheme(sch)).ToNot(o.HaveOccurred())
	o.Expect(machineconfigurationv1.AddToScheme(sch)).ToNot(o.HaveOccurred())
	o.Expect(configv1.AddToScheme(sch)).ToNot(o.HaveOccurred())

	fakeClient := fake.NewClientBuilder().WithScheme(sch).WithObjects(objs...).Build()

	var coreObjs []runtime.Object
	for _, o := range objs {
		if n, ok := o.(*corev1.Node); ok {
			coreObjs = append(coreObjs, n)
		}
	}
	fakeClientset := fakek.NewSimpleClientset(coreObjs...)

	cs := &client.ClientSet{
		Client:          fakeClient,
		CoreV1Interface: fakeClientset.CoreV1(),
	}

	return &endpointslices.EndpointSlicesExporter{ClientSet: cs}
}

var _ = g.Describe("Dynamic Ranges", func() {
	g.Context("NodePort dynamic range", func() {
		g.It("returns default when unset", func() {
			network := &configv1.Network{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
				Spec:       configv1.NetworkSpec{ServiceNodePortRange: ""},
			}
			exporter := newExporterWithObjects(network)

			got, err := getNodePortDynamicRange(exporter)
			o.Expect(err).ToNot(o.HaveOccurred())
			o.Expect(got).To(o.Equal(types.KubeletNodePortDefaultDynamicRange))
		})

		g.It("uses specified range", func() {
			network := &configv1.Network{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
				Spec:       configv1.NetworkSpec{ServiceNodePortRange: "10000-10100"},
			}
			exporter := newExporterWithObjects(network)

			got, err := getNodePortDynamicRange(exporter)
			o.Expect(err).ToNot(o.HaveOccurred())
			o.Expect(got).To(o.HaveLen(2))
			o.Expect(got[0].MinPort).To(o.Equal(10000))
			o.Expect(got[0].MaxPort).To(o.Equal(10100))
			o.Expect(got[0].Protocol).To(o.Equal("TCP"))
			o.Expect(got[1].MinPort).To(o.Equal(10000))
			o.Expect(got[1].MaxPort).To(o.Equal(10100))
			o.Expect(got[1].Protocol).To(o.Equal("UDP"))
		})
	})
})
