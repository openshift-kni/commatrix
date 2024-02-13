package ss

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/liornoy/node-comm-lib/internal/client"
	"github.com/liornoy/node-comm-lib/internal/consts"
	"github.com/liornoy/node-comm-lib/internal/debug"
	"github.com/liornoy/node-comm-lib/internal/nodes"
	"github.com/liornoy/node-comm-lib/internal/types"
)

const (
	processeNameFieldIdx  = 5
	localAddrPortFieldIdx = 3
	interval              = time.Millisecond * 500
	duration              = time.Second * 5
)

var (
	// TcpSSFilterFn is a function variable in Go that filters entries from the 'ss' command output.
	// It takes an entry from the 'ss' command output and returns true if the entry represents a TCP port in the listening state.
	tcpSSFilterFn = func(s string) bool {
		return strings.Contains(s, "127.0.0") || !strings.Contains(s, "LISTEN")
	}
	// UdpSSFilterFn is a function variable in Go that filters entries from the 'ss' command output.
	// It takes an entry from the 'ss' command output and returns true if the entry represents a UDP port in the listening state.
	udpSSFilterFn = func(s string) bool {
		return strings.Contains(s, "127.0.0") || !strings.Contains(s, "ESTAB")
	}

	optionalProcesses = map[string]bool{
		"rpcbind":   true,
		"sshd":      true,
		"rpc.statd": true,
	}

	hostServices = map[string]bool{
		"rpcbind":   true,
		"sshd":      true,
		"rpc.statd": true,
		"crio":      true,
		"systemd":   true,
		"kubelet":   true,
	}
)

// FilterPorts filters a slice of comDetails into two separate slices: knownPorts and unknownPorts.
// It identifies known ports based on well-known TCP and UDP port numbers and known service names.
func FilterPorts(comDetails []types.ComDetails) (knownPorts []types.ComDetails, unKnownPorts []types.ComDetails) {
	isKnownTCPPort := getKnownTCPPorts()
	isKnownuDPPort := getKnownUDPorts()

	isKnownPort := func(cd types.ComDetails) bool {
		res := false
		if _, ok := hostServices[cd.ServiceName]; ok {
			res = true
		}
		if cd.Protocol == "TCP" && isKnownTCPPort[cd.Port] {
			res = true
		}
		if cd.Protocol == "UDP" && isKnownuDPPort[cd.Port] {
			res = true
		}

		return res
	}

	for _, cd := range comDetails {
		if isKnownPort(cd) {
			knownPorts = append(knownPorts, cd)
		} else {
			unKnownPorts = append(unKnownPorts, cd)
		}
	}

	return knownPorts, unKnownPorts
}

func CreateComDetailsFromNode(cs *client.ClientSet, node *corev1.Node) ([]types.ComDetails, error) {
	debugPod, err := debug.New(cs, node.Name, consts.DefaultDebugNamespace, consts.DefaultDebugPodImage)
	if err != nil {
		return nil, err
	}
	defer func() {
		err := debugPod.Clean()
		if err != nil {
			fmt.Printf("failed cleaning debug pod %s: %v", debugPod, err)
		}
	}()

	ssOutTCP, err := debugPod.ExecWithRetry("ss -anplt", interval, duration)
	if err != nil {
		return nil, err
	}
	ssOutUDP, err := debugPod.ExecWithRetry("ss -anplu", interval, duration)
	if err != nil {
		return nil, err
	}

	ssOutFilteredTCP := filterStrings(tcpSSFilterFn, splitByLines(ssOutTCP))
	ssOutFilteredUDP := filterStrings(udpSSFilterFn, splitByLines(ssOutUDP))

	tcpComDetails, err := toComDetails(debugPod, ssOutFilteredTCP, "TCP", node)
	if err != nil {
		return nil, err
	}
	udpComDetails, err := toComDetails(debugPod, ssOutFilteredUDP, "UDP", node)
	if err != nil {
		return nil, err
	}

	res := []types.ComDetails{}
	res = append(res, udpComDetails...)
	res = append(res, tcpComDetails...)

	return res, nil
}

func splitByLines(bytes []byte) []string {
	str := string(bytes)
	return strings.Split(str, "\n")
}

func toComDetails(debugPod *debug.DebugPod, ssOutput []string, protocol string, node *corev1.Node) ([]types.ComDetails, error) {
	res := make([]types.ComDetails, 0)
	nodeRoles := nodes.GetRoles(node)

	for _, ssEntry := range ssOutput {
		cd, err := parseComDetail(debugPod, ssEntry)
		if err != nil {
			return nil, err
		}
		cd.Protocol = protocol
		cd.NodeRole = nodeRoles
		if _, ok := optionalProcesses[cd.ServiceName]; ok {
			cd.Required = false
		} else {
			cd.Required = true
		}
		res = append(res, *cd)
	}

	return res, nil
}

