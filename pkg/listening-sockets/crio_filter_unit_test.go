package listeningsockets

import (
	"errors"
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
	pod := debugPodWithContainerID()

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
		crioStreamPortProbeCommand("deadbeef"),
	).Return([]byte(`time="now" level=debug msg="ExecResponse: url:\"http://127.0.0.1:62000/exec/token\""`), nil)
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

func TestCreateSSOutputFiltersOnlyRuntimeCRIOStreamPort(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockUtils := mock_utils.NewMockUtilsInterface(ctrl)
	check := &ConnectionCheck{podUtils: mockUtils}
	pod := debugPodWithContainerID()

	tcpOutput := []byte(
		"LISTEN 0 4096 0.0.0.0:45000 0.0.0.0:* users:((\"crio\",pid=42,fd=3))\n" +
			"LISTEN 0 4096 0.0.0.0:40000 0.0.0.0:* users:((\"crio\",pid=42,fd=4))\n" +
			"LISTEN 0 4096 0.0.0.0:41000 0.0.0.0:* users:((\"custom-daemon\",pid=43,fd=5))",
	)
	mockUtils.EXPECT().RunCommandOnPod(pod, []string{"/bin/sh", "-c", "ss -anpltH"}).Return(tcpOutput, nil)
	mockUtils.EXPECT().RunCommandOnPod(pod, []string{"/bin/sh", "-c", "ss -anpluH"}).Return(nil, nil)
	mockUtils.EXPECT().RunCommandOnPod(
		pod,
		[]string{"/bin/sh", "-c", "cat /host/proc/sys/net/ipv4/ip_local_port_range"},
	).Return([]byte("32768 60999\n"), nil)
	mockUtils.EXPECT().RunCommandOnPod(
		pod,
		[]string{"chroot", "/host", "/bin/sh", "-c", "crio status config"},
	).Return([]byte("[crio.api]\nstream_port = \"0\"\n[crio.metrics]\nenable_metrics = true\nmetrics_host = \"0.0.0.0\"\nmetrics_port = 40000\n"), nil)
	mockUtils.EXPECT().RunCommandOnPod(
		pod,
		crioStreamPortProbeCommand("deadbeef"),
	).Return([]byte(`time="now" level=debug msg="ExecResponse: url:\"http://127.0.0.1:45000/exec/token\""`), nil)
	mockUtils.EXPECT().RunCommandOnPod(
		pod,
		[]string{"chroot", "/host", "/bin/sh", "-c", "ip -j addr show lo"},
	).Return([]byte("[]"), nil)
	mockUtils.EXPECT().RunCommandOnPod(
		pod,
		[]string{"/bin/sh", "-c", "cat /proc/42/cgroup"},
	).Return([]byte("0::/system.slice/crio.service\n"), nil).Times(2)
	mockUtils.EXPECT().RunCommandOnPod(
		pod,
		[]string{"/bin/sh", "-c", "cat /proc/43/cgroup"},
	).Return([]byte("0::/system.slice/custom-daemon.service\n"), nil)
	expectVerifiedCRIOProcess(mockUtils, pod, "42")

	got, _, _, err := check.createSSOutputFromNode(pod, "worker")
	if err != nil {
		t.Fatalf("createSSOutputFromNode returned an error: %v", err)
	}
	if len(got) != 2 || got[0].Port != 40000 || got[1].Port != 41000 {
		t.Fatalf("expected fixed metrics and unrelated listeners to remain, got %#v", got)
	}
}

