package dynamicranges

import (
	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	configv1 "github.com/openshift/api/config/v1"
	machineconfigurationv1 "github.com/openshift/api/machineconfiguration/v1"
	"go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	fakek "k8s.io/client-go/kubernetes/fake"
	rtclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/openshift-kni/commatrix/pkg/client"
	"github.com/openshift-kni/commatrix/pkg/consts"
	"github.com/openshift-kni/commatrix/pkg/endpointslices"
	"github.com/openshift-kni/commatrix/pkg/types"
	mock_utils "github.com/openshift-kni/commatrix/pkg/utils/mock"
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

	g.Context("Linux dynamic/private range", func() {
		g.It("returns default range", func() {
			// Need at least one node to select
			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "node-0",
					Labels: map[string]string{"node-role.kubernetes.io/master": ""},
				},
			}
			exporter := newExporterWithObjects(node)

			ctrl := gomock.NewController(g.GinkgoT())
			defer ctrl.Finish()

			mockUtils := mock_utils.NewMockUtilsInterface(ctrl)

			mockUtils.EXPECT().CreateNamespace(consts.DefaultDebugNamespace).Return(nil)
			mockUtils.EXPECT().DeleteNamespace(consts.DefaultDebugNamespace).Return(nil)

			// Pod lifecycle
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "debug-pod",
					Namespace: consts.DefaultDebugNamespace,
				},
				Status: corev1.PodStatus{Phase: corev1.PodRunning},
			}
			mockUtils.EXPECT().CreatePodOnNode(gomock.Any(), consts.DefaultDebugNamespace, gomock.Any(), gomock.Any()).Return(pod, nil)
			mockUtils.EXPECT().DeletePod(pod).Return(nil)
			mockUtils.EXPECT().WaitForPodStatus(consts.DefaultDebugNamespace, pod, corev1.PodRunning).Return(nil)

			// Simulate ip_local_port_range not set (empty output)
			mockUtils.EXPECT().RunCommandOnPod(pod, gomock.Any()).Return([]byte(""), nil)

			got, err := getLinuxDynamicPrivateRange(exporter, mockUtils)
			o.Expect(err).ToNot(o.HaveOccurred())
			o.Expect(got).To(o.Equal(types.LinuxDynamicPrivateDefaultDynamicRange))
		})

		g.It("uses range from pod value", func() {
			// Need at least one node to select
			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "node-0",
					Labels: map[string]string{"node-role.kubernetes.io/master": ""},
				},
			}
			exporter := newExporterWithObjects(node)

			ctrl := gomock.NewController(g.GinkgoT())
			defer ctrl.Finish()

			mockUtils := mock_utils.NewMockUtilsInterface(ctrl)

			mockUtils.EXPECT().CreateNamespace(consts.DefaultDebugNamespace).Return(nil)
			mockUtils.EXPECT().DeleteNamespace(consts.DefaultDebugNamespace).Return(nil)

			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "debug-pod",
					Namespace: consts.DefaultDebugNamespace,
				},
				Status: corev1.PodStatus{Phase: corev1.PodRunning},
			}
			mockUtils.EXPECT().CreatePodOnNode(gomock.Any(), consts.DefaultDebugNamespace, gomock.Any(), gomock.Any()).Return(pod, nil)
			mockUtils.EXPECT().DeletePod(pod).Return(nil)
			mockUtils.EXPECT().WaitForPodStatus(consts.DefaultDebugNamespace, pod, corev1.PodRunning).Return(nil)

			// Custom ephemeral range values
			mockUtils.EXPECT().RunCommandOnPod(pod, gomock.Any()).Return([]byte("40000 50000\n"), nil)

			got, err := getLinuxDynamicPrivateRange(exporter, mockUtils)
			o.Expect(err).ToNot(o.HaveOccurred())
			o.Expect(got).To(o.HaveLen(2))
			o.Expect(got[0].MinPort).To(o.Equal(40000))
			o.Expect(got[0].MaxPort).To(o.Equal(50000))
			o.Expect(got[0].Protocol).To(o.Equal("TCP"))
			o.Expect(got[1].MinPort).To(o.Equal(40000))
			o.Expect(got[1].MaxPort).To(o.Equal(50000))
			o.Expect(got[1].Protocol).To(o.Equal("UDP"))
		})
	})
})