func identifyContainerForPort(debugPod *debug.DebugPod, ssEntry string) (string, error) {
	pid, err := extractPID(ssEntry)
	if err != nil {
		return "", err
	}

	containerID, err := extractContainerID(debugPod, pid)
	if err != nil {
		return "", err
	}

	res, err := extractContainerInfo(debugPod, containerID)
	if err != nil {
		return "", err
	}

	return res, nil
}

func extractContainerInfo(debugPod *debug.DebugPod, containerID string) (string, error) {
	type ContainerInfo struct {
		Containers []struct {
			Labels struct {
				ContainerName string `json:"io.kubernetes.container.name"`
				PodName       string `json:"io.kubernetes.pod.name"`
				PodNamespace  string `json:"io.kubernetes.pod.namespace"`
			} `json:"labels"`
		} `json:"containers"`
	}
	containerInfo := &ContainerInfo{}
	cmd := fmt.Sprintf("crictl ps -o json --id %s", containerID)

	out, err := debugPod.ExecWithRetry(cmd, interval, duration)
	if err != nil {
		return "", err
	}

	err = json.Unmarshal(out, &containerInfo)
	if err != nil {
		return "", err
	}
	if len(containerInfo.Containers) != 1 {
		return "", fmt.Errorf("failed extracting pod info, got %d results expected 1. got output:\n%s", len(containerInfo.Containers), string(out))
	}

	// # Commented out logic to fetch namespace and pod details
	// # but for the final matrix we want a readable service name.
	//
	// podNamespace := containerInfo.Containers[0].Labels.PodNamespace
	// podName := containerInfo.Containers[0].Labels.PodName
	containerName := containerInfo.Containers[0].Labels.ContainerName

	return containerName, nil
}

func extractContainerID(debugPod *debug.DebugPod, pid string) (string, error) {
	cmd := fmt.Sprintf("cat /proc/%s/cgroup", pid)
	out, err := debugPod.ExecWithRetry(cmd, interval, duration)
	if err != nil {
		return "", err
	}
	re := regexp.MustCompile(`crio-([0-9a-fA-F]+)\.scope`)

	match := re.FindStringSubmatch(string(out))

	if len(match) < 2 {
		return "", fmt.Errorf("container ID not found node:%s  pid: %s", debugPod.NodeName, pid)
	}

	containerID := match[1]
	return containerID, nil
}

func extractPID(input string) (string, error) {
	re := regexp.MustCompile(`pid=(\d+)`)

	match := re.FindStringSubmatch(input)

	if len(match) < 2 {
		return "", fmt.Errorf("PID not found in the input string")
	}

	pid := match[1]
	return pid, nil
}

func filterStrings(filterOutFn func(string) bool, strs []string) []string {
	res := make([]string, 0)
	for _, s := range strs {
		if filterOutFn(s) {
			continue
		}

		res = append(res, s)
	}

	return res
}

func parseComDetail(debugPod *debug.DebugPod, ssEntry string) (*types.ComDetails, error) {
	serviceName, err := extractServiceName(ssEntry)
	if err != nil {
		return nil, err
	}

	// If not an host service, extract the full service name
	if _, ok := hostServices[serviceName]; !ok {
		containerInfo, err := identifyContainerForPort(debugPod, ssEntry)
		if err != nil {
			return nil, fmt.Errorf("failed identifying container for service %s: %v", serviceName, err)
		}

		serviceName = containerInfo
	}

	fields := strings.Fields(ssEntry)
	portIdx := strings.LastIndex(fields[localAddrPortFieldIdx], ":")
	port := fields[localAddrPortFieldIdx][portIdx+1:]

	return &types.ComDetails{
		Direction:   consts.IngressLabel,
		Port:        port,
		ServiceName: serviceName,
		Required:    true}, nil
}

func extractServiceName(ssEntry string) (string, error) {
	re := regexp.MustCompile(`users:\(\("(?P<servicename>[^"]+)"`)

	match := re.FindStringSubmatch(ssEntry)

	if len(match) < 2 {
		return "", fmt.Errorf("service name not found in the input string: %s", ssEntry)
	}

	serviceName := match[re.SubexpIndex("servicename")]

	return serviceName, nil
}
