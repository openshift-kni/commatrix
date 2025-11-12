package listeningsockets

import (
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"

	"github.com/openshift-kni/commatrix/pkg/client"
	"github.com/openshift-kni/commatrix/pkg/consts"
	"github.com/openshift-kni/commatrix/pkg/types"

	mock_utils "github.com/openshift-kni/commatrix/pkg/utils/mock"
	machineconfigurationv1 "github.com/openshift/api/machineconfiguration/v1"
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
			NodeGroup: "master",
			Service:   "rpcbind",
			Namespace: "test-namespace",
			Pod:       "test-pod",
			Container: "test-container",
			Optional:  false,
		},
		{
			Direction: "Ingress",
			Protocol:  "UDP",
			Port:      500,
			NodeGroup: "master",
			Service:   "pluto",
			Namespace: "test-namespace",
			Pod:       "test-pod",
			Container: "test-container",
			Optional:  false,
		},
	}

	pids = []string{"1399", "2115"}
)

var _ = Describe("GenerateSS", func() {
	// creating the fake clients, node, pods
	BeforeEach(func() {
		sch := runtime.NewScheme()

		err := v1.AddToScheme(sch)
		Expect(err).NotTo(HaveOccurred())
		err = machineconfigurationv1.AddToScheme(sch)
		Expect(err).NotTo(HaveOccurred())

		testNode := &v1.Node{
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

		mcpMaster := &machineconfigurationv1.MachineConfigPool{
			ObjectMeta: metav1.ObjectMeta{Name: "master"},
			Spec: machineconfigurationv1.MachineConfigPoolSpec{
				NodeSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"node-role.kubernetes.io/master": ""}},
			},
			Status: machineconfigurationv1.MachineConfigPoolStatus{MachineCount: 1},
		}
		fakeClient := fake.NewClientBuilder().WithScheme(sch).WithObjects(testNode, mcpMaster).Build()
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
		for _, pid := range pids {
			command := []string{"/bin/sh", "-c", fmt.Sprintf("cat /proc/%s/cgroup", pid)}
			mockUtils.EXPECT().RunCommandOnPod(gomock.Any(), command).
				Return([]byte(procExecCommandOutput), nil).
				AnyTimes()
		}

		// Mock expectation for crictl command
		mockUtils.EXPECT().RunCommandOnPod(gomock.Any(),
			[]string{"chroot", "/host", "/bin/sh", "-c", "crictl ps -o json --id 123abcd"}).
			Return([]byte(crictlExecCommandOut), nil).
			AnyTimes()

		// Mock expectation for loopback IP discovery on host
		mockUtils.EXPECT().RunCommandOnPod(gomock.Any(),
			[]string{"chroot", "/host", "/bin/sh", "-c", "ip -j addr show lo"}).
			Return([]byte(`[{"addr_info":[{"local":"127.0.0.1"},{"local":"::1"}]}]`), nil).
			AnyTimes()

		mockUtils.EXPECT().
			CreatePodOnNode(gomock.Any(), consts.DefaultDebugNamespace, consts.DefaultDebugPodImage, []string{}).
			Return(mockPod, nil).AnyTimes()

		mockUtils.EXPECT().WaitForPodStatus(consts.DefaultDebugNamespace, mockPod, v1.PodRunning).
			Return(nil).AnyTimes()

		mockUtils.EXPECT().DeletePod(mockPod).Return(nil).AnyTimes()

		connectionCheck, err := NewCheck(clientset, mockUtils, "/some/dest/dir")
		Expect(err).NotTo(HaveOccurred())

		ssMat, ssOutTCP, ssOutUDP, err := connectionCheck.GenerateSS(consts.DefaultDebugNamespace)
		Expect(err).NotTo(HaveOccurred())

		Expect(normalizeOutput(string(ssOutTCP))).To(Equal(normalizeOutput(expectedTCPOutput)))
		Expect(normalizeOutput(string(ssOutUDP))).To(Equal(normalizeOutput(expectedUDPOutput)))
		Expect(ssMat.Matrix).To(Equal(expectedSSMat))
	})
})

