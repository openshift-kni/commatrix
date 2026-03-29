package generate

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/cli-runtime/pkg/genericiooptions"

	"github.com/openshift-kni/commatrix/pkg/client"
	"github.com/openshift-kni/commatrix/pkg/consts"
	"github.com/openshift-kni/commatrix/pkg/types"
	mock_utils "github.com/openshift-kni/commatrix/pkg/utils/mock"
	machineconfigurationv1 "github.com/openshift/api/machineconfiguration/v1"
	"go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"

	configv1 "github.com/openshift/api/config/v1"
	fakek "k8s.io/client-go/kubernetes/fake"

	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func mustParseLabels(s string) labels.Selector {
	l, err := labels.Parse(s)
	if err != nil {
		panic(err)
	}
	return l
}

var (
	destDir = "communication-matrix-test"

	infra = &configv1.Infrastructure{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
		},
		Status: configv1.InfrastructureStatus{
			ControlPlaneTopology: configv1.HighlyAvailableTopologyMode,
			PlatformStatus: &configv1.PlatformStatus{
				Type: configv1.AWSPlatformType,
			},
		},
	}

	network = &configv1.Network{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
		},
		Spec: configv1.NetworkSpec{
			ClusterNetwork: []configv1.ClusterNetworkEntry{{CIDR: "10.0.0.0/16"}},
		},
	}

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
				Port:     ptr.To(int32(80)),
				Protocol: ptr.To(corev1.ProtocolTCP),
			},
		},
	}

	testEpsComDetails = []types.ComDetails{
		{
			Direction: "Ingress",
			Protocol:  "TCP",
			Port:      80,
			Namespace: "test-ns",
			Service:   "test-service",
			Pod:       "test-app-pod",
			Container: "test-container",
			NodeGroup: "master",
			Optional:  false,
		},
	}
)

func TestCommatrixGeneration(t *testing.T) {
	expectedComDetails := slices.Concat(testEpsComDetails, types.GeneralStaticEntriesMaster, types.GeneralStaticEntriesWorker, types.StandardStaticEntries)

	expectedComMatrix := types.ComMatrix{Ports: expectedComDetails}
	expectedComMatrix.SortAndRemoveDuplicates()

	testCases := []struct {
		name         string
		args         []string
		expectedFunc func() (string, error)
		wantErr      bool
	}{
		{
			name: "Should Return failure on format validation",
			args: []string{"generate", "--format", "test"},
			expectedFunc: func() (string, error) {
				return "", fmt.Errorf("invalid format 'test', valid options are: csv, json, yaml, nft, butane, mc")
			},
			wantErr: true,
		},
		{
			name: "Should return failure when customEntriesPath is set but customEntriesFormat is missing",
			args: []string{"generate", "--customEntriesPath", "/path/to/customEntriesFile"},
			expectedFunc: func() (string, error) {
				return "", fmt.Errorf("you must specify the --customEntriesFormat when using --customEntriesPath")
			},
			wantErr: true,
		},
		{
			name: "Should return failure when customEntriesFormat is set but customEntriesPath is missing",
			args: []string{"generate", "--customEntriesFormat", "nft"},
			expectedFunc: func() (string, error) {
				return "", fmt.Errorf("you must specify the --customEntriesPath when using --customEntriesFormat")
			},
			wantErr: true,
		},
		{
			name: "Should return failure when customEntriesFormat not valid",
			args: []string{"generate", "--customEntriesPath", "/path/to/customEntriesFile", "--customEntriesFormat", "invalid"},
			expectedFunc: func() (string, error) {
				return "", fmt.Errorf("invalid custom entries format 'invalid', valid options are: csv, json, yaml")
			},
			wantErr: true,
		},
	}
	sch := runtime.NewScheme()
	t.Helper()

	err := corev1.AddToScheme(sch)
	require.NoError(t, err)

	err = discoveryv1.AddToScheme(sch)
	require.NoError(t, err)

	err = configv1.AddToScheme(sch)
	require.NoError(t, err)

	err = machineconfigurationv1.AddToScheme(sch)
	require.NoError(t, err)

	fakeClient := fake.NewClientBuilder().WithScheme(sch).WithObjects(infra, network, testNode, testNodeWorker, testPod, testService, testEndpointSlice, mcpWorker, mcpMaster).Build()
	fakeClientset := fakek.NewSimpleClientset()

	clientset := &client.ClientSet{
		Client:          fakeClient,
		CoreV1Interface: fakeClientset.CoreV1(),
	}

	// Ensure the directory exists before running the test
	err = os.MkdirAll(destDir, 0755)
	require.NoError(t, err)

	// Clean up after test
	defer os.RemoveAll(destDir)

	ctrlTest := gomock.NewController(t)

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			streams, _, _, errOut := genericiooptions.NewTestIOStreams()
			cmdTest := NewCmd(clientset, streams)
			cmdTest.SetArgs(tt.args)
			err = cmdTest.Execute()

			expectedOutput, expectedErr := tt.expectedFunc()
			if tt.wantErr {
				assert.Contains(t, errOut.String(), expectedOutput)
				require.Error(t, err)
				require.Error(t, expectedErr)
				require.Equal(t, expectedErr, err) // errors need to be equal, expectedErr==err
			} else {
				require.NoError(t, err)
				require.NoError(t, expectedErr)

				// Get the generated file path
				var fileName string
				if tt.args[2] == "csv" {
					fileName = "communication-matrix.csv"
				} else {
					fileName = "communication-matrix.json"
				}
				filePath := filepath.Join(destDir, fileName)

				// Read the actual file contents
				actualOutputBytes, readErr := os.ReadFile(filePath)
				require.NoError(t, readErr)

				// Convert file content to string
				actualOutput := string(actualOutputBytes)

				// Compare actual file content with expected output
				assert.Equal(t, expectedOutput, actualOutput)
			}
		})
	}

	ctrlTest.Finish()
}

