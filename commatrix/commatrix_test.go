package commatrix

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gomock "go.uber.org/mock/gomock"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	clientutil "github.com/openshift-kni/commatrix/client"
	"github.com/openshift-kni/commatrix/debug"
	"github.com/openshift-kni/commatrix/types"
)

func TestGetPrintFunction(t *testing.T) {
	tests := []struct {
		format         string
		expectedFnType string
		expectedErr    bool
	}{
		{"json", "func(types.ComMatrix) ([]uint8, error)", false},
		{"csv", "func(types.ComMatrix) ([]uint8, error)", false},
		{"yaml", "func(types.ComMatrix) ([]uint8, error)", false},
		{"nft", "func(types.ComMatrix) ([]uint8, error)", false},
		{"invalid", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			fn, err := getPrintFunction(tt.format)
			if tt.expectedErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, fmt.Sprintf("%T", fn), tt.expectedFnType)
			}
		})
	}
}

func TestWriteMatrixToFile(t *testing.T) {
	destDir := t.TempDir()
	matrix := types.ComMatrix{
		Matrix: []types.ComDetails{
			{NodeRole: "master", Service: "testService"},
		},
	}
	printFn := types.ToJSON
	fileName := "test-matrix"
	format := "json"

	err := writeMatrixToFile(matrix, fileName, format, printFn, destDir)
	assert.NoError(t, err)
	assert.FileExists(t, filepath.Join(destDir, "test-matrix.json"))
}

func TestGenerateSS(t *testing.T) {
	clientset := fake.NewSimpleClientset()

	ctrlTest := gomock.NewController(t)
	defer ctrlTest.Finish()

	mockDebugPod := debug.NewMockDebugPodInterface(ctrlTest)

	tcpOutput := []byte(`LISTEN 0      4096      127.0.0.1:8797  0.0.0.0:* users:(("machine-config-",pid=3534,fd=3))                
LISTEN 0      4096      127.0.0.1:8798  0.0.0.0:* users:(("machine-config-",pid=3534,fd=13))               
LISTEN 0      4096      127.0.0.1:9100  0.0.0.0:* users:(("node_exporter",pid=4147,fd=3))`)

	udpOutput := []byte(`UNCONN 0      0           0.0.0.0:111   0.0.0.0:* users:(("rpcbind",pid=1399,fd=5),("systemd",pid=1,fd=78))
UNCONN 0      0         127.0.0.1:323   0.0.0.0:* users:(("chronyd",pid=1015,fd=5))                        
UNCONN 0      0      10.46.97.104:500   0.0.0.0:* users:(("pluto",pid=2115,fd=21))`)
	Output := []byte(`1: /system.slice/containerd.service
	2: /system.slice/kubelet.service
	3: /system.slice/sshd.service`)

	// Set up the expectations
	mockDebugPod.EXPECT().ExecWithRetry(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(cmd string, interval, duration time.Duration) ([]byte, error) {
			if strings.HasPrefix(cmd, "cat /proc/") && strings.Contains(cmd, "/cgroup") {
				return Output, nil
			}
			if cmd == "ss -anpltH" {
				return tcpOutput, nil
			}
			if cmd == "ss -anpluH" {
				return udpOutput, nil
			}
			return nil, fmt.Errorf("unknown command")
		},
	).AnyTimes()

	mockDebugPod.EXPECT().Clean().Return(nil).AnyTimes()

	mockDebugPod.EXPECT().GetNodeName().Return("test-node").AnyTimes()

	originalNew := debug.NewDebugPod
	debug.NewDebugPod = func(cs *clientutil.ClientSet, node string, namespace string, image string) (debug.DebugPodInterface, error) {
		return mockDebugPod, nil
	}
	defer func() { debug.NewDebugPod = originalNew }()

	cs := &clientutil.ClientSet{
		CoreV1Interface: clientset.CoreV1(),
	}
	testNode := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
		},
	}

	_, _ = clientset.CoreV1().Nodes().Create(context.TODO(), testNode, metav1.CreateOptions{})

	ssMat, ssOutTCP, ssOutUDP, err := GenerateSS(cs)
	expectedSSMat := &types.ComMatrix{
		Matrix: []types.ComDetails{
			{Direction: "Ingress", Protocol: "UDP", Port: 111, Namespace: "", Service: "rpcbind", Pod: "", Container: "", NodeRole: "", Optional: false},
			{Direction: "Ingress", Protocol: "UDP", Port: 500, Namespace: "", Service: "pluto", Pod: "", Container: "", NodeRole: "", Optional: false},
		}}

	assert.NoError(t, err)
	assert.Equal(t, expectedSSMat, ssMat, "Expected and actual ssMat values should match")
	assert.Equal(t, "node: test-node\nLISTEN 0      4096      127.0.0.1:8797  0.0.0.0:* users:((\"machine-config-\",pid=3534,fd=3))                \nLISTEN 0      4096      127.0.0.1:8798  0.0.0.0:* users:((\"machine-config-\",pid=3534,fd=13))               \nLISTEN 0      4096      127.0.0.1:9100  0.0.0.0:* users:((\"node_exporter\",pid=4147,fd=3))\n", string(ssOutTCP))
	assert.Equal(t, "node: test-node\nUNCONN 0      0           0.0.0.0:111   0.0.0.0:* users:((\"rpcbind\",pid=1399,fd=5),(\"systemd\",pid=1,fd=78))\nUNCONN 0      0         127.0.0.1:323   0.0.0.0:* users:((\"chronyd\",pid=1015,fd=5))                        \nUNCONN 0      0      10.46.97.104:500   0.0.0.0:* users:((\"pluto\",pid=2115,fd=21))\n", string(ssOutUDP))
}
