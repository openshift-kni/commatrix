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

	"github.com/golang/mock/gomock"
	"github.com/openshift-kni/commatrix/pkg/client"
	"github.com/openshift-kni/commatrix/pkg/types"
	machineconfigurationv1 "github.com/openshift/api/machineconfiguration/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/runtime"

	configv1 "github.com/openshift/api/config/v1"
	fakek "k8s.io/client-go/kubernetes/fake"

	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

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
				Port:     func(i int32) *int32 { return &i }(80),
				Protocol: func(p corev1.Protocol) *corev1.Protocol { return &p }(corev1.ProtocolTCP),
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

	expectedComMatrix := types.ComMatrix{Matrix: expectedComDetails}
	expectedComMatrix.SortAndRemoveDuplicates()

	testCases := []struct {
		name         string
		args         []string
		expectedFunc func() (string, error)
		wantErr      bool
	}{
		{
			name: "Should generate CSV output",
			args: []string{"generate", "--format", "csv", "--destDir", destDir},
			expectedFunc: func() (string, error) {
				expectedOutput, err := expectedComMatrix.ToCSV()
				return string(expectedOutput), err
			},
			wantErr: false,
		},
		{
			name: "Should generate JSON output",
			args: []string{"generate", "--format", "json", "--destDir", destDir},
			expectedFunc: func() (string, error) {
				expectedOutput, err := expectedComMatrix.ToJSON()
				return string(expectedOutput), err
			},
			wantErr: false,
		},
		{
			name: "Should Return failure on format validation",
			args: []string{"generate", "--format", "test"},
			expectedFunc: func() (string, error) {
				return "", fmt.Errorf("invalid format 'test', valid options are: csv, json, yaml, nft")
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