func TestCreateSSOutputRetainsListenersWhenClassificationUnavailable(t *testing.T) {
	tests := []struct {
		name            string
		rangeOutput     []byte
		rangeErr        error
		configOutput    []byte
		configErr       error
		expectConfig    bool
		probeOutput     []byte
		probeErr        error
		expectProbe     bool
		omitContainerID bool
	}{
		{
			name:         "CRI-O config command failure",
			rangeOutput:  []byte("32768 60999\n"),
			configErr:    errors.New("config unavailable"),
			expectConfig: true,
		},
		{
			name:         "unrecognized stream port formatting",
			rangeOutput:  []byte("32768 60999\n"),
			configOutput: []byte("[crio.api]\nstream_port = 0\n"),
			expectConfig: true,
		},
		{
			name:         "runtime stream probe failure",
			rangeOutput:  []byte("32768 60999\n"),
			configOutput: []byte("[crio.api]\nstream_port = \"0\"\n"),
			expectConfig: true,
			probeErr:     errors.New("probe unavailable"),
			expectProbe:  true,
		},
		{
			name:         "malformed runtime stream response",
			rangeOutput:  []byte("32768 60999\n"),
			configOutput: []byte("[crio.api]\nstream_port = \"0\"\n"),
			expectConfig: true,
			probeOutput:  []byte("ExecResponse: unavailable"),
			expectProbe:  true,
		},
		{
			name:            "debug container ID unavailable",
			rangeOutput:     []byte("32768 60999\n"),
			configOutput:    []byte("[crio.api]\nstream_port = \"0\"\n"),
			expectConfig:    true,
			omitContainerID: true,
		},
		{
			name:         "host range command failure",
			rangeErr:     errors.New("range unavailable"),
			expectConfig: false,
		},
		{
			name:         "malformed host range",
			rangeOutput:  []byte("not-a-range\n"),
			expectConfig: false,
		},
		{
			name:         "reversed host range",
			rangeOutput:  []byte("60999 32768\n"),
			expectConfig: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockUtils := mock_utils.NewMockUtilsInterface(ctrl)
			check := &ConnectionCheck{podUtils: mockUtils}
			pod := debugPodWithContainerID()
			if tt.omitContainerID {
				pod.Status.ContainerStatuses = nil
			}
			entry := []byte(`LISTEN 0 4096 0.0.0.0:40000 0.0.0.0:* users:(("crio",pid=42,fd=3))`)

			mockUtils.EXPECT().RunCommandOnPod(
				pod, []string{"/bin/sh", "-c", "ss -anpltH"},
			).Return(entry, nil)
			mockUtils.EXPECT().RunCommandOnPod(
				pod, []string{"/bin/sh", "-c", "ss -anpluH"},
			).Return(nil, nil)
			mockUtils.EXPECT().RunCommandOnPod(
				pod, []string{"/bin/sh", "-c", "cat /host/proc/sys/net/ipv4/ip_local_port_range"},
			).Return(tt.rangeOutput, tt.rangeErr)
			if tt.expectConfig {
				mockUtils.EXPECT().RunCommandOnPod(
					pod, []string{"chroot", "/host", "/bin/sh", "-c", "crio status config"},
				).Return(tt.configOutput, tt.configErr)
			}
			if tt.expectProbe {
				mockUtils.EXPECT().RunCommandOnPod(
					pod, crioStreamPortProbeCommand("deadbeef"),
				).Return(tt.probeOutput, tt.probeErr)
			}
			mockUtils.EXPECT().RunCommandOnPod(
				pod, []string{"chroot", "/host", "/bin/sh", "-c", "ip -j addr show lo"},
			).Return([]byte("[]"), nil)
			mockUtils.EXPECT().RunCommandOnPod(
				pod, []string{"/bin/sh", "-c", "cat /proc/42/cgroup"},
			).Return([]byte("0::/system.slice/crio.service\n"), nil)

			got, _, _, err := check.createSSOutputFromNode(pod, "worker")
			if err != nil {
				t.Fatalf("classification uncertainty must not abort collection: %v", err)
			}
			if len(got) != 1 || got[0].Port != 40000 {
				t.Fatalf("expected uncertain listener to remain reportable, got %#v", got)
			}
		})
	}
}

