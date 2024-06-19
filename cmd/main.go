package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/sync/errgroup"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clientutil "github.com/openshift-kni/commatrix/client"
	"github.com/openshift-kni/commatrix/commatrix"
	"github.com/openshift-kni/commatrix/consts"
	"github.com/openshift-kni/commatrix/debug"
	"github.com/openshift-kni/commatrix/ss"
	"github.com/openshift-kni/commatrix/types"
)

func main() {
	var (
		destDir           string
		format            string
		envStr            string
		deploymentStr     string
		customEntriesPath string
		printFn           func(m types.ComMatrix) ([]byte, error)
	)

	flag.StringVar(&destDir, "destDir", "communication-matrix", "Output files dir")
	flag.StringVar(&format, "format", "csv", "Desired format (json,yaml,csv)")
	flag.StringVar(&envStr, "env", "baremetal", "Cluster environment (baremetal/aws)")
	flag.StringVar(&deploymentStr, "deployment", "mno", "Deployment type (mno/sno)")
	flag.StringVar(&customEntriesPath, "customEntriesPath", "", "Add custom entries from a JSON file to the matrix")

	flag.Parse()

	switch format {
	case "json":
		printFn = types.ToJSON
	case "csv":
		printFn = types.ToCSV
	case "yaml":
		printFn = types.ToYAML
	default:
		panic(fmt.Sprintf("invalid format: %s. Please specify json, csv, or yaml.", format))
	}

	kubeconfig, ok := os.LookupEnv("KUBECONFIG")
	if !ok {
		panic("must set the KUBECONFIG environment variable")
	}

	var env commatrix.Env
	switch envStr {
	case "baremetal":
		env = commatrix.Baremetal
	case "aws":
		env = commatrix.AWS
	default:
		panic(fmt.Sprintf("invalid cluster environment: %s", envStr))
	}

	var deployment commatrix.Deployment
	switch deploymentStr {
	case "mno":
		deployment = commatrix.MNO
	case "sno":
		deployment = commatrix.SNO
	default:
		panic(fmt.Sprintf("invalid deployment type: %s", deploymentStr))
	}

	mat, err := commatrix.New(kubeconfig, customEntriesPath, env, deployment)
	if err != nil {
		panic(fmt.Sprintf("failed to create the communication matrix: %s", err))
	}

	res, err := printFn(*mat)
	if err != nil {
		panic(err)
	}

	comMatrixFileName := filepath.Join(destDir, fmt.Sprintf("communication-matrix.%s", format))
	err = os.WriteFile(comMatrixFileName, res, 0644)
	if err != nil {
		panic(err)
	}

	cs, err := clientutil.New(kubeconfig)
	if err != nil {
		panic(err)
	}

	tcpFile, err := os.OpenFile(path.Join(destDir, "raw-ss-tcp"), os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		panic(err)
	}
	defer tcpFile.Close()

	udpFile, err := os.OpenFile(path.Join(destDir, "raw-ss-udp"), os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		panic(err)
	}
	defer udpFile.Close()

	nodesList, err := cs.CoreV1Interface.Nodes().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		panic(err)
	}

	nodesComDetails := []types.ComDetails{}

	err = debug.CreateNamespace(cs, consts.DefaultDebugNamespace)
	if err != nil {
		panic(err)
	}
	defer func() {
		err := debug.DeleteNamespace(cs, consts.DefaultDebugNamespace)
		if err != nil {
			panic(err)
		}
	}()

	nLock := &sync.Mutex{}
	g := new(errgroup.Group)
	for _, n := range nodesList.Items {
		node := n
		g.Go(func() error {
			debugPod, err := debug.New(cs, node.Name, consts.DefaultDebugNamespace, consts.DefaultDebugPodImage)
			if err != nil {
				return err
			}
			defer func() {
				err := debugPod.Clean()
				if err != nil {
					fmt.Printf("failed cleaning debug pod %s: %v", debugPod, err)
				}
			}()

			cds, err := ss.CreateComDetailsFromNode(debugPod, &node, tcpFile, udpFile)
			if err != nil {
				return err
			}
			nLock.Lock()
			nodesComDetails = append(nodesComDetails, cds...)
			nLock.Unlock()
			return nil
		})
	}

	err = g.Wait()
	if err != nil {
		panic(err)
	}

	cleanedComDetails := types.CleanComDetails(nodesComDetails)
	ssComMat := types.ComMatrix{Matrix: cleanedComDetails}

	res, err = printFn(ssComMat)
	if err != nil {
		panic(err)
	}

	ssMatrixFileName := filepath.Join(destDir, fmt.Sprintf("ss-generated-matrix.%s", format))
	err = os.WriteFile(ssMatrixFileName, res, 0644)
	if err != nil {
		panic(err)
	}

	diff, err := buildMatrixDiff(cs, *mat, ssComMat)
	if err != nil {
		panic(err)
	}

	err = os.WriteFile(filepath.Join(destDir, "matrix-diff-ss"),
		[]byte(diff),
		0644)
	if err != nil {
		panic(err)
	}
}

func buildMatrixDiff(cs *clientutil.ClientSet, mat1 types.ComMatrix, mat2 types.ComMatrix) (string, error) {
	diff := consts.CSVHeaders + "\n"

	nodePortMin, nodePortMax, err := getNodePortRange(cs)
	if err != nil {
		return "", err
	}

	skipCondition := func(cd types.ComDetails) bool {
		// Skip "rpc.statd" ports, these are randomly open ports on the node.
		// Skip ovnkube NodePort dynamic ports.
		return cd.Service == "rpc.statd" || (cd.Service == "ovenkube" && cd.Port > nodePortMin && cd.Port < nodePortMax)
	}

	for _, cd := range mat1.Matrix {
		if skipCondition(cd) {
			continue
		}
		if mat2.Contains(cd) {
			diff += fmt.Sprintf("%s\n", cd)
			continue
		}

		diff += fmt.Sprintf("+ %s\n", cd)
	}

	for _, cd := range mat2.Matrix {
		if skipCondition(cd) {
			continue
		}
		if !mat1.Contains(cd) {
			diff += fmt.Sprintf("- %s\n", cd)
		}
	}

	return diff, nil
}

func getNodePortRange(cs *clientutil.ClientSet) (int, int, error) {
	const (
		serviceNodePortMin = 30000
		serviceNodePortMax = 32767
	)

	nodePortMin := serviceNodePortMin
	nodePortMax := serviceNodePortMax

	clusterNetwork, err := cs.ConfigV1Interface.Networks().Get(context.TODO(), "cluster", metav1.GetOptions{})
	if err != nil {
		return 0, 0, err
	}

	serviceNodePortRange := clusterNetwork.Spec.ServiceNodePortRange
	if serviceNodePortRange != "" {
		rangeStr := strings.Split(serviceNodePortRange, "-")
		nodePortMin, err = strconv.Atoi(rangeStr[0])
		if err != nil {
			return 0, 0, err
		}

		nodePortMax, err = strconv.Atoi(rangeStr[1])
		if err != nil {
			return 0, 0, err
		}
	}

	return nodePortMin, nodePortMax, nil
}
