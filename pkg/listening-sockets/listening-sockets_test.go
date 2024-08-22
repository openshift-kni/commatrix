package listeningsockets

import (
	"fmt"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/openshift-kni/commatrix/pkg/client"
	"github.com/openshift-kni/commatrix/pkg/consts"
	"github.com/openshift-kni/commatrix/pkg/types"
	mock_utils "github.com/openshift-kni/commatrix/pkg/utils/mock"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	fakek "k8s.io/client-go/kubernetes/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func normalizeOutput(s string) string {
	s = strings.ReplaceAll(s, "\t", " ")
	s = strings.ReplaceAll(s, "\n ", "\n") // Handle newlines with leading spaces
	s = strings.TrimSpace(s)
	return s
}

func TestGenerateSS(t *testing.T) {
	sch := runtime.NewScheme()

	if err := v1.AddToScheme(sch); err != nil {
		t.Fatalf("failed to add corev1 to scheme: %v", err)
	}

	testNode := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Labels: map[string]string{
				"node-role.kubernetes.io/master": "", // Label to mark this node as a master
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(sch).WithObjects(testNode).Build()

	fakeClientset := fakek.NewSimpleClientset()

	clientset := &client.ClientSet{
		Client:          fakeClient,
		CoreV1Interface: fakeClientset.CoreV1(),
	}

	ctrlTest := gomock.NewController(t)
	defer ctrlTest.Finish()

	mockUtils := mock_utils.NewMockUtilsInterface(ctrlTest)

	tcpOutput := []byte(`LISTEN 0      4096      127.0.0.1:8797  0.0.0.0:* users:(("machine-config-",pid=3534,fd=3))                
LISTEN 0      4096      127.0.0.1:8798  0.0.0.0:* users:(("machine-config-",pid=3534,fd=13))               
LISTEN 0      4096      127.0.0.1:9100  0.0.0.0:* users:(("node_exporter",pid=4147,fd=3))`)

	udpOutput := []byte(`UNCONN 0      0           0.0.0.0:111   0.0.0.0:* users:(("rpcbind",pid=1399,fd=5),("systemd",pid=1,fd=78))
UNCONN 0      0         127.0.0.1:323   0.0.0.0:* users:(("chronyd",pid=1015,fd=5))                        
UNCONN 0      0      10.46.97.104:500   0.0.0.0:* users:(("pluto",pid=2115,fd=21))`)

	output := []byte(`1: /system.slice/crio-1234567890abcdef.scope
2: /system.slice/other-service.scope
3: /system.slice/sshd.service`)

	mockUtils.EXPECT().RunCommandOnPod(gomock.Any(), gomock.Any()).DoAndReturn(
		func(pod *v1.Pod, cmd []string) ([]byte, error) {
			if len(cmd) > 2 && strings.HasPrefix(cmd[2], "crictl ps -o json --id") {
				return []byte(`{
					"containers": [
						{
							"labels": {
								"io.kubernetes.container.name": "test-container",
								"io.kubernetes.pod.name": "test-pod",
								"io.kubernetes.pod.namespace": "test-namespace"
							}
						}
					]
				}`), nil
			}
			if strings.HasPrefix(cmd[2], "cat /proc/") && strings.Contains(cmd[2], "/cgroup") {
				return output, nil
			}
			if cmd[2] == "ss -anpltH" {
				return tcpOutput, nil
			}
			if cmd[2] == "ss -anpluH" {
				return udpOutput, nil
			}
			return nil, fmt.Errorf("unknown command")
		},
	).AnyTimes()

	mockUtils.EXPECT().
		CreateNamespace(consts.DefaultDebugNamespace).
		Return(nil).
		AnyTimes()

	mockUtils.EXPECT().
		DeleteNamespace(consts.DefaultDebugNamespace).
		Return(nil).
		AnyTimes()

	mockPod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mock-pod",
			Namespace: "mock-namespace",
		},
	}

	mockUtils.EXPECT().
		CreatePodOnNode(gomock.Any(), consts.DefaultDebugNamespace, consts.DefaultDebugPodImage).
		Return(mockPod, nil).AnyTimes()

	mockUtils.EXPECT().DeletePod(mockPod).Return(nil).AnyTimes()

	connectionCheck, err := NewCheck(clientset, mockUtils, "/some/dest/dir")
	assert.NoError(t, err)

	ssMat, ssOutTCP, ssOutUDP, err := connectionCheck.GenerateSS()

	expectedTCPOutput := `node: test-node
LISTEN 0      4096      127.0.0.1:8797  0.0.0.0:* users:(("machine-config-",pid=3534,fd=3))                
LISTEN 0      4096      127.0.0.1:8798  0.0.0.0:* users:(("machine-config-",pid=3534,fd=13))               
LISTEN 0      4096      127.0.0.1:9100  0.0.0.0:* users:(("node_exporter",pid=4147,fd=3))`

	expectedUDPOutput := `node: test-node
UNCONN 0      0           0.0.0.0:111   0.0.0.0:* users:(("rpcbind",pid=1399,fd=5),("systemd",pid=1,fd=78))
UNCONN 0      0         127.0.0.1:323   0.0.0.0:* users:(("chronyd",pid=1015,fd=5))                        
UNCONN 0      0      10.46.97.104:500   0.0.0.0:* users:(("pluto",pid=2115,fd=21))`

	expectedssMat := []types.ComDetails{
		{
			Direction: "Ingress",
			Protocol:  "UDP",
			Port:      111,
			NodeRole:  "master",
			Service:   "rpcbind",
			Namespace: "",
			Pod:       "",
			Container: "test-container",
			Optional:  false,
		},
		{
			Direction: "Ingress",
			Protocol:  "UDP",
			Port:      500,
			NodeRole:  "master",
			Service:   "pluto",
			Namespace: "",
			Pod:       "",
			Container: "test-container",
			Optional:  false,
		},
	}

	assert.NoError(t, err)
	assert.NotNil(t, ssMat)
	assert.Equal(t, normalizeOutput(expectedTCPOutput), normalizeOutput(string(ssOutTCP)))
	assert.Equal(t, normalizeOutput(expectedUDPOutput), normalizeOutput(string(ssOutUDP)))
	assert.Equal(t, expectedssMat, ssMat.Matrix)

}