func TestHostEphemeralRangeEndpointsAreInclusive(t *testing.T) {
	tests := []struct {
		name       string
		streamPort int
		streamPID  string
	}{
		{name: "minimum endpoint", streamPort: defaultEphemeralMin, streamPID: "42"},
		{name: "maximum endpoint", streamPort: defaultEphemeralMax, streamPID: "43"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockUtils := mock_utils.NewMockUtilsInterface(ctrl)
			check := &ConnectionCheck{podUtils: mockUtils}
			pod := &corev1.Pod{}
			entries := []string{
				`LISTEN 0 4096 0.0.0.0:32767 0.0.0.0:* users:(("crio",pid=41,fd=3))`,
				`LISTEN 0 4096 0.0.0.0:32768 0.0.0.0:* users:(("crio",pid=42,fd=3))`,
				`LISTEN 0 4096 0.0.0.0:60999 0.0.0.0:* users:(("crio",pid=43,fd=3))`,
				`LISTEN 0 4096 0.0.0.0:61000 0.0.0.0:* users:(("crio",pid=44,fd=3))`,
			}

			for _, pid := range []string{"41", "42", "43", "44"} {
				mockUtils.EXPECT().RunCommandOnPod(
					pod, []string{"/bin/sh", "-c", fmt.Sprintf("cat /proc/%s/cgroup", pid)},
				).Return([]byte("0::/system.slice/crio.service\n"), nil)
			}
			expectVerifiedCRIOProcess(mockUtils, pod, tt.streamPID)

			got := check.toComDetails(
				pod,
				entries,
				"TCP",
				"worker",
				portRange{min: defaultEphemeralMin, max: defaultEphemeralMax},
				tt.streamPort,
			)
			if len(got) != 3 {
				t.Fatalf("expected only runtime stream port %d to be filtered, got %#v", tt.streamPort, got)
			}
			for _, listener := range got {
				if listener.Port == tt.streamPort {
					t.Fatalf("expected runtime stream port %d to be filtered, got %#v", tt.streamPort, got)
				}
			}
		})
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
		40000,
	)
	if len(got) != 1 || got[0].Service != "crio" {
		t.Fatalf("expected spoofed process name to remain reportable, got %#v", got)
	}
}

func TestUncertainCRIOProvenanceRemainsReportable(t *testing.T) {
	tests := []struct {
		name       string
		exeOutput  []byte
		exeErr     error
		cgroup     []byte
		cgroupErr  error
		readCgroup bool
	}{
		{
			name:   "executable read failure",
			exeErr: errors.New("readlink failed"),
		},
		{
			name:       "cgroup read failure",
			exeOutput:  []byte("/usr/bin/crio\n"),
			cgroupErr:  errors.New("cgroup read failed"),
			readCgroup: true,
		},
		{
			name:       "cgroup mismatch",
			exeOutput:  []byte("/usr/bin/crio\n"),
			cgroup:     []byte("0::/system.slice/not-crio.service\n"),
			readCgroup: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockUtils := mock_utils.NewMockUtilsInterface(ctrl)
			check := &ConnectionCheck{podUtils: mockUtils}
			pod := &corev1.Pod{}
			entry := `LISTEN 0 4096 0.0.0.0:40000 0.0.0.0:* users:(("crio",pid=42,fd=3))`

			mockUtils.EXPECT().RunCommandOnPod(
				pod, []string{"/bin/sh", "-c", "cat /proc/42/cgroup"},
			).Return([]byte("0::/system.slice/crio.service\n"), nil)
			mockUtils.EXPECT().RunCommandOnPod(
				pod, []string{"/bin/sh", "-c", "readlink /host/proc/42/exe"},
			).Return(tt.exeOutput, tt.exeErr)
			if tt.readCgroup {
				mockUtils.EXPECT().RunCommandOnPod(
					pod, []string{"/bin/sh", "-c", "cat /host/proc/42/cgroup"},
				).Return(tt.cgroup, tt.cgroupErr)
			}

			got := check.toComDetails(
				pod,
				[]string{entry},
				"TCP",
				"worker",
				portRange{min: defaultEphemeralMin, max: defaultEphemeralMax},
				40000,
			)
			if len(got) != 1 || got[0].Port != 40000 {
				t.Fatalf("expected listener with uncertain provenance to remain reportable, got %#v", got)
			}
		})
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
				40000,
			)
			matrix := &types.ComMatrix{Ports: ports}
			matrix.SortAndRemoveDuplicates()
			if len(matrix.Ports) != 1 || matrix.Ports[0].Service != "custom-daemon" {
				t.Fatalf("expected reportable owner to survive regardless of input order, got %#v", matrix.Ports)
			}
		})
	}
}

