package listeningsockets

import (
	"fmt"
	"testing"

	"go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"

	"github.com/openshift-kni/commatrix/pkg/types"
	mock_utils "github.com/openshift-kni/commatrix/pkg/utils/mock"
)

const (
	defaultEphemeralMin = 32768
	defaultEphemeralMax = 60999
)

func TestCreateSSOutputUsesConfiguredHostEphemeralRange(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockUtils := mock_utils.NewMockUtilsInterface(ctrl)
	check := &ConnectionCheck{podUtils: mockUtils}
	pod := &corev1.Pod{}

	tcpOutput := []byte(
		"LISTEN 0 4096 0.0.0.0:40000 0.0.0.0:* users:((\"crio\",pid=42,fd=3))\n" +
			"LISTEN 0 4096 0.0.0.0:62000 0.0.0.0:* users:((\"crio\",pid=42,fd=4))",
	)
	mockUtils.EXPECT().RunCommandOnPod(pod, []string{"/bin/sh", "-c", "ss -anpltH"}).Return(tcpOutput, nil)
	mockUtils.EXPECT().RunCommandOnPod(pod, []string{"/bin/sh", "-c", "ss -anpluH"}).Return(nil, nil)
	mockUtils.EXPECT().RunCommandOnPod(
		pod,
		[]string{"/bin/sh", "-c", "cat /host/proc/sys/net/ipv4/ip_local_port_range"},
	).Return([]byte("61000 65000\n"), nil)
	mockUtils.EXPECT().RunCommandOnPod(
		pod,
		[]string{"chroot", "/host", "/bin/sh", "-c", "crio status config"},
	).Return([]byte("[crio.api]\nstream_port = \"0\"\n"), nil)
	mockUtils.EXPECT().RunCommandOnPod(
		pod,
		[]string{"chroot", "/host", "/bin/sh", "-c", "ip -j addr show lo"},
	).Return([]byte("[]"), nil)
	mockUtils.EXPECT().RunCommandOnPod(
		pod,
		[]string{"/bin/sh", "-c", "cat /proc/42/cgroup"},
	).Return([]byte("0::/system.slice/crio.service\n"), nil).Times(2)
	expectVerifiedCRIOProcess(mockUtils, pod, "42")

	got, _, _, err := check.createSSOutputFromNode(pod, "worker")
	if err != nil {
		t.Fatalf("createSSOutputFromNode returned an error: %v", err)
	}
	if len(got) != 1 || got[0].Port != 40000 {
		t.Fatalf("expected fixed port 40000 to remain and ephemeral port 62000 to be ignored, got %#v", got)
	}
}

func TestMutableProcessNameDoesNotEstablishCRIOProvenance(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockUtils := mock_utils.NewMockUtilsInterface(ctrl)
	check := &ConnectionCheck{podUtils: mockUtils}
	pod := &corev1.Pod{}
	entry := `LISTEN 0 4096 0.0.0.0:40000 0.0.0.0:* users:(("crio",pid=99,fd=3))`

	mockUtils.EXPECT().RunCommandOnPod(
		pod,
		[]string{"/bin/sh", "-c", "cat /proc/99/cgroup"},
	).Return([]byte("0::/user.slice/user-1000.slice/session.scope\n"), nil)
	mockUtils.EXPECT().RunCommandOnPod(
		pod,
		[]string{"/bin/sh", "-c", "readlink /host/proc/99/exe"},
	).Return([]byte("/usr/bin/python3\n"), nil)

	got := check.toComDetails(
		pod,
		[]string{entry},
		"TCP",
		"worker",
		portRange{min: defaultEphemeralMin, max: defaultEphemeralMax},
		true,
	)
	if len(got) != 1 || got[0].Service != "crio" {
		t.Fatalf("expected spoofed process name to remain reportable, got %#v", got)
	}
}

