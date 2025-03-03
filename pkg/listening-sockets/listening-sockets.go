package listeningsockets

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	clientOptions "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift-kni/commatrix/pkg/client"
	"github.com/openshift-kni/commatrix/pkg/consts"
	"github.com/openshift-kni/commatrix/pkg/endpointslices"
	"github.com/openshift-kni/commatrix/pkg/types"
	"github.com/openshift-kni/commatrix/pkg/utils"
)

const (
	localAddrPortFieldIdx = 3
	interval              = time.Millisecond * 500
	duration              = time.Second * 5
)

type ConnectionCheck struct {
	*client.ClientSet
	podUtils   utils.UtilsInterface
	destDir    string
	nodeToRole map[string]string
}

func NewCheck(c *client.ClientSet, podUtils utils.UtilsInterface, destDir string) (*ConnectionCheck, error) {
	nodeList := &corev1.NodeList{}
	err := c.List(context.TODO(), nodeList)
	if err != nil {
		return nil, err
	}

	nodeToRole := map[string]string{}
	for _, node := range nodeList.Items {
		nodeToRole[node.Name], err = types.GetNodeRole(&node)
		if err != nil {
			return nil, err
		}
	}

	return &ConnectionCheck{
		c,
		podUtils,
		destDir,
		nodeToRole,
	}, nil
}

func (cc *ConnectionCheck) GenerateSS(namespace string) (*types.ComMatrix, []byte, []byte, error) {
	var ssOutTCP, ssOutUDP []byte
	nodesComDetails := []types.ComDetails{}

	nLock := &sync.Mutex{}
	g := new(errgroup.Group)
	for nodeName := range cc.nodeToRole {
		name := nodeName
		g.Go(func() error {
			debugPod, err := cc.podUtils.CreatePodOnNode(name, namespace, consts.DefaultDebugPodImage, []string{})
			if err != nil {
				return err
			}

			err = cc.podUtils.WaitForPodStatus(namespace, debugPod, corev1.PodRunning)
			if err != nil {
				return err
			}

			defer func() {
				err := cc.podUtils.DeletePod(debugPod)
				if err != nil {
					fmt.Printf("failed cleaning debug pod %s: %v", debugPod, err)
				}
			}()

			cds, ssTCP, ssUDP, err := cc.createSSOutputFromNode(debugPod, name)
			if err != nil {
				return err
			}
			nLock.Lock()
			defer nLock.Unlock()
			ssTCPLine := fmt.Sprintf("node: %s\n%s\n", name, string(ssTCP))
			ssUDPLine := fmt.Sprintf("node: %s\n%s\n", name, string(ssUDP))

			nodesComDetails = append(nodesComDetails, cds...)
			ssOutTCP = append(ssOutTCP, []byte(ssTCPLine)...)
			ssOutUDP = append(ssOutUDP, []byte(ssUDPLine)...)
			return nil
		})
	}

	err := g.Wait()
	if err != nil {
		return nil, nil, nil, err
	}

	ssComMat := types.ComMatrix{Matrix: nodesComDetails}
	ssComMat.SortAndRemoveDuplicates()

	return &ssComMat, ssOutTCP, ssOutUDP, nil
}

func (cc *ConnectionCheck) WriteSSRawFiles(ssOutTCP, ssOutUDP []byte) error {
	err := cc.podUtils.WriteFile(path.Join(cc.destDir, "raw-ss-tcp"), ssOutTCP)
	if err != nil {
		return fmt.Errorf("failed writing to file: %s", err)
	}

	err = cc.podUtils.WriteFile(path.Join(cc.destDir, "raw-ss-udp"), ssOutUDP)
	if err != nil {
		return fmt.Errorf("failed writing to file: %s", err)
	}

	return nil
}

func (cc *ConnectionCheck) createSSOutputFromNode(debugPod *corev1.Pod, nodeName string) ([]types.ComDetails, []byte, []byte, error) {
	ssOutTCP, err := cc.podUtils.RunCommandOnPod(debugPod, []string{"/bin/sh", "-c", "ss -anpltH"})
	if err != nil {
		return nil, nil, nil, err
	}
	ssOutUDP, err := cc.podUtils.RunCommandOnPod(debugPod, []string{"/bin/sh", "-c", "ss -anpluH"})
	if err != nil {
		return nil, nil, nil, err
	}

	ssOutFilteredTCP := filterEntries(splitByLines(ssOutTCP))
	ssOutFilteredUDP := filterEntries(splitByLines(ssOutUDP))

	tcpComDetails := cc.toComDetails(debugPod, ssOutFilteredTCP, "TCP", cc.nodeToRole[nodeName])
	udpComDetails := cc.toComDetails(debugPod, ssOutFilteredUDP, "UDP", cc.nodeToRole[nodeName])

	res := []types.ComDetails{}
	res = append(res, udpComDetails...)
	res = append(res, tcpComDetails...)

	return res, ssOutTCP, ssOutUDP, nil
}

func splitByLines(bytes []byte) []string {
	str := string(bytes)
	return strings.Split(str, "\n")
}