func TestParseCustomNodeGroups(t *testing.T) {
	testCases := []struct {
		name     string
		raw      []string
		expected map[string]labels.Selector
		wantErr  string
	}{
		{
			name:     "nil input returns nil",
			raw:      nil,
			expected: nil,
		},
		{
			name:     "empty input returns nil",
			raw:      []string{},
			expected: nil,
		},
		{
			name: "single group with label existence selector",
			raw:  []string{"mc-special=node-role.kubernetes.io/special"},
			expected: map[string]labels.Selector{
				"mc-special": mustParseLabels("node-role.kubernetes.io/special"),
			},
		},
		{
			name: "single group with equality selector",
			raw:  []string{"mc-ingress=node-role.kubernetes.io/ingress=true"},
			expected: map[string]labels.Selector{
				"mc-ingress": mustParseLabels("node-role.kubernetes.io/ingress=true"),
			},
		},
		{
			name: "multiple groups via repeated flag",
			raw:  []string{"mc-ingress=node-role.kubernetes.io/ingress", "mc-storage=node-role.kubernetes.io/storage"},
			expected: map[string]labels.Selector{
				"mc-ingress": mustParseLabels("node-role.kubernetes.io/ingress"),
				"mc-storage": mustParseLabels("node-role.kubernetes.io/storage"),
			},
		},
		{
			name:    "missing equals sign",
			raw:     []string{"mc-special"},
			wantErr: "expected format groupName=labelSelector",
		},
		{
			name:    "empty group name",
			raw:     []string{"=node-role.kubernetes.io/ingress"},
			wantErr: "expected format groupName=labelSelector",
		},
		{
			name:    "empty selector",
			raw:     []string{"mc-special="},
			wantErr: "expected format groupName=labelSelector",
		},
		{
			name:    "duplicate group name",
			raw:     []string{"mc-ingress=node-role.kubernetes.io/ingress", "mc-ingress=node-role.kubernetes.io/storage"},
			wantErr: "duplicate --custom-node-group group name",
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseCustomNodeGroups(tt.raw)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				require.NoError(t, err)
				if tt.expected == nil {
					assert.Nil(t, result)
				} else {
					require.Equal(t, len(tt.expected), len(result))
					for k, expectedSel := range tt.expected {
						assert.Equal(t, expectedSel.String(), result[k].String())
					}
				}
			}
		})
	}
}