func TestMultipleOwnersRemainReportableRegardlessOfOrder(t *testing.T) {
	orders := []struct {
		name  string
		entry string
		first string
	}{
		{
			name:  "CRI-O owner first",
			entry: `LISTEN 0 4096 0.0.0.0:40000 0.0.0.0:* users:(("crio",pid=42,fd=3),("custom-daemon",pid=43,fd=4))`,
			first: "42",
		},
		{
			name:  "reportable owner first",
			entry: `LISTEN 0 4096 0.0.0.0:40000 0.0.0.0:* users:(("custom-daemon",pid=43,fd=4),("crio",pid=42,fd=3))`,
			first: "43",
		},
	}

	for _, tt := range orders {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockUtils := mock_utils.NewMockUtilsInterface(ctrl)
			check := &ConnectionCheck{podUtils: mockUtils}
			pod := &corev1.Pod{}

			mockUtils.EXPECT().RunCommandOnPod(
				pod, []string{"/bin/sh", "-c", fmt.Sprintf("cat /proc/%s/cgroup", tt.first)},
			).Return([]byte("0::/system.slice/service.scope\n"), nil)
			expectVerifiedCRIOProcess(mockUtils, pod, "42")
			mockUtils.EXPECT().RunCommandOnPod(
				pod, []string{"/bin/sh", "-c", "readlink /host/proc/43/exe"},
			).Return([]byte("/usr/bin/custom-daemon\n"), nil)

			got := check.toComDetails(
				pod,
				[]string{tt.entry},
				"TCP",
				"worker",
				portRange{min: defaultEphemeralMin, max: defaultEphemeralMax},
				40000,
			)
			if len(got) != 1 || got[0].Port != 40000 {
				t.Fatalf("expected mixed-owner listener to remain visible, got %#v", got)
			}
		})
	}
}

func TestEmptyNamedCoOwnerRemainsReportableRegardlessOfOrder(t *testing.T) {
	tests := []struct {
		name     string
		entry    string
		firstPID string
	}{
		{
			name:     "empty owner first",
			entry:    `LISTEN 0 4096 0.0.0.0:40000 0.0.0.0:* users:(("",pid=43,fd=4),("crio",pid=42,fd=3))`,
			firstPID: "43",
		},
		{
			name:     "CRI-O owner first",
			entry:    `LISTEN 0 4096 0.0.0.0:40000 0.0.0.0:* users:(("crio",pid=42,fd=3),("",pid=43,fd=4))`,
			firstPID: "42",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockUtils := mock_utils.NewMockUtilsInterface(ctrl)
			check := &ConnectionCheck{podUtils: mockUtils}
			pod := &corev1.Pod{}

			mockUtils.EXPECT().RunCommandOnPod(
				pod, []string{"/bin/sh", "-c", fmt.Sprintf("cat /proc/%s/cgroup", tt.firstPID)},
			).Return([]byte("0::/system.slice/service.scope\n"), nil)
			expectVerifiedCRIOProcess(mockUtils, pod, "42")
			mockUtils.EXPECT().RunCommandOnPod(
				pod,
				[]string{"/bin/sh", "-c", "readlink /host/proc/43/exe"},
			).Return([]byte("/usr/bin/custom-daemon\n"), nil)

			got := check.toComDetails(
				pod,
				[]string{tt.entry},
				"TCP",
				"worker",
				portRange{min: defaultEphemeralMin, max: defaultEphemeralMax},
				40000,
			)
			if len(got) != 1 || got[0].Port != 40000 {
				t.Fatalf("expected empty-named mixed owner to remain visible, got %#v", got)
			}
		})
	}
}