func (cc *ConnectionCheck) toComDetails(debugPod *corev1.Pod, ssOutput []string, protocol string, nodeRole string) []types.ComDetails {
	res := make([]types.ComDetails, 0)

	for _, ssEntry := range ssOutput {
		cd := parseComDetail(ssEntry)

		name, err := cc.getContainerName(debugPod, ssEntry)
		if err != nil {
			log.Debugf("failed to identify container for ss entry: %serr: %s", ssEntry, err)
		}

		cd.Container = name
		cd.Protocol = protocol
		cd.NodeRole = nodeRole
		cd.Optional = false
		res = append(res, *cd)
	}
	return res
}

// getContainerName receives an ss entry and gets the name of the container exposing this port.
func (cc *ConnectionCheck) getContainerName(debugPod *corev1.Pod, ssEntry string) (string, error) {
	pid, err := extractPID(ssEntry)
	if err != nil {
		return "", err
	}

	containerID, err := cc.extractContainerID(debugPod, pid)
	if err != nil {
		return "", err
	}

	res, err := cc.extractContainerName(debugPod, containerID)
	if err != nil {
		return "", err
	}

	return res, nil
}

// extractContainerID receives a PID of a container, and returns its CRI-O ID.
func (cc *ConnectionCheck) extractContainerID(debugPod *corev1.Pod, pid string) (string, error) {
	cmd := fmt.Sprintf("cat /proc/%s/cgroup", pid)
	out, err := cc.podUtils.RunCommandOnPod(debugPod, []string{"/bin/sh", "-c", cmd})
	if err != nil {
		return "", err
	}
	re := regexp.MustCompile(`crio-([0-9a-fA-F]+)\.scope`)

	match := re.FindStringSubmatch(string(out))

	if len(match) < 2 {
		return "", fmt.Errorf("container ID not found node:%s  pid: %s", debugPod.Spec.NodeName, pid)
	}

	containerID := match[1]
	return containerID, nil
}

// extractContainerName receives CRI-O container ID and returns the container's name.
func (cc *ConnectionCheck) extractContainerName(debugPod *corev1.Pod, containerID string) (string, error) {
	containerInfo := &types.ContainerInfo{}
	cmd := fmt.Sprintf("crictl ps -o json --id %s", containerID)
	out, err := cc.podUtils.RunCommandOnPod(debugPod, []string{"chroot", "/host", "/bin/sh", "-c", cmd})
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

	containerName := containerInfo.Containers[0].Labels.ContainerName

	return containerName, nil
}

// extractPID receives an ss entry and returns the PID number of it.
func extractPID(ssEntry string) (string, error) {
	re := regexp.MustCompile(`pid=(\d+)`)

	match := re.FindStringSubmatch(ssEntry)

	if len(match) < 2 {
		return "", fmt.Errorf("PID not found in the input string")
	}

	pid := match[1]
	return pid, nil
}

func filterEntries(ssEntries []string) []string {
	res := make([]string, 0)
	for _, s := range ssEntries {
		if strings.Contains(s, "127.0.0") || strings.Contains(s, "::1") || s == "" {
			continue
		}

		res = append(res, s)
	}

	return res
}

func parseComDetail(ssEntry string) *types.ComDetails {
	serviceName, err := extractServiceName(ssEntry)
	if err != nil {
		log.Debug(err.Error())
	}

	fields := strings.Fields(ssEntry)
	portIdx := strings.LastIndex(fields[localAddrPortFieldIdx], ":")
	portStr := fields[localAddrPortFieldIdx][portIdx+1:]

	port, err := strconv.Atoi(portStr)
	if err != nil {
		log.Debug(err.Error())
		return nil
	}

	return &types.ComDetails{
		Direction: consts.IngressLabel,
		Port:      port,
		Service:   serviceName,
		Optional:  false}
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

func (cc *ConnectionCheck) FilterExternalPorts(matrix *types.ComMatrix) (*types.ComMatrix, error) {
	var ssComDetails []types.ComDetails
	for _, cd := range matrix.Matrix {
		serviceList := &corev1.ServiceList{}
		err := cc.List(context.TODO(), serviceList)
		if err != nil {
			return nil, fmt.Errorf("failed to list services: %w", err)
		}

		var foundService *corev1.Service
		for _, service := range serviceList.Items {
			for _, port := range service.Spec.Ports {
				if int32(cd.Port) == port.Port {
					foundService = &service
					break
				}
			}
			if foundService != nil {
				break
			}
		}

		if foundService == nil {
			ssComDetails = append(ssComDetails, cd)
			continue
		}

		label := labels.SelectorFromSet(foundService.Spec.Selector)
		pods := &corev1.PodList{}
		err = cc.List(context.TODO(), pods, &clientOptions.ListOptions{Namespace: foundService.Namespace, LabelSelector: label})
		if err != nil {
			return nil, fmt.Errorf("failed to list pods, %v", err)
		}

		if len(pods.Items) == 0 {
			ssComDetails = append(ssComDetails, cd)
			continue
		}

		if !endpointslices.FilterServiceTypes(*foundService) && endpointslices.FilterHostNetwork(pods.Items[0]) {
			continue
		} // dont include ports that are internal and host network.

		ssComDetails = append(ssComDetails, cd)
	}
	return &types.ComMatrix{Matrix: ssComDetails}, nil
}
