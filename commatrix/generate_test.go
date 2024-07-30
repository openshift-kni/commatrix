package commatrix

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	clientutil "github.com/openshift-kni/commatrix/client"
	"github.com/openshift-kni/commatrix/debug"
	"github.com/openshift-kni/commatrix/types"
)

func TestGenerateSS(t *testing.T) {
	clientset := fake.NewSimpleClientset()

	mockDebugPod := new(debug.MockDebugPodInterface)
	tcpOutput := []byte(`LISTEN 0      4096      127.0.0.1:8797  0.0.0.0:* users:(("machine-config-",pid=3534,fd=3))                
LISTEN 0      4096      127.0.0.1:8798  0.0.0.0:* users:(("machine-config-",pid=3534,fd=13))               
LISTEN 0      4096      127.0.0.1:9100  0.0.0.0:* users:(("node_exporter",pid=4147,fd=3))`)

	udpOutput := []byte(`UNCONN 0      0           0.0.0.0:111   0.0.0.0:* users:(("rpcbind",pid=1399,fd=5),("systemd",pid=1,fd=78))
UNCONN 0      0         127.0.0.1:323   0.0.0.0:* users:(("chronyd",pid=1015,fd=5))                        
UNCONN 0      0      10.46.97.104:500   0.0.0.0:* users:(("pluto",pid=2115,fd=21))`)
	Output := []byte(`1: /system.slice/containerd.service
2: /system.slice/kubelet.service
3: /system.slice/sshd.service`)
	mockDebugPod.On("ExecWithRetry", mock.MatchedBy(func(cmd string) bool {
		return strings.HasPrefix(cmd, "cat /proc/") && strings.Contains(cmd, "/cgroup")
	}), mock.Anything, mock.Anything).Return(
		Output, nil,
	)

	mockDebugPod.On("ExecWithRetry", "ss -anpltH", mock.Anything, mock.Anything).Return(
		tcpOutput, nil,
	)
	mockDebugPod.On("ExecWithRetry", "ss -anpluH", mock.Anything, mock.Anything).Return(
		udpOutput, nil,
	)
	mockDebugPod.On("Clean").Return(nil)
	mockDebugPod.On("GetNodeName").Return("test-node")

	// Mock the New function
	debug.New = func(cs *clientutil.ClientSet, node string, namespace string, image string) (debug.DebugPodInterface, error) {
		return mockDebugPod, nil
	}

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