func TestMalformedCoOwnerRemainsReportable(t *testing.T) {
	tests := []struct {
		name         string
		entry        string
		verifyOwners bool
	}{
		{
			name:  "missing PID",
			entry: `LISTEN 0 4096 0.0.0.0:40000 0.0.0.0:* users:(("crio",pid=42,fd=3),("unknown",fd=4))`,
		},
		{
			name:         "unquoted task name with valid PID",
			entry:        `LISTEN 0 4096 0.0.0.0:40000 0.0.0.0:* users:(("crio",pid=42,fd=3),(unknown,pid=43,fd=4))`,
			verifyOwners: true,
		},
		{
			name:  "unterminated task name",
			entry: `LISTEN 0 4096 0.0.0.0:40000 0.0.0.0:* users:(("crio",pid=42,fd=3),("unknown,pid=43,fd=4))`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockUtils := mock_utils.NewMockUtilsInterface(ctrl)
			check := &ConnectionCheck{podUtils: mockUtils}
			pod := &corev1.Pod{}

			mockUtils.EXPECT().RunCommandOnPod(
				pod, []string{"/bin/sh", "-c", "cat /proc/42/cgroup"},
			).Return([]byte("0::/system.slice/crio.service\n"), nil)
			if tt.verifyOwners {
				expectVerifiedCRIOProcess(mockUtils, pod, "42")
				mockUtils.EXPECT().RunCommandOnPod(
					pod,
					[]string{"/bin/sh", "-c", "readlink /host/proc/43/exe"},
				).Return([]byte("/usr/bin/custom-daemon\n"), nil)
			}

			got := check.toComDetails(
				pod,
				[]string{tt.entry},
				"TCP",
				"worker",
				portRange{min: defaultEphemeralMin, max: defaultEphemeralMax},
				40000,
			)
			if len(got) != 1 || got[0].Port != 40000 {
				t.Fatalf("expected listener with malformed co-owner to remain visible, got %#v", got)
			}
		})
	}
}

func TestAllVerifiedCRIOOwnersAreSuppressed(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockUtils := mock_utils.NewMockUtilsInterface(ctrl)
	check := &ConnectionCheck{podUtils: mockUtils}
	pod := &corev1.Pod{}
	entry := `LISTEN 0 4096 0.0.0.0:40000 0.0.0.0:* users:(("crio",pid=42,fd=3),("crio",pid=43,fd=4))`

	mockUtils.EXPECT().RunCommandOnPod(
		pod, []string{"/bin/sh", "-c", "cat /proc/42/cgroup"},
	).Return([]byte("0::/system.slice/crio.service\n"), nil)
	expectVerifiedCRIOProcess(mockUtils, pod, "42")
	expectVerifiedCRIOProcess(mockUtils, pod, "43")

	got := check.toComDetails(
		pod,
		[]string{entry},
		"TCP",
		"worker",
		portRange{min: defaultEphemeralMin, max: defaultEphemeralMax},
		40000,
	)
	if len(got) != 0 {
		t.Fatalf("expected listener with only verified CRI-O owners to be suppressed, got %#v", got)
	}
}

func TestVerifiedCRIOUDPListenerRemainsReportable(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockUtils := mock_utils.NewMockUtilsInterface(ctrl)
	check := &ConnectionCheck{podUtils: mockUtils}
	pod := &corev1.Pod{}
	entry := `UNCONN 0 0 0.0.0.0:40000 0.0.0.0:* users:(("crio",pid=42,fd=3))`

	mockUtils.EXPECT().RunCommandOnPod(
		pod, []string{"/bin/sh", "-c", "cat /proc/42/cgroup"},
	).Return([]byte("0::/system.slice/crio.service\n"), nil)

	got := check.toComDetails(
		pod,
		[]string{entry},
		"UDP",
		"worker",
		portRange{min: defaultEphemeralMin, max: defaultEphemeralMax},
		40000,
	)
	if len(got) != 1 || got[0].Protocol != "UDP" || got[0].Port != 40000 {
		t.Fatalf("expected CRI-O UDP listener to remain reportable, got %#v", got)
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
		40000,
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

	ephemeralStreamPort, err := check.getCRIOEphemeralStreamPort(pod)
	if err != nil {
		t.Fatalf("getCRIOEphemeralStreamPort returned an error: %v", err)
	}
	got := check.toComDetails(
		pod,
		[]string{entry},
		"TCP",
		"worker",
		portRange{min: defaultEphemeralMin, max: defaultEphemeralMax},
		ephemeralStreamPort,
	)
	if len(got) != 1 || got[0].Port != 40000 {
		t.Fatalf("expected configured fixed CRI-O stream port to remain reportable, got %#v", got)
	}
}

func debugPodWithContainerID() *corev1.Pod {
	return &corev1.Pod{
		Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "container"}}},
		Status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{{
			Name:        "container",
			ContainerID: "cri-o://deadbeef",
		}}},
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