var _ = Describe("isLoopbackEntry", func() {
	It("should correctly identify IPv4 loopback variants", func() {
		empty := map[string]bool{}
		Expect(isLoopbackEntry(`LISTEN 0 4096 127.0.0.1:8797 0.0.0.0:* users:(("svc",pid=1,fd=3))`, empty)).To(BeTrue())
		Expect(isLoopbackEntry(`LISTEN 0 4096 127.0.0.2:8797 0.0.0.0:* users:(("svc",pid=1,fd=3))`, empty)).To(BeTrue())
		Expect(isLoopbackEntry(`LISTEN 0 4096 127.1.2.3:8797 0.0.0.0:* users:(("svc",pid=1,fd=3))`, empty)).To(BeTrue())
		Expect(isLoopbackEntry(`LISTEN 0 4096 127.255.255.255:8797 0.0.0.0:* users:(("svc",pid=1,fd=3))`, empty)).To(BeTrue())
	})

	It("should correctly identify IPv6 loopback variants", func() {
		empty := map[string]bool{}
		Expect(isLoopbackEntry(`LISTEN 0 4096 ::1:8797 :::* users:(("svc",pid=1,fd=3))`, empty)).To(BeTrue())
		Expect(isLoopbackEntry(`LISTEN 0 4096 [::1]:8797 [::]:* users:(("svc",pid=1,fd=3))`, empty)).To(BeTrue())
	})

	It("should not flag non-loopback addresses", func() {
		empty := map[string]bool{}
		Expect(isLoopbackEntry(`LISTEN 0 4096 0.0.0.0:8797 0.0.0.0:* users:(("svc",pid=1,fd=3))`, empty)).To(BeFalse())
		Expect(isLoopbackEntry(`UNCONN 0 0 10.46.97.104:500 0.0.0.0:* users:(("svc",pid=1,fd=3))`, empty)).To(BeFalse())
		Expect(isLoopbackEntry(`LISTEN 0 4096 192.168.1.1:8797 0.0.0.0:* users:(("svc",pid=1,fd=3))`, empty)).To(BeFalse())
		Expect(isLoopbackEntry(`LISTEN 0 4096 fe80::1:8797 :::* users:(("svc",pid=1,fd=3))`, empty)).To(BeFalse())
		Expect(isLoopbackEntry(``, empty)).To(BeFalse())
	})

	It("should detect IPs assigned to loopback interface (alias)", func() {
		aliases := map[string]bool{
			"127.0.0.1":  true,
			"::1":        true,
			"172.20.0.1": true,
		}
		Expect(isLoopbackEntry(`LISTEN 0 4096 127.0.0.1:8797 0.0.0.0:* users:(("svc",pid=1,fd=3))`, aliases)).To(BeTrue())
		Expect(isLoopbackEntry(`LISTEN 0 4096 172.20.0.1:6443 0.0.0.0:* users:(("svc",pid=1,fd=3))`, aliases)).To(BeTrue())
		Expect(isLoopbackEntry(`LISTEN 0 4096 192.168.1.1:8797 0.0.0.0:* users:(("svc",pid=1,fd=3))`, aliases)).To(BeFalse())
	})
})

var _ = Describe("filterEntries (loopback filtering)", func() {
	It("should filter out loopback addresses from ss entries", func() {
		entries := []string{
			`LISTEN 0 4096 127.0.0.1:8797 0.0.0.0:* users:(("service1",pid=1234,fd=3))`,
			`LISTEN 0 4096 127.1.2.3:8798 0.0.0.0:* users:(("service2",pid=1235,fd=3))`,
			`LISTEN 0 4096 0.0.0.0:9100 0.0.0.0:* users:(("service3",pid=1236,fd=3))`,
			`UNCONN 0 0 10.46.97.104:500 0.0.0.0:* users:(("service4",pid=1237,fd=3))`,
			`LISTEN 0 4096 ::1:8800 :::* users:(("service5",pid=1238,fd=3))`,
			`LISTEN 0 4096 192.168.1.1:8801 0.0.0.0:* users:(("service6",pid=1239,fd=3))`,
			``,
		}
		empty := map[string]bool{}
		filtered := filterEntries(entries, empty)

		Expect(filtered).To(HaveLen(3))
		Expect(filtered).To(ContainElement(ContainSubstring("0.0.0.0:9100")))
		Expect(filtered).To(ContainElement(ContainSubstring("10.46.97.104:500")))
		Expect(filtered).To(ContainElement(ContainSubstring("192.168.1.1:8801")))

		Expect(filtered).NotTo(ContainElement(ContainSubstring("127.0.0.1")))
		Expect(filtered).NotTo(ContainElement(ContainSubstring("127.1.2.3")))
		Expect(filtered).NotTo(ContainElement(ContainSubstring("::1")))
	})

	It("should filter out loopback aliases (e.g., HyperShift 172.20.0.1)", func() {
		loopbacks := map[string]bool{
			"127.0.0.1":  true,
			"::1":        true,
			"172.20.0.1": true,
		}
		entries := []string{
			`LISTEN 0 4096 172.20.0.1:6443 0.0.0.0:* users:(("kube-apiserver",pid=1234,fd=3))`,
			`LISTEN 0 4096 127.0.0.1:8797 0.0.0.0:* users:(("service1",pid=1235,fd=3))`,
			`LISTEN 0 4096 0.0.0.0:443 0.0.0.0:* users:(("service2",pid=1236,fd=3))`,
			`LISTEN 0 4096 192.168.1.1:8801 0.0.0.0:* users:(("service3",pid=1237,fd=3))`,
		}
		filtered := filterEntries(entries, loopbacks)
		Expect(filtered).To(HaveLen(2))
		Expect(filtered).To(ContainElement(ContainSubstring("0.0.0.0:443")))
		Expect(filtered).To(ContainElement(ContainSubstring("192.168.1.1:8801")))
		Expect(filtered).NotTo(ContainElement(ContainSubstring("172.20.0.1:6443")))
		Expect(filtered).NotTo(ContainElement(ContainSubstring("127.0.0.1")))
	})
})

// Normalize output by replacing tabs with spaces, removing extra newlines, and trimming spaces.
func normalizeOutput(s string) string {
	s = strings.ReplaceAll(s, "\t", " ")
	s = strings.ReplaceAll(s, "\n ", "\n")
	s = strings.TrimSpace(s)
	return s
}
