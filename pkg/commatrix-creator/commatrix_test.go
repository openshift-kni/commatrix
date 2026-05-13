package commatrixcreator

import (
	"fmt"
	"slices"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	gomock "go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/openshift-kni/commatrix/pkg/client"
	"github.com/openshift-kni/commatrix/pkg/consts"
	"github.com/openshift-kni/commatrix/pkg/endpointslices"
	matrixdiff "github.com/openshift-kni/commatrix/pkg/matrix-diff"
	"github.com/openshift-kni/commatrix/pkg/types"
	mock_utils "github.com/openshift-kni/commatrix/pkg/utils/mock"
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
			Protocol:  consts.ProtocolTCP,
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
			Protocol:  consts.ProtocolUDP,
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
			Protocol:  consts.ProtocolTCP,
			Port:      80,
			Namespace: "test-ns",
			Service:   "test-service",
			Pod:       "test-pod",
			Container: "test-container",
			NodeGroup: "master",
			Optional:  false,
		},
	}
	exampleDynamicRanges = types.DynamicRangeList{
		{
			Direction:   "ingress",
			Protocol:    consts.ProtocolTCP,
			MinPort:     9000,
			MaxPort:     9999,
			Description: "example dynamic range",
			Optional:    false,
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
			HostNetwork: true,
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

	// Test resources for localhost filtering test.
	testPodWithLocalhostPorts = &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "localhost-test-pod",
			Namespace: "localhost-ns",
			Labels: map[string]string{
				"app": "localhost-app",
			},
		},
		Spec: corev1.PodSpec{
			HostNetwork: true,
			Containers: []corev1.Container{
				{
					Name:  "main-container",
					Image: "test-image:latest",
					Ports: []corev1.ContainerPort{
						{
							ContainerPort: 8080,
							HostIP:        "127.0.0.1", // Should be filtered out
						},
						{
							ContainerPort: 9090,
							HostIP:        "0.0.0.0", // Should NOT be filtered out
						},
						{
							ContainerPort: 3000,
							HostIP:        "::1", // Should be filtered out
						},
					},
				},
			},
		},
	}

	testServiceLocalhost = &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "localhost-service",
			Namespace: "localhost-ns",
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app": "localhost-app",
			},
			Ports: []corev1.ServicePort{
				{
					Name:     "port-8080",
					Port:     8080,
					Protocol: corev1.ProtocolTCP,
				},
				{
					Name:     "port-9090",
					Port:     9090,
					Protocol: corev1.ProtocolTCP,
				},
				{
					Name:     "port-3000",
					Port:     3000,
					Protocol: corev1.ProtocolTCP,
				},
			},
			Type: corev1.ServiceTypeNodePort,
		},
	}

	testEndpointSliceLocalhost = &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "localhost-service-endpoints",
			Namespace: "localhost-ns",
			Labels: map[string]string{
				"kubernetes.io/service-name": "localhost-service",
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					Kind: "Service",
					Name: "localhost-service",
				},
			},
		},
		AddressType: discoveryv1.AddressTypeIPv4,
		Endpoints: []discoveryv1.Endpoint{
			{
				NodeName:  &testNode.Name,
				Addresses: []string{"192.168.1.10"},
			},
		},
		Ports: []discoveryv1.EndpointPort{
			{
				Name:     func(s string) *string { return &s }("port-8080"),
				Port:     func(i int32) *int32 { return &i }(8080),
				Protocol: func(p corev1.Protocol) *corev1.Protocol { return &p }(corev1.ProtocolTCP),
			},
			{
				Name:     func(s string) *string { return &s }("port-9090"),
				Port:     func(i int32) *int32 { return &i }(9090),
				Protocol: func(p corev1.Protocol) *corev1.Protocol { return &p }(corev1.ProtocolTCP),
			},
			{
				Name:     func(s string) *string { return &s }("port-3000"),
				Port:     func(i int32) *int32 { return &i }(3000),
				Protocol: func(p corev1.Protocol) *corev1.Protocol { return &p }(corev1.ProtocolTCP),
			},
		},
	}

	// Test resources for non-hostNetwork pod behind NodePort service (no hostPort).
	testPodNonHostNet = &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "non-hostnet-pod",
			Namespace: "non-hostnet-ns",
			Labels:    map[string]string{"app": "non-hostnet-app"},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "web",
					Image: "test-image:latest",
					Ports: []corev1.ContainerPort{
						{ContainerPort: 8080},
					},
				},
			},
		},
	}

	testServiceNonHostNet = &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "non-hostnet-service",
			Namespace: "non-hostnet-ns",
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": "non-hostnet-app"},
			Ports: []corev1.ServicePort{
				{Port: 8080, Protocol: corev1.ProtocolTCP},
			},
			Type: corev1.ServiceTypeNodePort,
		},
	}

	testEndpointSliceNonHostNet = &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "non-hostnet-service-eps",
			Namespace: "non-hostnet-ns",
			Labels:    map[string]string{"kubernetes.io/service-name": "non-hostnet-service"},
			OwnerReferences: []metav1.OwnerReference{
				{Kind: "Service", Name: "non-hostnet-service"},
			},
		},
		AddressType: discoveryv1.AddressTypeIPv4,
		Endpoints: []discoveryv1.Endpoint{
			{
				NodeName:  &testNode.Name,
				Addresses: []string{"10.0.0.1"},
			},
		},
		Ports: []discoveryv1.EndpointPort{
			{
				Port:     func(i int32) *int32 { return &i }(8080),
				Protocol: func(p corev1.Protocol) *corev1.Protocol { return &p }(corev1.ProtocolTCP),
			},
		},
	}

	// Test resources for non-hostNetwork pod with explicit hostPort on one port.
	testPodNonHostNetWithHostPort = &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "non-hostnet-hostport-pod",
			Namespace: "non-hostnet-hp-ns",
			Labels:    map[string]string{"app": "non-hostnet-hp-app"},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "web",
					Image: "test-image:latest",
					Ports: []corev1.ContainerPort{
						{ContainerPort: 8080, HostPort: 8080},
						{ContainerPort: 9090},
					},
				},
			},
		},
	}

	testServiceNonHostNetWithHostPort = &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "non-hostnet-hp-service",
			Namespace: "non-hostnet-hp-ns",
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": "non-hostnet-hp-app"},
			Ports: []corev1.ServicePort{
				{Name: "port-8080", Port: 8080, Protocol: corev1.ProtocolTCP},
				{Name: "port-9090", Port: 9090, Protocol: corev1.ProtocolTCP},
			},
			Type: corev1.ServiceTypeNodePort,
		},
	}

	testEndpointSliceNonHostNetWithHostPort = &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "non-hostnet-hp-service-eps",
			Namespace: "non-hostnet-hp-ns",
			Labels:    map[string]string{"kubernetes.io/service-name": "non-hostnet-hp-service"},
			OwnerReferences: []metav1.OwnerReference{
				{Kind: "Service", Name: "non-hostnet-hp-service"},
			},
		},
		AddressType: discoveryv1.AddressTypeIPv4,
		Endpoints: []discoveryv1.Endpoint{
			{
				NodeName:  &testNode.Name,
				Addresses: []string{"10.0.0.2"},
			},
		},
		Ports: []discoveryv1.EndpointPort{
			{
				Name:     func(s string) *string { return &s }("port-8080"),
				Port:     func(i int32) *int32 { return &i }(8080),
				Protocol: func(p corev1.Protocol) *corev1.Protocol { return &p }(corev1.ProtocolTCP),
			},
			{
				Name:     func(s string) *string { return &s }("port-9090"),
				Port:     func(i int32) *int32 { return &i }(9090),
				Protocol: func(p corev1.Protocol) *corev1.Protocol { return &p }(corev1.ProtocolTCP),
			},
		},
	}

	testNetwork = &configv1.Network{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
		},
		Spec: configv1.NetworkSpec{
			ServiceNodePortRange: "1024-65535",
		},
	}
)