func TestSpecialNodesButaneOutput(t *testing.T) {
	sch := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(sch))
	require.NoError(t, discoveryv1.AddToScheme(sch))
	require.NoError(t, configv1.AddToScheme(sch))
	require.NoError(t, machineconfigurationv1.AddToScheme(sch))

	masterNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "master-1",
			Annotations: map[string]string{"machineconfiguration.openshift.io/currentConfig": "rendered-master-abc"},
			Labels:      map[string]string{"node-role.kubernetes.io/master": ""},
		},
	}
	workerNode1 := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "worker-1",
			Annotations: map[string]string{"machineconfiguration.openshift.io/currentConfig": "rendered-worker-def"},
			Labels:      map[string]string{"node-role.kubernetes.io/worker": ""},
		},
	}
	workerNode2 := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "worker-2",
			Annotations: map[string]string{"machineconfiguration.openshift.io/currentConfig": "rendered-worker-ghi"},
			Labels: map[string]string{
				"node-role.kubernetes.io/worker": "",
				"custom-group":                   "mc-special",
			},
		},
	}

	// Service running on BOTH workers (port 8080)
	commonPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "common-pod", Namespace: "ns1",
			Labels:          map[string]string{"app": "common"},
			OwnerReferences: []metav1.OwnerReference{{Kind: "DaemonSet", Name: "common-ds"}},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Name: "common-ctr", Image: "img",
				Ports: []corev1.ContainerPort{{ContainerPort: 8080}},
			}},
		},
	}
	commonSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "common-svc", Namespace: "ns1"},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": "common"},
			Ports:    []corev1.ServicePort{{Port: 8080, Protocol: corev1.ProtocolTCP}},
			Type:     corev1.ServiceTypeNodePort,
		},
	}
	commonEPS := &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name: "common-svc-eps", Namespace: "ns1",
			Labels:          map[string]string{"kubernetes.io/service-name": "common-svc"},
			OwnerReferences: []metav1.OwnerReference{{Kind: "Service", Name: "common-svc"}},
		},
		AddressType: discoveryv1.AddressTypeIPv4,
		Endpoints: []discoveryv1.Endpoint{
			{NodeName: &workerNode1.Name, Addresses: []string{"10.0.0.1"}},
			{NodeName: &workerNode2.Name, Addresses: []string{"10.0.0.2"}},
		},
		Ports: []discoveryv1.EndpointPort{{
			Port:     ptr.To(int32(8080)),
			Protocol: ptr.To(corev1.ProtocolTCP),
		}},
	}

	// Service running ONLY on worker-2 (port 443)
	specialPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "ingress-pod", Namespace: "ns2",
			Labels:          map[string]string{"app": "ingress"},
			OwnerReferences: []metav1.OwnerReference{{Kind: "DaemonSet", Name: "ingress-ds"}},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Name: "ingress-ctr", Image: "img",
				Ports: []corev1.ContainerPort{{ContainerPort: 443}},
			}},
		},
	}
	specialSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "ingress-svc", Namespace: "ns2"},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": "ingress"},
			Ports:    []corev1.ServicePort{{Port: 443, Protocol: corev1.ProtocolTCP}},
			Type:     corev1.ServiceTypeNodePort,
		},
	}
	specialEPS := &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name: "ingress-svc-eps", Namespace: "ns2",
			Labels:          map[string]string{"kubernetes.io/service-name": "ingress-svc"},
			OwnerReferences: []metav1.OwnerReference{{Kind: "Service", Name: "ingress-svc"}},
		},
		AddressType: discoveryv1.AddressTypeIPv4,
		Endpoints: []discoveryv1.Endpoint{
			{NodeName: &workerNode2.Name, Addresses: []string{"10.0.0.2"}},
		},
		Ports: []discoveryv1.EndpointPort{{
			Port:     ptr.To(int32(443)),
			Protocol: ptr.To(corev1.ProtocolTCP),
		}},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(sch).WithObjects(
		infra, network, mcpMaster, mcpWorker,
		masterNode, workerNode1, workerNode2,
		commonPod, commonSvc, commonEPS,
		specialPod, specialSvc, specialEPS,
	).Build()
	fakeClientset := fakek.NewSimpleClientset(masterNode, workerNode1, workerNode2)

	cs := &client.ClientSet{Client: fakeClient, CoreV1Interface: fakeClientset.CoreV1()}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockUtils := mock_utils.NewMockUtilsInterface(ctrl)
	mockUtils.EXPECT().ListNodes().Return([]corev1.Node{*masterNode, *workerNode1, *workerNode2}, nil).AnyTimes()
	mockUtils.EXPECT().GetControlPlaneTopology().Return(configv1.HighlyAvailableTopologyMode, nil).AnyTimes()
	mockUtils.EXPECT().GetPlatformType().Return(configv1.AWSPlatformType, nil).AnyTimes()
	mockUtils.EXPECT().IsIPv6Enabled().Return(false, nil).AnyTimes()
	mockUtils.EXPECT().GetClusterVersion().Return("4.17", nil).AnyTimes()

	mockPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "debug-pod", Namespace: consts.DefaultDebugNamespace},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning},
	}
	mockUtils.EXPECT().CreateNamespace(consts.DefaultDebugNamespace).Return(nil).AnyTimes()
	mockUtils.EXPECT().DeleteNamespace(consts.DefaultDebugNamespace).Return(nil).AnyTimes()
	mockUtils.EXPECT().CreatePodOnNode(gomock.Any(), consts.DefaultDebugNamespace, gomock.Any(), gomock.Any()).Return(mockPod, nil).AnyTimes()
	mockUtils.EXPECT().DeletePod(mockPod).Return(nil).AnyTimes()
	mockUtils.EXPECT().WaitForPodStatus(consts.DefaultDebugNamespace, mockPod, corev1.PodRunning).Return(nil).AnyTimes()
	mockUtils.EXPECT().RunCommandOnPod(mockPod, gomock.Any()).Return([]byte("32768 60999\n"), nil).AnyTimes()

	writtenFiles := map[string][]byte{}
	mockUtils.EXPECT().WriteFile(gomock.Any(), gomock.Any()).DoAndReturn(
		func(path string, data []byte) error {
			writtenFiles[filepath.Base(path)] = data
			return nil
		},
	).AnyTimes()

	dir := t.TempDir()
	opts := &GenerateOptions{
		destDir:          dir,
		format:           "butane",
		cs:               cs,
		utilsHelpers:     mockUtils,
		customNodeGroups: map[string]labels.Selector{"mc-special": mustParseLabels("custom-group=mc-special")},
	}

	err := Run(opts)
	require.NoError(t, err)

	// Should produce 3 butane files + node disruption policy
	_, hasMaster := writtenFiles["butane-master.yaml"]
	assert.True(t, hasMaster, "expected butane-master.yaml to be written")

	// worker group (worker-1 only): has port 8080, does NOT have 443
	workerContent, ok := writtenFiles["butane-worker.yaml"]
	require.True(t, ok, "expected butane-worker.yaml to be written")
	assert.Contains(t, string(workerContent), "8080",
		"worker butane should contain port 8080 (common service)")
	assert.NotContains(t, string(workerContent), "443",
		"worker butane should NOT contain port 443 (only on special node)")
	assert.Contains(t, string(workerContent), "machineconfiguration.openshift.io/role: worker")
	// Static entries should be present
	assert.Contains(t, string(workerContent), "22",
		"worker butane should contain static SSH port 22")

	// mc-special group (worker-2 only): has BOTH 8080 and 443
	specialContent, ok := writtenFiles["butane-mc-special.yaml"]
	require.True(t, ok, "expected butane-mc-special.yaml to be written")
	assert.Contains(t, string(specialContent), "8080",
		"mc-special butane should contain port 8080 (common service)")
	assert.Contains(t, string(specialContent), "443",
		"mc-special butane should contain port 443 (special service)")
	assert.Contains(t, string(specialContent), "machineconfiguration.openshift.io/role: mc-special")
	// Static entries should be present
	assert.Contains(t, string(specialContent), "22",
		"mc-special butane should contain static SSH port 22")

	// Node disruption policy
	_, ok = writtenFiles["node-disruption-policy.yaml"]
	assert.True(t, ok, "expected node-disruption-policy.yaml to be written")
}

