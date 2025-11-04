package commatrixcreator

import (
	"fmt"
	"slices"

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
	configv1 "github.com/openshift/api/config/v1"
	machineconfigurationv1 "github.com/openshift/api/machineconfiguration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fakek "k8s.io/client-go/kubernetes/fake"
)

// Global uninitialized vars.
var (
	endpointSlices    *endpointslices.EndpointSlicesExporter
	nilComDetailsList []types.ComDetails
)

// Example Comdetails lists.
var (
	exampleComDetailsList = []types.ComDetails{
		{
			Direction: "ingress",
			Protocol:  "TCP",
			Port:      9050,
			Namespace: "example-namespace",
			Service:   "example-service",
			Pod:       "example-pod",
			Container: "example-container",
			NodeGroup: "master",
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
			NodeGroup: "worker",
			Optional:  false,
		},
	}

	testEpsComDetails = []types.ComDetails{
		{
			Direction: "Ingress",
			Protocol:  "TCP",
			Port:      80,
			Namespace: "test-ns",
			Service:   "test-service",
			Pod:       "test-pod",
			Container: "test-container",
			NodeGroup: "master",
			Optional:  false,
		},
	}
)

// Test resources.
var (
	mcpMaster = &machineconfigurationv1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "master"},
		Spec: machineconfigurationv1.MachineConfigPoolSpec{
			NodeSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"node-role.kubernetes.io/master": ""}},
		},
		Status: machineconfigurationv1.MachineConfigPoolStatus{MachineCount: 1},
	}

	mcpWorker = &machineconfigurationv1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "worker"},
		Spec: machineconfigurationv1.MachineConfigPoolSpec{
			NodeSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"node-role.kubernetes.io/worker": ""}},
		},
		Status: machineconfigurationv1.MachineConfigPoolStatus{MachineCount: 1},
	}

	testNode = &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Annotations: map[string]string{
				"machineconfiguration.openshift.io/currentConfig": "rendered-master-abc",
			},
			Labels: map[string]string{
				"node-role.kubernetes.io/master": "",
			},
		},
	}

	testNodeWorker = &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node-worker",
			Annotations: map[string]string{
				"machineconfiguration.openshift.io/currentConfig": "rendered-worker-abc",
			},
			Labels: map[string]string{
				"node-role.kubernetes.io/worker": "",
			},
		},
	}

	testPod = &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-app-pod",
			Namespace: "test-ns",
			Labels: map[string]string{
				"kubernetes.io/service-name": "test-service",
				"app":                        "test-app",
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

	testService = &corev1.Service{
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

	testEndpointSlice = &discoveryv1.EndpointSlice{
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
)

var _ = g.Describe("Commatrix creator pkg tests", func() {
	g.Context("Get Costume entries List From File", func() {
		for _, format := range []string{types.FormatCSV, types.FormatJSON, types.FormatYAML} {
			g.It(fmt.Sprintf("Should successfully extract ComDetails from a %s file", format), func() {
				g.By(fmt.Sprintf("Creating new communication matrix with %s static entries format", format))
				cm, err := New(nil, fmt.Sprintf("../../samples/custom-entries/example-custom-entries.%s", format), format, configv1.BareMetalPlatformType, 0, false)
				o.Expect(err).ToNot(o.HaveOccurred())

				g.By("Getting ComDetails List From File")
				gotComDetails, err := cm.GetComDetailsListFromFile()
				o.Expect(err).ToNot(o.HaveOccurred())

				g.By("Comparing gotten ComDetails to wanted ComDetials")
				o.Expect(gotComDetails).To(o.Equal(exampleComDetailsList))
			})
		}

		g.It("Should return an error due to non-matched customEntriesPath and customEntriesFormat types", func() {
			g.By("Creating new communication matrix with non-matched customEntriesPath and customEntriesFormat")
			cm, err := New(nil, "../../samples/custom-entries/example-custom-entries.csv", types.FormatJSON, configv1.BareMetalPlatformType, 0, false)
			o.Expect(err).ToNot(o.HaveOccurred())

			g.By("Getting ComDetails List From File")
			gotComDetails, err := cm.GetComDetailsListFromFile()
			o.Expect(err).To(o.HaveOccurred())

			g.By("Comparing gotten ComDetails to empty ComDetials")
			o.Expect(gotComDetails).To(o.Equal(nilComDetailsList))
		})

		g.It("Should return an error due to an invalid customEntriesFormat", func() {
			g.By("Creating new communication matrix with invalid customEntriesFormat")
			cm, err := New(nil, "../../samples/custom-entries/example-custom-entries.csv", types.FormatNFT, configv1.BareMetalPlatformType, 0, false)
			o.Expect(err).ToNot(o.HaveOccurred())

			g.By("Getting ComDetails List From File")
			gotComDetails, err := cm.GetComDetailsListFromFile()
			o.Expect(err).To(o.HaveOccurred())

			g.By("Comparing gotten ComDetails to empty ComDetials")
			o.Expect(gotComDetails).To(o.Equal(nilComDetailsList))
		})
	})

	g.Context("Get static entries from file", func() {
		g.It("Should successfully get static entries suitable to baremetal standard cluster", func() {
			g.By("Creating new communication matrix suitable to baremetal standard cluster")
			cm, err := New(nil, "", "", configv1.BareMetalPlatformType, types.Standard, false)
			o.Expect(err).ToNot(o.HaveOccurred())

			g.By("Getting static entries comDetails of the created communication matrix")
			gotComDetails, err := cm.GetStaticEntries()
			o.Expect(err).ToNot(o.HaveOccurred())

			g.By("Comparing gotten static entries to wanted comDetails")
			wantedComDetails := slices.Concat(types.BaremetalStaticEntriesMaster, types.BaremetalStaticEntriesWorker,
				types.GeneralStaticEntriesMaster, types.StandardStaticEntries, types.GeneralStaticEntriesWorker)
			o.Expect(gotComDetails).To(o.Equal(wantedComDetails))
		})

		g.It("Should successfully get static entries suitable to baremetal SNO cluster", func() {
			g.By("Creating new communication matrix suitable to baremetal SNO cluster")
			cm, err := New(nil, "", "", configv1.BareMetalPlatformType, types.SNO, false)
			o.Expect(err).ToNot(o.HaveOccurred())

			g.By("Getting static entries comDetails of the created communication matrix")
			gotComDetails, err := cm.GetStaticEntries()
			o.Expect(err).ToNot(o.HaveOccurred())

			g.By("Comparing gotten static entries to wanted comDetails")
			wantedComDetails := slices.Concat(types.BaremetalStaticEntriesMaster, types.GeneralStaticEntriesMaster)
			o.Expect(gotComDetails).To(o.Equal(wantedComDetails))
		})

		g.It("Should return an error due to an invalid value for cluster environment", func() {
			g.By("Creating new communication matrix with an invalid value for cluster environment")
			cm, err := New(nil, "", "", "invalid", types.SNO, false)
			o.Expect(err).ToNot(o.HaveOccurred())

			g.By("Getting static entries comDetails of the created communication matrix")
			gotComDetails, err := cm.GetStaticEntries()
			o.Expect(err).To(o.HaveOccurred())

			g.By("Comparing gotten static entries to empty comDetails")
			o.Expect(gotComDetails).To(o.Equal(nilComDetailsList))
		})

	})

	g.Context("Create EndpointSlice Matrix", func() {
		g.BeforeEach(func() {
			sch := runtime.NewScheme()

			err := corev1.AddToScheme(sch)
			o.Expect(err).NotTo(o.HaveOccurred())
			err = discoveryv1.AddToScheme(sch)
			o.Expect(err).NotTo(o.HaveOccurred())
			err = machineconfigurationv1.AddToScheme(sch)
			o.Expect(err).NotTo(o.HaveOccurred())

			fakeClient := fake.NewClientBuilder().WithScheme(sch).WithObjects(testNode, testNodeWorker, testPod, testService, testEndpointSlice, mcpWorker, mcpMaster).Build()
			fakeClientset := fakek.NewSimpleClientset()

			clientset := &client.ClientSet{
				Client:          fakeClient,
				CoreV1Interface: fakeClientset.CoreV1(),
			}

			endpointSlices, err = endpointslices.New(clientset)
			o.Expect(err).ToNot(o.HaveOccurred())
		})

		g.It("Should successfully create an endpoint matrix with custom entries", func() {
			g.By("Creating new communication matrix with static entries")
			commatrixCreator, err := New(endpointSlices, "../../samples/custom-entries/example-custom-entries.csv", types.FormatCSV, configv1.AWSPlatformType, types.SNO, false)
			o.Expect(err).ToNot(o.HaveOccurred())
			commatrix, err := commatrixCreator.CreateEndpointMatrix()
			o.Expect(err).ToNot(o.HaveOccurred())

			g.By("Generating wanted comDetails based on cluster features")
			wantedComDetails := slices.Concat(testEpsComDetails, types.GeneralStaticEntriesMaster)

			g.By("Add to wanted comDetails the example static entries")
			wantedComDetails = slices.Concat(wantedComDetails, exampleComDetailsList)

			wantedComMatrix := types.ComMatrix{Matrix: wantedComDetails}
			wantedComMatrix.SortAndRemoveDuplicates()

			g.By("Generate diff between created commatrix and eanted commatrix")
			diff := matrixdiff.Generate(&wantedComMatrix, commatrix)

			g.By("Checking whether diff is empty")
			o.Expect(diff.GetUniquePrimary().Matrix).To(o.BeEmpty())
			o.Expect(diff.GetUniqueSecondary().Matrix).To(o.BeEmpty())
		})

		g.It("Should successfully create an endpoint matrix without custom entries", func() {
			g.By("Creating new communication matrix without static entries")
			commatrixCreator, err := New(endpointSlices, "", "", configv1.AWSPlatformType, types.SNO, false)
			o.Expect(err).ToNot(o.HaveOccurred())
			commatrix, err := commatrixCreator.CreateEndpointMatrix()
			o.Expect(err).ToNot(o.HaveOccurred())

			g.By("Generating wanted comDetails")
			wantedComDetails := slices.Concat(testEpsComDetails, types.GeneralStaticEntriesMaster)
			wantedComMatrix := types.ComMatrix{Matrix: wantedComDetails}
			wantedComMatrix.SortAndRemoveDuplicates()

			g.By("Generate diff between created commatrix and eanted commatrix")
			diff := matrixdiff.Generate(&wantedComMatrix, commatrix)

			g.By("Checking whether diff is empty")
			o.Expect(diff.GetUniquePrimary().Matrix).To(o.BeEmpty())
			o.Expect(diff.GetUniqueSecondary().Matrix).To(o.BeEmpty())
		})

		g.It("Should include IPv6 static entries when ipv6Enabled is true on Standard", func() {
			g.By("Creating communication matrix with ipv6Enabled=true for Standard")
			commatrixCreator, err := New(endpointSlices, "", "", configv1.AWSPlatformType, types.Standard, true)
			o.Expect(err).ToNot(o.HaveOccurred())
			commatrix, err := commatrixCreator.CreateEndpointMatrix()
			o.Expect(err).ToNot(o.HaveOccurred())

			g.By("Building expected details: eps + general master/worker + standard + ipv6 master/worker")
			wanted := slices.Concat(testEpsComDetails,
				types.StandardStaticEntries,
				types.GeneralStaticEntriesMaster,
				types.GeneralStaticEntriesWorker,
				types.GeneralIPv6StaticEntriesMaster,
				types.GeneralIPv6StaticEntriesWorker,
			)
			wantedMatrix := types.ComMatrix{Matrix: wanted}
			wantedMatrix.SortAndRemoveDuplicates()

			diff := matrixdiff.Generate(&wantedMatrix, commatrix)
			o.Expect(diff.GetUniquePrimary().Matrix).To(o.BeEmpty())
			o.Expect(diff.GetUniqueSecondary().Matrix).To(o.BeEmpty())
		})
	})

	g.Context("expandEntriesForPools", func() {
		g.It("should fan-out role-scoped entries to all matching pools", func() {
			// Given two static entries: one for master and one for worker
			entries := []types.ComDetails{
				{
					Direction: "Ingress",
					Protocol:  "TCP",
					Port:      1000,
					Namespace: "ns1",
					Service:   "svc1",
					Pod:       "pod1",
					Container: "c1",
					NodeGroup: "master",
					Optional:  false,
				},
				{
					Direction: "Ingress",
					Protocol:  "UDP",
					Port:      2000,
					Namespace: "ns2",
					Service:   "svc2",
					Pod:       "pod2",
					Container: "c2",
					NodeGroup: "worker",
					Optional:  true,
				},
			}

			// And pool roles: poolA has master+worker, poolB has worker, poolC has master
			poolToRoles := map[string][]string{
				"poolA": {"master", "worker"},
				"poolB": {"worker"},
				"poolC": {"master"},
			}

			out := expandStaticEntriesByPool(entries, poolToRoles)

			// Expect one entry for master in poolA, and worker duplicated for poolA and poolB and one for master poolC
			expected := []types.ComDetails{
				{
					Direction: "Ingress",
					Protocol:  "TCP",
					Port:      1000,
					Namespace: "ns1",
					Service:   "svc1",
					Pod:       "pod1",
					Container: "c1",
					NodeGroup: "poolC",
					Optional:  false,
				},
				{
					Direction: "Ingress",
					Protocol:  "TCP",
					Port:      1000,
					Namespace: "ns1",
					Service:   "svc1",
					Pod:       "pod1",
					Container: "c1",
					NodeGroup: "poolA",
					Optional:  false,
				},
				{
					Direction: "Ingress",
					Protocol:  "UDP",
					Port:      2000,
					Namespace: "ns2",
					Service:   "svc2",
					Pod:       "pod2",
					Container: "c2",
					NodeGroup: "poolA",
					Optional:  true,
				},
				{
					Direction: "Ingress",
					Protocol:  "UDP",
					Port:      2000,
					Namespace: "ns2",
					Service:   "svc2",
					Pod:       "pod2",
					Container: "c2",
					NodeGroup: "poolB",
					Optional:  true,
				},
			}

			// Compare ignoring order
			gotMatrix := types.ComMatrix{Matrix: out}
			gotMatrix.SortAndRemoveDuplicates()
			expectedMatrix := types.ComMatrix{Matrix: expected}
			expectedMatrix.SortAndRemoveDuplicates()
			o.Expect(gotMatrix.Matrix).To(o.Equal(expectedMatrix.Matrix))
		})
	})
})
