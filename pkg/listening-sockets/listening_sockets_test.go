package listeningsockets

import (
	"fmt"
	"strings"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/openshift-kni/commatrix/pkg/client"
	"github.com/openshift-kni/commatrix/pkg/consts"
	"github.com/openshift-kni/commatrix/pkg/types"

	mock_utils "github.com/openshift-kni/commatrix/pkg/utils/mock"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	fakek "k8s.io/client-go/kubernetes/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	tcpExecCommandOutput = (`LISTEN 0      4096      127.0.0.1:8797  0.0.0.0:* users:(("machine-config-",pid=3534,fd=3))                
	LISTEN 0      4096      127.0.0.1:8798  0.0.0.0:* users:(("machine-config-",pid=3534,fd=13))               
	LISTEN 0      4096      127.0.0.1:9100  0.0.0.0:* users:(("node_exporter",pid=4147,fd=3))`)

	udpExecCommandOutput = (`UNCONN 0      0           0.0.0.0:111   0.0.0.0:* users:(("rpcbind",pid=1399,fd=5),("systemd",pid=1,fd=78))
	UNCONN 0      0         127.0.0.1:323   0.0.0.0:* users:(("chronyd",pid=1015,fd=5))                        
	UNCONN 0      0      10.46.97.104:500   0.0.0.0:* users:(("pluto",pid=2115,fd=21))`)

	procExecCommandOutput = (`1: /system.slice/crio-123abcd.scope
	2: /system.slice/other-service.scope
	
	3: /system.slice/sshd.service`)

	crictlExecCommandOut = (`{
		"containers": [
			{
				"labels": {
					"io.kubernetes.container.name": "test-container",
					"io.kubernetes.pod.name": "test-pod",
					"io.kubernetes.pod.namespace": "test-namespace"
				}
			}
		]
	}`)

	expectedTCPOutput = `node: test-node
	LISTEN 0      4096      127.0.0.1:8797  0.0.0.0:* users:(("machine-config-",pid=3534,fd=3))                
	LISTEN 0      4096      127.0.0.1:8798  0.0.0.0:* users:(("machine-config-",pid=3534,fd=13))               
	LISTEN 0      4096      127.0.0.1:9100  0.0.0.0:* users:(("node_exporter",pid=4147,fd=3))`

	expectedUDPOutput = `node: test-node
	UNCONN 0      0           0.0.0.0:111   0.0.0.0:* users:(("rpcbind",pid=1399,fd=5),("systemd",pid=1,fd=78))
	UNCONN 0      0         127.0.0.1:323   0.0.0.0:* users:(("chronyd",pid=1015,fd=5))                        
	UNCONN 0      0      10.46.97.104:500   0.0.0.0:* users:(("pluto",pid=2115,fd=21))`
)

var (
	clientset *client.ClientSet
	mockUtils *mock_utils.MockUtilsInterface
	ctrlTest  *gomock.Controller

	mockPod = &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mock-pod",
			Namespace: "mock-namespace",
		},
	}

	expectedSSMat = []types.ComDetails{
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
)

var _ = Describe("GenerateSS", func() {
	// creating the fake clients, node, pods
	BeforeEach(func() {
		sch := runtime.NewScheme()

		err := v1.AddToScheme(sch)
		Expect(err).NotTo(HaveOccurred())

		testNode := &v1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node",
				Labels: map[string]string{
					"node-role.kubernetes.io/master": "",
				},
			},
		}

		fakeClient := fake.NewClientBuilder().WithScheme(sch).WithObjects(testNode).Build()
		fakeClientset := fakek.NewSimpleClientset()

		clientset = &client.ClientSet{
			Client:          fakeClient,
			CoreV1Interface: fakeClientset.CoreV1(),
		}

		ctrlTest = gomock.NewController(GinkgoT())
		mockUtils = mock_utils.NewMockUtilsInterface(ctrlTest)
	})

	AfterEach(func() {
		ctrlTest.Finish()
	})

	It("should generate the correct ss tcp, udp output and the correct ssMatrix", func() {
		// RunCommandOnPod had more than one calling and in each call we want other output
		// Mock expectation for TCP socket check
		mockUtils.EXPECT().RunCommandOnPod(gomock.Any(),
			[]string{"/bin/sh", "-c", "ss -anpltH"}).
			Return([]byte(tcpExecCommandOutput), nil).AnyTimes()

		// Mock expectation for UDP socket check
		mockUtils.EXPECT().RunCommandOnPod(gomock.Any(),
			[]string{"/bin/sh", "-c", "ss -anpluH"}).
			Return([]byte(udpExecCommandOutput), nil).AnyTimes()

		// Mock expectation for /proc/{pid}/cgroup command
		pids := []string{"1399", "1015", "2115"}

		for _, pid := range pids {
			command := []string{"/bin/sh", "-c", fmt.Sprintf("cat /proc/%s/cgroup", pid)}
			mockUtils.EXPECT().RunCommandOnPod(gomock.Any(), command).
				Return([]byte(procExecCommandOutput), nil).
				AnyTimes()
		}

		// Mock expectation for crictl command
		mockUtils.EXPECT().RunCommandOnPod(gomock.Any(),
			[]string{"/bin/sh", "-c", "crictl ps -o json --id 123abcd"}).
			Return([]byte(crictlExecCommandOut), nil).
			AnyTimes()

		mockUtils.EXPECT().
			CreateNamespace(consts.DefaultDebugNamespace).
			Return(nil).
			AnyTimes()

		mockUtils.EXPECT().
			DeleteNamespace(consts.DefaultDebugNamespace).
			Return(nil).
			AnyTimes()

		mockUtils.EXPECT().
			CreatePodOnNode(gomock.Any(), consts.DefaultDebugNamespace, consts.DefaultDebugPodImage).
			Return(mockPod, nil).AnyTimes()

		mockUtils.EXPECT().DeletePod(mockPod).Return(nil).AnyTimes()

		connectionCheck, err := NewCheck(clientset, mockUtils, "/some/dest/dir")
		Expect(err).NotTo(HaveOccurred())

		ssMat, ssOutTCP, ssOutUDP, err := connectionCheck.GenerateSS()
		Expect(err).NotTo(HaveOccurred())

		Expect(normalizeOutput(string(ssOutTCP))).To(Equal(normalizeOutput(expectedTCPOutput)))
		Expect(normalizeOutput(string(ssOutUDP))).To(Equal(normalizeOutput(expectedUDPOutput)))
		Expect(ssMat.Matrix).To(Equal(expectedSSMat))
	})
})

// Normalize output by replacing tabs with spaces, removing extra newlines, and trimming spaces.
func normalizeOutput(s string) string {
	s = strings.ReplaceAll(s, "\t", " ")
	s = strings.ReplaceAll(s, "\n ", "\n")
	s = strings.TrimSpace(s)
	return s
}
