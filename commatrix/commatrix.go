package commatrix

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sync/errgroup"

	"github.com/gocarina/gocsv"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"

	"github.com/openshift-kni/commatrix/client"
	"github.com/openshift-kni/commatrix/consts"
	"github.com/openshift-kni/commatrix/debug"
	"github.com/openshift-kni/commatrix/endpointslices"
	nodesutil "github.com/openshift-kni/commatrix/nodes"
	"github.com/openshift-kni/commatrix/types"
)

// TODO: add integration tests.

type Env int

const (
	Baremetal Env = iota
	AWS
)

type Deployment int

const (
	SNO Deployment = iota
	MNO
)

// New initializes a ComMatrix using Kubernetes cluster data.
// It takes kubeconfigPath for cluster access to  fetch EndpointSlice objects,
// detailing open ports for ingress traffic.
// Custom entries from a JSON file can be added to the matrix by setting `customEntriesPath`.
// Returns a pointer to ComMatrix and error. Entries include traffic direction, protocol,
// port number, namespace, service name, pod, container, node role, and flow optionality for OpenShift.
func New(kubeconfigPath string, customEntriesPath string, customEntriesFormat string, e Env, d Deployment) (*types.ComMatrix, error) {
	res := make([]types.ComDetails, 0)

	cs, err := client.New(kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed creating the k8s client: %w", err)
	}

	epSlicesInfo, err := endpointslices.GetIngressEndpointSlicesInfo(cs)
	if err != nil {
		return nil, fmt.Errorf("failed getting endpointslices: %w", err)
	}

	epSliceComDetails, err := endpointslices.ToComDetails(cs, epSlicesInfo)
	if err != nil {
		return nil, err
	}
	res = append(res, epSliceComDetails...)

	staticEntries, err := getStaticEntries(e, d)
	if err != nil {
		return nil, fmt.Errorf("failed adding static entries: %s", err)
	}

	res = append(res, staticEntries...)

	if customEntriesPath != "" {
		inputFormat, err := parseFormat(customEntriesFormat)
		if err != nil {
			return nil, fmt.Errorf("failed adding custom entries: %s", err)
		}
		customComDetails, err := addFromFile(customEntriesPath, inputFormat)
		if err != nil {
			return nil, fmt.Errorf("failed adding custom entries: %s", err)
		}

		res = append(res, customComDetails...)
	}

	cleanedComDetails := types.CleanComDetails(res)

	return &types.ComMatrix{Matrix: cleanedComDetails}, nil
}

func addFromFile(fp string, format types.Format) ([]types.ComDetails, error) {
	var res []types.ComDetails
	f, err := os.Open(filepath.Clean(fp))
	if err != nil {
		return nil, fmt.Errorf("failed to open file %s: %v", fp, err)
	}
	defer f.Close()
	raw, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %v", fp, err)
	}
	switch format {
	case types.JSON:
		err = json.Unmarshal(raw, &res)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal custom entries file: %v", err)
		}
	case types.YAML:
		err = yaml.Unmarshal(raw, &res)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal custom entries file: %v", err)
		}
	case types.CSV:
		err = gocsv.UnmarshalBytes(raw, &res)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal custom entries file: %v", err)
		}
	default:
		return nil, fmt.Errorf("invalid value for format must be (json,yaml,csv)")
	}

	return res, nil
}

func getStaticEntries(e Env, d Deployment) ([]types.ComDetails, error) {
	comDetails := []types.ComDetails{}

	switch e {
	case Baremetal:
		comDetails = append(comDetails, baremetalStaticEntriesMaster...)
		if d == SNO {
			break
		}
		comDetails = append(comDetails, baremetalStaticEntriesWorker...)
	case AWS:
		comDetails = append(comDetails, awsCloudStaticEntriesMaster...)
		if d == SNO {
			break
		}
		comDetails = append(comDetails, awsCloudStaticEntriesWorker...)
	default:
		return nil, fmt.Errorf("invalid value for cluster environment")
	}

	comDetails = append(comDetails, generalStaticEntriesMaster...)
	if d == SNO {
		return comDetails, nil
	}

	comDetails = append(comDetails, MNOStaticEntries...)
	comDetails = append(comDetails, generalStaticEntriesWorker...)

	return comDetails, nil
}

func parseFormat(format string) (types.Format, error) {
	switch format {
	case types.FormatJSON:
		return types.JSON, nil
	case types.FormatYAML:
		return types.YAML, nil
	case types.FormatCSV:
		return types.CSV, nil
	}

	return types.FormatErr, fmt.Errorf("failed to parse format: %s. options are: (json/yaml/csv)", format)
}

func ApplyFireWallRules(cs *client.ClientSet, m *types.ComMatrix, role string) error {
	var tcpPorts []string
	var udpPorts []string

	for _, line := range m.Matrix {
		if line.NodeRole != role {
			continue
		}
		if line.Protocol == "TCP" {
			tcpPorts = append(tcpPorts, fmt.Sprint(line.Port))
		} else if line.Protocol == "UDP" {
			udpPorts = append(udpPorts, fmt.Sprint(line.Port))
		}
	}

	tcpPortsStr := strings.Join(tcpPorts, ", ")
	udpPortsStr := strings.Join(udpPorts, ", ")

	nodesList, err := cs.CoreV1Interface.Nodes().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list nodes: %w", err)
	}

	g := new(errgroup.Group)
	for _, n := range nodesList.Items {
		node := n
		nodeRole := nodesutil.GetRole(&node)
		if nodeRole != role {
			continue
		}

		g.Go(func() error {
			return applyRulesToNode(cs, node.Name, tcpPortsStr, udpPortsStr)
		})
	}
	if err := g.Wait(); err != nil {
		return fmt.Errorf("failed to apply firewall rules: %w", err)
	}

	return nil

}

func applyRulesToNode(cs *client.ClientSet, nodeName, tcpPortsStr, udpPortsStr string) error {
	debugPod, err := debug.New(cs, nodeName, consts.DefaultDebugNamespace, consts.DefaultDebugPodImage)
	if err != nil {
		return fmt.Errorf("failed to create debug pod on node %s: %w", nodeName, err)
	}
	defer func() {
		err := debugPod.Clean()
		if err != nil {
			fmt.Printf("failed cleaning debug pod %s: %v", debugPod, err)
		}
	}()
	rules := []string{
		"sudo nft add chain ip filter FIREWALL",
		"sudo nft add rule ip filter FIREWALL iif lo accept",
		"sudo nft add rule ip filter FIREWALL ct state established,related accept",
		"sudo nft add rule ip filter FIREWALL tcp dport { 22 } accept",
		"sudo nft add rule ip filter FIREWALL udp dport { 67, 68 } accept",
		"sudo nft add rule ip filter FIREWALL ip protocol icmp accept",
		fmt.Sprintf("sudo nft add rule ip filter FIREWALL tcp dport { %s } accept", tcpPortsStr),
		fmt.Sprintf("sudo nft add rule ip filter FIREWALL udp dport { %s } accept", udpPortsStr),
		"sudo nft add rule ip filter FIREWALL log prefix firewall drop",
		"sudo nft add rule ip filter INPUT jump FIREWALL",
	}
	for _, rule := range rules {
		_, err = debugPod.Exec(rule)
		if err != nil {
			return fmt.Errorf("failed to execute rule %q on node %s: %w", rule, nodeName, err)
		}
	}
	return nil
}