func TestVerifiedCRIOFilteringPrecedesOwnershipDeduplication(t *testing.T) {
	orders := []struct {
		name    string
		entries []string
	}{
		{
			name: "CRI-O first",
			entries: []string{
				`LISTEN 0 4096 0.0.0.0:40000 0.0.0.0:* users:(("anything",pid=42,fd=3))`,
				`LISTEN 0 4096 0.0.0.0:40000 0.0.0.0:* users:(("custom-daemon",pid=43,fd=3))`,
			},
		},
		{
			name: "reportable owner first",
			entries: []string{
				`LISTEN 0 4096 0.0.0.0:40000 0.0.0.0:* users:(("custom-daemon",pid=43,fd=3))`,
				`LISTEN 0 4096 0.0.0.0:40000 0.0.0.0:* users:(("anything",pid=42,fd=3))`,
			},
		},
	}

	for _, tt := range orders {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockUtils := mock_utils.NewMockUtilsInterface(ctrl)
			check := &ConnectionCheck{podUtils: mockUtils}
			pod := &corev1.Pod{}

			mockUtils.EXPECT().RunCommandOnPod(
				pod,
				[]string{"/bin/sh", "-c", "cat /proc/42/cgroup"},
			).Return([]byte("0::/system.slice/crio.service\n"), nil)
			mockUtils.EXPECT().RunCommandOnPod(
				pod,
				[]string{"/bin/sh", "-c", "cat /proc/43/cgroup"},
			).Return([]byte("0::/system.slice/custom-daemon.service\n"), nil)
			expectVerifiedCRIOProcess(mockUtils, pod, "42")
			mockUtils.EXPECT().RunCommandOnPod(
				pod,
				[]string{"/bin/sh", "-c", "readlink /host/proc/43/exe"},
			).Return([]byte("/usr/bin/custom-daemon\n"), nil)

			ports := check.toComDetails(
				pod,
				tt.entries,
				"TCP",
				"worker",
				portRange{min: defaultEphemeralMin, max: defaultEphemeralMax},
				true,
			)
			matrix := &types.ComMatrix{Ports: ports}
			matrix.SortAndRemoveDuplicates()
			if len(matrix.Ports) != 1 || matrix.Ports[0].Service != "custom-daemon" {
				t.Fatalf("expected reportable owner to survive regardless of input order, got %#v", matrix.Ports)
			}
		})
	}
}

func TestKubernetesIdentifiedListenerRemainsReportable(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockUtils := mock_utils.NewMockUtilsInterface(ctrl)
	check := &ConnectionCheck{podUtils: mockUtils}
	pod := &corev1.Pod{}
	entry := `LISTEN 0 4096 0.0.0.0:40000 0.0.0.0:* users:(("crio",pid=44,fd=3))`

	mockUtils.EXPECT().RunCommandOnPod(
		pod,
		[]string{"/bin/sh", "-c", "cat /proc/44/cgroup"},
	).Return([]byte("0::/kubepods.slice/crio-abcd.scope\n"), nil)
	mockUtils.EXPECT().RunCommandOnPod(
		pod,
		[]string{"chroot", "/host", "/bin/sh", "-c", "crictl ps -o json --id abcd"},
	).Return([]byte(`{"containers":[{"labels":{"io.kubernetes.container.name":"container","io.kubernetes.pod.name":"pod","io.kubernetes.pod.namespace":"namespace"}}]}`), nil)

	got := check.toComDetails(
		pod,
		[]string{entry},
		"TCP",
		"worker",
		portRange{min: defaultEphemeralMin, max: defaultEphemeralMax},
		true,
	)
	if len(got) != 1 || got[0].Namespace != "namespace" || got[0].Pod != "pod" || got[0].Container != "container" {
		t.Fatalf("expected Kubernetes-identified listener to remain reportable, got %#v", got)
	}
}

func TestFixedCRIOStreamPortRemainsReportable(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockUtils := mock_utils.NewMockUtilsInterface(ctrl)
	check := &ConnectionCheck{podUtils: mockUtils}
	pod := &corev1.Pod{}
	entry := `LISTEN 0 4096 0.0.0.0:40000 0.0.0.0:* users:(("crio",pid=42,fd=3))`

	mockUtils.EXPECT().RunCommandOnPod(
		pod,
		[]string{"chroot", "/host", "/bin/sh", "-c", "crio status config"},
	).Return([]byte("[crio.api]\nstream_port = \"40000\"\n"), nil)
	mockUtils.EXPECT().RunCommandOnPod(
		pod,
		[]string{"/bin/sh", "-c", "cat /proc/42/cgroup"},
	).Return([]byte("0::/system.slice/crio.service\n"), nil)

	usesEphemeralStreamPort, err := check.crioUsesEphemeralStreamPort(pod)
	if err != nil {
		t.Fatalf("crioUsesEphemeralStreamPort returned an error: %v", err)
	}
	got := check.toComDetails(
		pod,
		[]string{entry},
		"TCP",
		"worker",
		portRange{min: defaultEphemeralMin, max: defaultEphemeralMax},
		usesEphemeralStreamPort,
	)
	if len(got) != 1 || got[0].Port != 40000 {
		t.Fatalf("expected configured fixed CRI-O stream port to remain reportable, got %#v", got)
	}
}

func expectVerifiedCRIOProcess(mockUtils *mock_utils.MockUtilsInterface, pod *corev1.Pod, pid string) {
	mockUtils.EXPECT().RunCommandOnPod(
		pod,
		[]string{"/bin/sh", "-c", fmt.Sprintf("readlink /host/proc/%s/exe", pid)},
	).Return([]byte("/usr/bin/crio\n"), nil)
	mockUtils.EXPECT().RunCommandOnPod(
		pod,
		[]string{"/bin/sh", "-c", fmt.Sprintf("cat /host/proc/%s/cgroup", pid)},
	).Return([]byte("0::/system.slice/crio.service\n"), nil)
}
