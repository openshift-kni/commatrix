package commatrixcreator

import (
	"fmt"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/openshift-kni/commatrix/pkg/client"
	"github.com/openshift-kni/commatrix/pkg/endpointslices"
	matrixdiff "github.com/openshift-kni/commatrix/pkg/matrix-diff"
	"github.com/openshift-kni/commatrix/pkg/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fakek "k8s.io/client-go/kubernetes/fake"
)

var endpointSlices *endpointslices.EndpointSlicesExporter
var nilComDetailsList []types.ComDetails
var testEpsComDetails []types.ComDetails
var exampleComDetailsList = []types.ComDetails{
	{
		Direction: "ingress",
		Protocol:  "TCP",
		Port:      9050,
		Namespace: "example-namespace",
		Service:   "example-service",
		Pod:       "example-pod",
		Container: "example-container",
		NodeRole:  "master",
		Optional:  false,
	},
	{
		Direction: "ingress",
		Protocol:  "UDP",
		Port:      9051,
		Namespace: "example-namespace2",
		Service:   "example-service2",
		Pod:       "example-pod2",
		Container: "example-container2",
		NodeRole:  "worker",
		Optional:  false,
	},
}

var _ = g.Describe("Commatrix", func() {
	g.Context("Get ComDetails List From File", func() {
		for _, format := range []string{types.FormatCSV, types.FormatJSON, types.FormatYAML} {
			g.It(fmt.Sprintf("Should successfully extract ComDetails from a %s file", format), func() {
				cm, err := New(nil, fmt.Sprintf("../../samples/custom-entries/example-custom-entries.%s", format), format, 0, 0)
				o.Expect(err).ToNot(o.HaveOccurred())
				gotComDetails, err := cm.GetComDetailsListFromFile()
				o.Expect(err).ToNot(o.HaveOccurred())
				o.Expect(gotComDetails).To(o.Equal(exampleComDetailsList))
			})
		}

		g.It("Should return an error due to non-matched customEntriesPath and customEntriesFormat types", func() {
			cm, err := New(nil, "../../samples/custom-entries/example-custom-entries.csv", types.FormatJSON, 0, 0)
			o.Expect(err).ToNot(o.HaveOccurred())
			gotComDetails, err := cm.GetComDetailsListFromFile()
			o.Expect(err).To(o.HaveOccurred())
			o.Expect(gotComDetails).To(o.Equal(nilComDetailsList))
		})

		g.It("Should return an error due to an invalid customEntriesFormat", func() {
			cm, err := New(nil, "../../samples/custom-entries/example-custom-entries.csv", types.FormatNFT, 0, 0)
			o.Expect(err).ToNot(o.HaveOccurred())
			gotComDetails, err := cm.GetComDetailsListFromFile()
			o.Expect(err).To(o.HaveOccurred())
			o.Expect(gotComDetails).To(o.Equal(nilComDetailsList))
		})
	})

	g.Context("Get static enteries from file", func() {
		g.It("Should successfully get static entries suitable to baremetal standard cluster", func() {
			cm, err := New(nil, "", "", types.Baremetal, types.Standard)
			o.Expect(err).ToNot(o.HaveOccurred())
			gotComDetails, err := cm.getStaticEntries()
			o.Expect(err).ToNot(o.HaveOccurred())
			wantedComDetails := append(append(append(append(types.BaremetalStaticEntriesMaster, types.BaremetalStaticEntriesWorker...),
				types.GeneralStaticEntriesMaster...), types.StandardStaticEntries...), types.GeneralStaticEntriesWorker...)
			o.Expect(gotComDetails).To(o.Equal(wantedComDetails))
		})

		g.It("Should successfully get static entries suitable to baremetal SNO cluster", func() {
			cm, err := New(nil, "", "", types.Baremetal, types.SNO)
			o.Expect(err).ToNot(o.HaveOccurred())
			gotComDetails, err := cm.getStaticEntries()
			o.Expect(err).ToNot(o.HaveOccurred())
			wantedComDetails := append(types.BaremetalStaticEntriesMaster, types.GeneralStaticEntriesMaster...)
			o.Expect(gotComDetails).To(o.Equal(wantedComDetails))
		})

		g.It("Should successfully get static entries suitable to cloud standard cluster", func() {
			cm, err := New(nil, "", "", types.Cloud, types.Standard)
			o.Expect(err).ToNot(o.HaveOccurred())
			gotComDetails, err := cm.getStaticEntries()
			o.Expect(err).ToNot(o.HaveOccurred())
			wantedComDetails := append(append(append(append(types.CloudStaticEntriesMaster, types.CloudStaticEntriesWorker...),
				types.GeneralStaticEntriesMaster...), types.StandardStaticEntries...), types.GeneralStaticEntriesWorker...)
			o.Expect(gotComDetails).To(o.Equal(wantedComDetails))
		})

		g.It("Should successfully get static entries suitable to cloud SNO cluster", func() {
			cm, err := New(nil, "", "", types.Cloud, types.SNO)
			o.Expect(err).ToNot(o.HaveOccurred())
			gotComDetails, err := cm.getStaticEntries()
			o.Expect(err).ToNot(o.HaveOccurred())
			wantedComDetails := append(types.CloudStaticEntriesMaster, types.GeneralStaticEntriesMaster...)
			o.Expect(gotComDetails).To(o.Equal(wantedComDetails))
		})

		g.It("Should return an error due to an invalid value for cluster environment", func() {
			cm, err := New(nil, "", "", -1, types.SNO)
			o.Expect(err).ToNot(o.HaveOccurred())
			gotComDetails, err := cm.getStaticEntries()
			o.Expect(err).To(o.HaveOccurred())
			o.Expect(gotComDetails).To(o.Equal(nilComDetailsList))
		})

	})

	g.Context("Create Endpoint Matrix", func() {
		g.BeforeEach(func() {
			sch := runtime.NewScheme()

			err := corev1.AddToScheme(sch)
			o.Expect(err).NotTo(o.HaveOccurred())
			err = discoveryv1.AddToScheme(sch)
			o.Expect(err).NotTo(o.HaveOccurred())

			testNode := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-node",
					Namespace: "test-ns",
					Labels: map[string]string{
						"node-role.kubernetes.io/master": "",
					},
				},
			}

			testPod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-app-pod",
					Namespace: "test-ns",
					Labels: map[string]string{
						"kubernetes.io/service-name": "test-service",
						"app": "test-app",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "test-container",
							Image: "test-image:latest",
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 80,
								},
							},
						},
					},
				},
			}

			testService := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-service",
					Namespace: "test-ns",
					Labels: map[string]string{
						"kubernetes.io/service-name": "test-service",
					},
				},
				Spec: corev1.ServiceSpec{
					Selector: map[string]string{
						"app": "test-app",
					},
					Ports: []corev1.ServicePort{
						{
							Port:     80,
							Protocol: corev1.ProtocolTCP,
						},
					},
					Type: corev1.ServiceTypeNodePort,
				},
			}

			testEndpointSlice := &discoveryv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-service-endpoints",
					Namespace: "test-ns",
					Labels: map[string]string{
						"kubernetes.io/service-name": "test-service",
					},
					OwnerReferences: []metav1.OwnerReference{
						{
							Kind: "Service",
							Name: "test-service",
						},
					},
				},
				AddressType: discoveryv1.AddressTypeIPv4,
				Endpoints: []discoveryv1.Endpoint{
					{
						NodeName:  &testNode.Name,
						Addresses: []string{"192.168.1.1", "192.168.1.2"},
					},
				},
				Ports: []discoveryv1.EndpointPort{
					{
						Name:     nil,
						Port:     func(i int32) *int32 { return &i }(80),
						Protocol: func(p corev1.Protocol) *corev1.Protocol { return &p }(corev1.ProtocolTCP),
					},
				},
			}

			fakeClient := fake.NewClientBuilder().WithScheme(sch).WithObjects(testNode, testPod, testService, testEndpointSlice).Build()
			fakeClientset := fakek.NewSimpleClientset()

			clientset := &client.ClientSet{
				Client:          fakeClient,
				CoreV1Interface: fakeClientset.CoreV1(),
			}

			testEpsComDetails = []types.ComDetails{
				{
					Direction: "Ingress",
					Protocol: "TCP",
					Port: 80,
					Namespace: "test-ns",
					Service: "test-service",
					Pod: "test-pod",
					Container: "test-container",
					NodeRole: "master",
					Optional: false,
				},
			}

			endpointSlices, err = endpointslices.New(clientset)
			o.Expect(err).ToNot(o.HaveOccurred())
		})

		g.It("Should successfully create an endpoint matrix with custom entries", func() {
			commatrixCreator, err := New(endpointSlices, "../../samples/custom-entries/example-custom-entries.csv", types.FormatCSV, types.Cloud, types.SNO)
			o.Expect(err).ToNot(o.HaveOccurred())
			commatrix, err := commatrixCreator.CreateEndpointMatrix()
			o.Expect(err).ToNot(o.HaveOccurred())

			wantedComDetails := append(append(append(testEpsComDetails, types.CloudStaticEntriesMaster...), types.GeneralStaticEntriesMaster...), exampleComDetailsList...)
			wantedComMatrix := types.ComMatrix{Matrix: wantedComDetails}
			wantedComMatrix.SortAndRemoveDuplicates()
			
			diff := matrixdiff.Generate(&wantedComMatrix, commatrix)
			o.Expect(diff.GenerateUniquePrimary().Matrix).To(o.BeEmpty())
			o.Expect(diff.GenerateUniqueSecondary().Matrix).To(o.BeEmpty())
		})

		g.It("Should successfully create an endpoint matrix without custom entries", func() {
			commatrixCreator, err := New(endpointSlices, "", "", types.Cloud, types.SNO)
			o.Expect(err).ToNot(o.HaveOccurred())
			commatrix, err := commatrixCreator.CreateEndpointMatrix()
			o.Expect(err).ToNot(o.HaveOccurred())

			wantedComDetails := append(append(testEpsComDetails, types.CloudStaticEntriesMaster...), types.GeneralStaticEntriesMaster...)
			wantedComMatrix := types.ComMatrix{Matrix: wantedComDetails}
			wantedComMatrix.SortAndRemoveDuplicates()
			
			diff := matrixdiff.Generate(&wantedComMatrix, commatrix)
			o.Expect(diff.GenerateUniquePrimary().Matrix).To(o.BeEmpty())
			o.Expect(diff.GenerateUniqueSecondary().Matrix).To(o.BeEmpty())
		})
	})
})