func TestValidateAcceptsButaneAndMCFormats(t *testing.T) {
	testCases := []struct {
		name    string
		format  string
		wantErr bool
	}{
		{name: "butane format is valid", format: "butane", wantErr: false},
		{name: "mc format is valid", format: "mc", wantErr: false},
		{name: "nft format is valid", format: "nft", wantErr: false},
		{name: "csv format is valid", format: "csv", wantErr: false},
		{name: "invalid format is rejected", format: "invalid", wantErr: true},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			o := &GenerateOptions{format: tt.format}
			err := Validate(o)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "invalid format")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestButaneAndMCOutputFiles(t *testing.T) {
	sch := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(sch))
	require.NoError(t, discoveryv1.AddToScheme(sch))
	require.NoError(t, configv1.AddToScheme(sch))
	require.NoError(t, machineconfigurationv1.AddToScheme(sch))

	fakeClient := fake.NewClientBuilder().WithScheme(sch).WithObjects(
		infra, network, testNode, testNodeWorker, testPod, testService, testEndpointSlice, mcpWorker, mcpMaster,
	).Build()
	fakeClientset := fakek.NewSimpleClientset(testNode, testNodeWorker)

	clientset := &client.ClientSet{
		Client:          fakeClient,
		CoreV1Interface: fakeClientset.CoreV1(),
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockUtils := mock_utils.NewMockUtilsInterface(ctrl)
	mockUtils.EXPECT().ListNodes().Return([]corev1.Node{*testNode, *testNodeWorker}, nil).AnyTimes()
	mockUtils.EXPECT().GetControlPlaneTopology().Return(configv1.HighlyAvailableTopologyMode, nil).AnyTimes()
	mockUtils.EXPECT().GetPlatformType().Return(configv1.AWSPlatformType, nil).AnyTimes()
	mockUtils.EXPECT().IsIPv6Enabled().Return(false, nil).AnyTimes()
	mockUtils.EXPECT().GetClusterVersion().Return("4.17", nil).AnyTimes()

	mockPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "debug-pod", Namespace: consts.DefaultDebugNamespace},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning},
	}
	mockUtils.EXPECT().CreateNamespace(consts.DefaultDebugNamespace).Return(nil).AnyTimes()
	mockUtils.EXPECT().DeleteNamespace(consts.DefaultDebugNamespace).Return(nil).AnyTimes()
	mockUtils.EXPECT().CreatePodOnNode(gomock.Any(), consts.DefaultDebugNamespace, gomock.Any(), gomock.Any()).Return(mockPod, nil).AnyTimes()
	mockUtils.EXPECT().DeletePod(mockPod).Return(nil).AnyTimes()
	mockUtils.EXPECT().WaitForPodStatus(consts.DefaultDebugNamespace, mockPod, corev1.PodRunning).Return(nil).AnyTimes()
	mockUtils.EXPECT().RunCommandOnPod(
		mockPod,
		[]string{"chroot", "/host", "/bin/sh", "-c", "ip -j addr show lo"},
	).Return([]byte(""), nil).AnyTimes()
	mockUtils.EXPECT().RunCommandOnPod(
		mockPod,
		[]string{"/bin/sh", "-c", "cat /host/proc/sys/net/ipv4/ip_local_port_range"},
	).Return([]byte("32768 60999\n"), nil).AnyTimes()
	mockUtils.EXPECT().RunCommandOnPod(mockPod, []string{"/bin/sh", "-c", "ss -anpltH"}).Return([]byte(
		"LISTEN 0 4096 0.0.0.0:5355 0.0.0.0:* \n",
	), nil).AnyTimes()
	mockUtils.EXPECT().RunCommandOnPod(mockPod, []string{"/bin/sh", "-c", "ss -anpluH"}).Return([]byte(
		"LISTEN 0 4096 0.0.0.0:5356 0.0.0.0:* \n",
	), nil).AnyTimes()

	// Capture all written files
	writtenFiles := map[string][]byte{}
	mockUtils.EXPECT().WriteFile(gomock.Any(), gomock.Any()).DoAndReturn(
		func(path string, data []byte) error {
			writtenFiles[filepath.Base(path)] = data
			return nil
		},
	).AnyTimes()

	testCases := []struct {
		name           string
		format         string
		openPorts      bool
		expectedFiles  []string
		expectedInFile map[string][]string
	}{
		{
			name:   "butane format produces per-pool butane files and node disruption policy",
			format: "butane",
			expectedFiles: []string{
				"butane-master.yaml",
				"node-disruption-policy.yaml",
			},
			expectedInFile: map[string][]string{
				"butane-master.yaml": {
					"variant: openshift",
					"name: 98-nftables-commatrix-master",
					"machineconfiguration.openshift.io/role: master",
					"nftables.service",
					"/etc/sysconfig/nftables.conf",
					"tcp dport { 22, 80, 111, 9107, 10250, 10256, 30000-60999 } accept",
					"udp dport { 111, 6081, 30000-60999 } accept",
				},
				"node-disruption-policy.yaml": {
					"oc patch machineconfiguration cluster",
					"nodeDisruptionPolicy",
					"nftables.service",
				},
			},
		},
		{
			name:      "butane format produces per-pool butane files and node disruption policy with openPorts",
			format:    "butane",
			openPorts: true,
			expectedFiles: []string{
				"butane-master.yaml",
				"node-disruption-policy.yaml",
			},
			expectedInFile: map[string][]string{
				"butane-master.yaml": {
					"variant: openshift",
					"name: 98-nftables-commatrix-master",
					"machineconfiguration.openshift.io/role: master",
					"nftables.service",
					"/etc/sysconfig/nftables.conf",
					"tcp dport { 22, 80, 111, 5355, 9107, 10250, 10256, 30000-60999 } accept",
					"udp dport { 111, 5356, 6081, 30000-60999 } accept",
				},
				"node-disruption-policy.yaml": {
					"oc patch machineconfiguration cluster",
					"nodeDisruptionPolicy",
					"nftables.service",
				},
			},
		},
		{
			name:   "mc format produces per-pool MachineConfig files and node disruption policy",
			format: "mc",
			expectedFiles: []string{
				"mc-master.yaml",
				"node-disruption-policy.yaml",
			},
			expectedInFile: map[string][]string{
				"mc-master.yaml": {
					"kind: MachineConfig",
					"name: 98-nftables-commatrix-master",
					"machineconfiguration.openshift.io/role: master",
				},
				"node-disruption-policy.yaml": {
					"oc patch machineconfiguration cluster",
					"nodeDisruptionPolicy",
				},
			},
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			// Clear written files from previous run
			for k := range writtenFiles {
				delete(writtenFiles, k)
			}

			dir := t.TempDir()
			opts := &GenerateOptions{
				destDir:      dir,
				format:       tt.format,
				cs:           clientset,
				utilsHelpers: mockUtils,
				openPorts:    tt.openPorts,
			}

			err := Run(opts)
			require.NoError(t, err)

			for _, fileName := range tt.expectedFiles {
				content, ok := writtenFiles[fileName]
				assert.True(t, ok, "expected file %s to be written", fileName)
				assert.NotEmpty(t, content, "expected file %s to have content", fileName)

				for _, expected := range tt.expectedInFile[fileName] {
					assert.Contains(t, string(content), expected,
						"file %s should contain %q", fileName, expected)
				}
			}
		})
	}
}