var _ = g.Describe("Commatrix creator pkg tests", func() {
	g.Context("Get Costume entries List From File", func() {
		for _, format := range []string{types.FormatCSV, types.FormatJSON, types.FormatYAML} {
			g.It(fmt.Sprintf("Should successfully extract ComDetails from a %s file", format), func() {
				g.By(fmt.Sprintf("Creating new communication matrix with %s static entries format", format))
				cm := New(
					configv1.BareMetalPlatformType,
					configv1.HighlyAvailableTopologyMode,
					WithCustomEntries(
						fmt.Sprintf("../../samples/custom-entries/example-custom-entries.%s", format),
						format,
					),
				)

				g.By("Getting ComMatrix From File")
				gotComMatrix, err := cm.GetComMatrixFromFile()
				o.Expect(err).ToNot(o.HaveOccurred())

				g.By("Comparing gotten ComMatrix.Ports to wanted ComDetials")
				o.Expect(gotComMatrix.Ports).To(o.Equal(exampleComDetailsList))
				o.Expect(gotComMatrix.DynamicRanges).To(o.Equal(exampleDynamicRanges))
			})
		}

		g.It("Should return an error due to non-matched customEntriesPath and customEntriesFormat types", func() {
			g.By("Creating new communication matrix with non-matched customEntriesPath and customEntriesFormat")
			cm := New(
				configv1.BareMetalPlatformType,
				configv1.HighlyAvailableTopologyMode,
				WithCustomEntries(
					"../../samples/custom-entries/example-custom-entries.csv",
					types.FormatJSON,
				),
			)

			g.By("Getting ComMatrix From File")
			gotComMatrix, err := cm.GetComMatrixFromFile()
			o.Expect(err).To(o.HaveOccurred())

			g.By("Expecting nil ComMatrix on error")
			o.Expect(gotComMatrix).To(o.BeNil())
		})

		g.It("Should return an error due to an invalid customEntriesFormat", func() {
			g.By("Creating new communication matrix with invalid customEntriesFormat")
			cm := New(
				configv1.BareMetalPlatformType,
				configv1.HighlyAvailableTopologyMode,
				WithCustomEntries(
					"../../samples/custom-entries/example-custom-entries.csv",
					types.FormatNFT,
				),
			)

			g.By("Getting ComMatrix From File")
			gotComMatrix, err := cm.GetComMatrixFromFile()
			o.Expect(err).To(o.HaveOccurred())

			g.By("Expecting nil ComMatrix on error")
			o.Expect(gotComMatrix).To(o.BeNil())
		})
	})

	g.Context("Get static entries", func() {
		g.It("Should successfully get static entries suitable to baremetal standard cluster", func() {
			g.By("Creating new communication matrix suitable to baremetal standard cluster")
			cm := New(
				configv1.BareMetalPlatformType,
				configv1.HighlyAvailableTopologyMode,
			)

			g.By("Getting static entries comDetails of the created communication matrix")
			gotComDetails, err := cm.getStaticEntries()
			o.Expect(err).ToNot(o.HaveOccurred())

			g.By("Comparing gotten static entries to wanted comDetails")
			wantedComDetails := slices.Concat(types.BaremetalStaticEntriesMaster, types.BaremetalStaticEntriesWorker,
				types.GeneralStaticEntriesMaster, types.StandardStaticEntries, types.GeneralStaticEntriesWorker)
			o.Expect(gotComDetails).To(o.Equal(wantedComDetails))
		})

		g.It("Should successfully get static entries suitable to baremetal SNO cluster", func() {
			g.By("Creating new communication matrix suitable to baremetal SNO cluster")
			cm := New(
				configv1.BareMetalPlatformType,
				configv1.SingleReplicaTopologyMode,
			)

			g.By("Getting static entries comDetails of the created communication matrix")
			gotComDetails, err := cm.getStaticEntries()
			o.Expect(err).ToNot(o.HaveOccurred())

			g.By("Comparing gotten static entries to wanted comDetails")
			wantedComDetails := slices.Concat(types.BaremetalStaticEntriesMaster, types.GeneralStaticEntriesMaster)
			o.Expect(gotComDetails).To(o.Equal(wantedComDetails))
		})

		g.It("Should successfully get static entries suitable to None standard cluster", func() {
			g.By("Creating new communication matrix suitable to None standard cluster")
			cm := New(
				configv1.NonePlatformType,
				configv1.HighlyAvailableTopologyMode,
			)

			g.By("Getting static entries comDetails of the created communication matrix")
			gotComDetails, err := cm.getStaticEntries()
			o.Expect(err).ToNot(o.HaveOccurred())

			g.By("Comparing gotten static entries to wanted comDetails")
			wantedComDetails := slices.Concat(types.NoneStaticEntriesMaster, types.NoneStaticEntriesWorker,
				types.GeneralStaticEntriesMaster, types.StandardStaticEntries, types.GeneralStaticEntriesWorker)
			o.Expect(gotComDetails).To(o.Equal(wantedComDetails))
		})

		g.It("Should successfully get static entries suitable to None SNO cluster", func() {
			g.By("Creating new communication matrix suitable to None SNO cluster")
			cm := New(
				configv1.NonePlatformType,
				configv1.SingleReplicaTopologyMode,
			)

			g.By("Getting static entries comDetails of the created communication matrix")
			gotComDetails, err := cm.getStaticEntries()
			o.Expect(err).ToNot(o.HaveOccurred())

			g.By("Comparing gotten static entries to wanted comDetails")
			wantedComDetails := slices.Concat(types.NoneStaticEntriesMaster, types.GeneralStaticEntriesMaster)
			o.Expect(gotComDetails).To(o.Equal(wantedComDetails))
		})

		g.It("Should include DHCP static entries when dhcpEnabled is true on standard cluster", func() {
			g.By("Creating new communication matrix with dhcpEnabled=true for standard cluster")
			cm := New(
				configv1.BareMetalPlatformType,
				configv1.HighlyAvailableTopologyMode,
				WithDHCP(),
			)

			g.By("Getting static entries comDetails of the created communication matrix")
			gotComDetails, err := cm.getStaticEntries()
			o.Expect(err).ToNot(o.HaveOccurred())

			g.By("Comparing gotten static entries to wanted comDetails")
			wantedComDetails := slices.Concat(types.BaremetalStaticEntriesMaster, types.BaremetalStaticEntriesWorker,
				types.GeneralStaticEntriesMaster, types.GeneralDHCPStaticEntriesMaster,
				types.StandardStaticEntries, types.GeneralStaticEntriesWorker, types.GeneralDHCPStaticEntriesWorker)
			o.Expect(gotComDetails).To(o.Equal(wantedComDetails))
		})

		g.It("Should include DHCP static entries when dhcpEnabled is true on SNO cluster", func() {
			g.By("Creating new communication matrix with dhcpEnabled=true for SNO cluster")
			cm := New(
				configv1.BareMetalPlatformType,
				configv1.SingleReplicaTopologyMode,
				WithDHCP(),
			)

			g.By("Getting static entries comDetails of the created communication matrix")
			gotComDetails, err := cm.getStaticEntries()
			o.Expect(err).ToNot(o.HaveOccurred())

			g.By("Comparing gotten static entries to wanted comDetails")
			wantedComDetails := slices.Concat(types.BaremetalStaticEntriesMaster,
				types.GeneralStaticEntriesMaster, types.GeneralDHCPStaticEntriesMaster)
			o.Expect(gotComDetails).To(o.Equal(wantedComDetails))
		})

		g.It("Should return an error due to an invalid value for cluster environment", func() {
			g.By("Creating new communication matrix with an invalid value for cluster environment")
			cm := New(
				"invalid",
				configv1.SingleReplicaTopologyMode,
			)

			g.By("Getting static entries comDetails of the created communication matrix")
			gotComDetails, err := cm.getStaticEntries()
			o.Expect(err).To(o.HaveOccurred())

			g.By("Comparing gotten static entries to empty comDetails")
			o.Expect(gotComDetails).To(o.Equal(nilComDetailsList))
		})
	})

	g.Context("Create EndpointSlice Matrix", func() {
		var ctrl *gomock.Controller
		var mockUtils *mock_utils.MockUtilsInterface

		g.BeforeEach(func() {
			sch := runtime.NewScheme()

			err := corev1.AddToScheme(sch)
			o.Expect(err).NotTo(o.HaveOccurred())
			err = discoveryv1.AddToScheme(sch)
			o.Expect(err).NotTo(o.HaveOccurred())
			err = machineconfigurationv1.AddToScheme(sch)
			o.Expect(err).NotTo(o.HaveOccurred())
			err = configv1.AddToScheme(sch)
			o.Expect(err).NotTo(o.HaveOccurred())

			fakeClient := fake.NewClientBuilder().WithScheme(sch).WithObjects(testNode, testNodeWorker, testPod, testService, testEndpointSlice, mcpWorker, mcpMaster, testNetwork).Build()
			fakeClientset := fakek.NewSimpleClientset(testNode, testNodeWorker)

			clientset := &client.ClientSet{
				Client:          fakeClient,
				CoreV1Interface: fakeClientset.CoreV1(),
			}

			endpointSlices, err = endpointslices.New(clientset, nil)
			o.Expect(err).ToNot(o.HaveOccurred())

			// Set up mock utils to avoid pod creation in tests
			ctrl = gomock.NewController(g.GinkgoT())
			mockUtils = mock_utils.NewMockUtilsInterface(ctrl)
			mockUtils.EXPECT().ListNodes().Return([]corev1.Node{*testNode, *testNodeWorker}, nil).AnyTimes()
		})

		g.AfterEach(func() {
			// Finish the controller
			if ctrl != nil {
				ctrl.Finish()
			}
		})

		g.It("Should successfully create an endpoint matrix with custom entries", func() {
			g.By("Creating new communication matrix with static entries")
			commatrixCreator := New(
				configv1.AWSPlatformType,
				configv1.SingleReplicaTopologyMode,
				WithExporter(endpointSlices),
				WithUtilsHelpers(mockUtils),
				WithCustomEntries(
					"../../samples/custom-entries/example-custom-entries.csv",
					types.FormatCSV,
				),
			)
			commatrix, err := commatrixCreator.CreateEndpointMatrix()
			o.Expect(err).ToNot(o.HaveOccurred())

			g.By("Generating wanted comDetails based on cluster features")
			staticEntries, err := types.GetStaticEntries(configv1.AWSPlatformType, configv1.SingleReplicaTopologyMode, false, false)
			o.Expect(err).ToNot(o.HaveOccurred())
			wantedComDetails := slices.Concat(testEpsComDetails, staticEntries)

			g.By("Add to wanted comDetails the example static entries")
			wantedComDetails = slices.Concat(wantedComDetails, exampleComDetailsList)

			wantedComMatrix := types.ComMatrix{Ports: wantedComDetails}
			wantedComMatrix.SortAndRemoveDuplicates()

			g.By("Generate diff between created commatrix and eanted commatrix")
			diff := matrixdiff.Generate(&wantedComMatrix, commatrix)

			g.By("Checking whether diff is empty")
			o.Expect(diff.GetUniquePrimary().Ports).To(o.BeEmpty())
			o.Expect(diff.GetUniqueSecondary().Ports).To(o.BeEmpty())
		})

		g.It("Should successfully create an endpoint matrix without custom entries", func() {
			g.By("Creating new communication matrix without static entries")
			commatrixCreator := New(
				configv1.AWSPlatformType,
				configv1.SingleReplicaTopologyMode,
				WithExporter(endpointSlices),
				WithUtilsHelpers(mockUtils),
			)
			commatrix, err := commatrixCreator.CreateEndpointMatrix()
			o.Expect(err).ToNot(o.HaveOccurred())

			g.By("Generating wanted comDetails")
			staticEntries, err := types.GetStaticEntries(configv1.AWSPlatformType, configv1.SingleReplicaTopologyMode, false, false)
			o.Expect(err).ToNot(o.HaveOccurred())
			wantedComDetails := slices.Concat(testEpsComDetails, staticEntries)
			wantedComMatrix := types.ComMatrix{Ports: wantedComDetails}
			wantedComMatrix.SortAndRemoveDuplicates()

			g.By("Generate diff between created commatrix and eanted commatrix")
			diff := matrixdiff.Generate(&wantedComMatrix, commatrix)

			g.By("Checking whether diff is empty")
			o.Expect(diff.GetUniquePrimary().Ports).To(o.BeEmpty())
			o.Expect(diff.GetUniqueSecondary().Ports).To(o.BeEmpty())
		})

		g.It("Should include IPv6 static entries when ipv6Enabled is true on Standard", func() {
			g.By("Creating communication matrix with ipv6Enabled=true for Standard")
			commatrixCreator := New(
				configv1.AWSPlatformType,
				configv1.HighlyAvailableTopologyMode,
				WithExporter(endpointSlices),
				WithUtilsHelpers(mockUtils),
				WithIPv6(),
			)
			commatrix, err := commatrixCreator.CreateEndpointMatrix()
			o.Expect(err).ToNot(o.HaveOccurred())

			g.By("Building expected details: eps + general master/worker + standard + ipv6 master/worker")
			staticEntries, err := types.GetStaticEntries(configv1.AWSPlatformType, configv1.HighlyAvailableTopologyMode, true, false)
			o.Expect(err).ToNot(o.HaveOccurred())
			wanted := slices.Concat(testEpsComDetails, staticEntries)
			wantedMatrix := types.ComMatrix{Ports: wanted}
			wantedMatrix.SortAndRemoveDuplicates()

			diff := matrixdiff.Generate(&wantedMatrix, commatrix)
			o.Expect(diff.GetUniquePrimary().Ports).To(o.BeEmpty())
			o.Expect(diff.GetUniqueSecondary().Ports).To(o.BeEmpty())
		})

		g.It("Should filter out localhost-bound ports from endpoint matrix", func() {
			g.By("Setting up fake client with localhost test resources")
			sch := runtime.NewScheme()
			err := corev1.AddToScheme(sch)
			o.Expect(err).NotTo(o.HaveOccurred())
			err = discoveryv1.AddToScheme(sch)
			o.Expect(err).NotTo(o.HaveOccurred())
			err = machineconfigurationv1.AddToScheme(sch)
			o.Expect(err).NotTo(o.HaveOccurred())
			err = configv1.AddToScheme(sch)
			o.Expect(err).NotTo(o.HaveOccurred())

			fakeClient := fake.NewClientBuilder().WithScheme(sch).WithObjects(
				testNode, testNodeWorker,
				testPodWithLocalhostPorts, testServiceLocalhost, testEndpointSliceLocalhost,
				mcpWorker, mcpMaster, testNetwork,
			).Build()
			fakeClientset := fakek.NewSimpleClientset(testNode, testNodeWorker)

			clientset := &client.ClientSet{
				Client:          fakeClient,
				CoreV1Interface: fakeClientset.CoreV1(),
			}

			localhostEndpointSlices, err := endpointslices.New(clientset, nil)
			o.Expect(err).ToNot(o.HaveOccurred())

			g.By("Creating endpoint matrix")
			commatrixCreator := New(
				configv1.AWSPlatformType,
				configv1.SingleReplicaTopologyMode,
				WithExporter(localhostEndpointSlices),
				WithUtilsHelpers(mockUtils),
			)
			commatrix, err := commatrixCreator.CreateEndpointMatrix()
			o.Expect(err).ToNot(o.HaveOccurred())

			g.By("Verifying that localhost-bound ports (8080 on 127.0.0.1, 3000 on ::1) are filtered out")
			for _, entry := range commatrix.Ports {
				if entry.Service == "localhost-service" {
					o.Expect(entry.Port).ToNot(o.Equal(8080), "Port 8080 bound to 127.0.0.1 should be filtered out")
					o.Expect(entry.Port).ToNot(o.Equal(3000), "Port 3000 bound to ::1 should be filtered out")
				}
			}

			g.By("Verifying that non-localhost port (9090 on 0.0.0.0) is present")
			foundPort9090 := false
			for _, entry := range commatrix.Ports {
				if entry.Service == "localhost-service" && entry.Port == 9090 {
					foundPort9090 = true
					break
				}
			}
			o.Expect(foundPort9090).To(o.BeTrue(), "Port 9090 bound to 0.0.0.0 should be present in the matrix")
		})

		g.It("Should not add targetPort for non-hostNetwork pod behind NodePort service", func() {
			g.By("Setting up fake client with non-hostNetwork pod behind NodePort service (no hostPort)")
			sch := runtime.NewScheme()
			err := corev1.AddToScheme(sch)
			o.Expect(err).NotTo(o.HaveOccurred())
			err = discoveryv1.AddToScheme(sch)
			o.Expect(err).NotTo(o.HaveOccurred())
			err = machineconfigurationv1.AddToScheme(sch)
			o.Expect(err).NotTo(o.HaveOccurred())
			err = configv1.AddToScheme(sch)
			o.Expect(err).NotTo(o.HaveOccurred())

			fakeClient := fake.NewClientBuilder().WithScheme(sch).WithObjects(
				testNode, testNodeWorker,
				testPodNonHostNet, testServiceNonHostNet, testEndpointSliceNonHostNet,
				mcpWorker, mcpMaster, testNetwork,
			).Build()
			fakeClientset := fakek.NewSimpleClientset(testNode, testNodeWorker)

			clientset := &client.ClientSet{
				Client:          fakeClient,
				CoreV1Interface: fakeClientset.CoreV1(),
			}

			nonHostNetEps, err := endpointslices.New(clientset, nil)
			o.Expect(err).ToNot(o.HaveOccurred())

			g.By("Creating endpoint matrix")
			commatrixCreator := New(
				configv1.AWSPlatformType,
				configv1.SingleReplicaTopologyMode,
				WithExporter(nonHostNetEps),
				WithUtilsHelpers(mockUtils),
			)
			commatrix, err := commatrixCreator.CreateEndpointMatrix()
			o.Expect(err).ToNot(o.HaveOccurred())

			g.By("Verifying that targetPort 8080 is NOT present (no hostPort, non-hostNetwork)")
			for _, entry := range commatrix.Ports {
				o.Expect(entry.Service).ToNot(o.Equal("non-hostnet-service"),
					"Non-hostNetwork pod without hostPort should not have any firewall entries")
			}
		})

		g.It("Should only add hostPort entries for non-hostNetwork pod behind NodePort service", func() {
			g.By("Setting up fake client with non-hostNetwork pod with hostPort on one port")
			sch := runtime.NewScheme()
			err := corev1.AddToScheme(sch)
			o.Expect(err).NotTo(o.HaveOccurred())
			err = discoveryv1.AddToScheme(sch)
			o.Expect(err).NotTo(o.HaveOccurred())
			err = machineconfigurationv1.AddToScheme(sch)
			o.Expect(err).NotTo(o.HaveOccurred())
			err = configv1.AddToScheme(sch)
			o.Expect(err).NotTo(o.HaveOccurred())

			fakeClient := fake.NewClientBuilder().WithScheme(sch).WithObjects(
				testNode, testNodeWorker,
				testPodNonHostNetWithHostPort, testServiceNonHostNetWithHostPort, testEndpointSliceNonHostNetWithHostPort,
				mcpWorker, mcpMaster, testNetwork,
			).Build()
			fakeClientset := fakek.NewSimpleClientset(testNode, testNodeWorker)

			clientset := &client.ClientSet{
				Client:          fakeClient,
				CoreV1Interface: fakeClientset.CoreV1(),
			}

			hostPortEps, err := endpointslices.New(clientset, nil)
			o.Expect(err).ToNot(o.HaveOccurred())

			g.By("Creating endpoint matrix")
			commatrixCreator := New(
				configv1.AWSPlatformType,
				configv1.SingleReplicaTopologyMode,
				WithExporter(hostPortEps),
				WithUtilsHelpers(mockUtils),
			)
			commatrix, err := commatrixCreator.CreateEndpointMatrix()
			o.Expect(err).ToNot(o.HaveOccurred())

			g.By("Verifying that port 8080 (with hostPort) is present")
			found8080 := false
			for _, entry := range commatrix.Ports {
				if entry.Service == "non-hostnet-hp-service" && entry.Port == 8080 {
					found8080 = true
				}
			}
			o.Expect(found8080).To(o.BeTrue(), "Port 8080 with explicit hostPort should be in the matrix")

			g.By("Verifying that port 9090 (without hostPort) is NOT present")
			for _, entry := range commatrix.Ports {
				if entry.Service == "non-hostnet-hp-service" {
					o.Expect(entry.Port).ToNot(o.Equal(9090),
						"Port 9090 without hostPort should not be in the matrix for non-hostNetwork pods")
				}
			}
		})
	})

	g.Context("expandEntriesForPools", func() {
		g.It("should fan-out role-scoped entries to all matching pools", func() {
			// Given two static entries: one for master and one for worker
			entries := []types.ComDetails{
				{
					Direction: "Ingress",
					Protocol:  consts.ProtocolTCP,
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
					Protocol:  consts.ProtocolUDP,
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

			out := ExpandStaticEntriesByPool(entries, poolToRoles)

			// Expect one entry for master in poolA, and worker duplicated for poolA and poolB and one for master poolC
			expected := []types.ComDetails{
				{
					Direction: "Ingress",
					Protocol:  consts.ProtocolTCP,
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
					Protocol:  consts.ProtocolTCP,
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
					Protocol:  consts.ProtocolUDP,
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
					Protocol:  consts.ProtocolUDP,
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
			gotMatrix := types.ComMatrix{Ports: out}
			gotMatrix.SortAndRemoveDuplicates()
			expectedMatrix := types.ComMatrix{Ports: expected}
			expectedMatrix.SortAndRemoveDuplicates()
			o.Expect(gotMatrix.Ports).To(o.Equal(expectedMatrix.Ports))
		})
	})
})
