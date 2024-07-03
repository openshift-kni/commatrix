package generateMatrix

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sync"

	"golang.org/x/sync/errgroup"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clientutil "github.com/openshift-kni/commatrix/client"
	"github.com/openshift-kni/commatrix/cmd/apply-firewall"
	"github.com/openshift-kni/commatrix/commatrix"
	"github.com/openshift-kni/commatrix/consts"
	"github.com/openshift-kni/commatrix/debug"
	"github.com/openshift-kni/commatrix/ss"
	"github.com/openshift-kni/commatrix/types"
)

func GeneratCommatrix(kubeconfig, customEntriesPath, customEntriesFormat, format string, env commatrix.Env, deployment commatrix.Deployment, printFn func(m types.ComMatrix) ([]byte, error), destDir string) {
	mat, err := commatrix.New(kubeconfig, customEntriesPath, customEntriesFormat, env, deployment)
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

	diff := buildMatrixDiff(*mat, ssComMat)

	err = os.WriteFile(filepath.Join(destDir, "matrix-diff-ss"),
		[]byte(diff),
		0644)
	if err != nil {
		panic(err)
	}
	applyFirewall.CreateNFTtable(destDir, mat, deployment)
}

func buildMatrixDiff(mat1 types.ComMatrix, mat2 types.ComMatrix) string {
	diff := consts.CSVHeaders + "\n"
	for _, cd := range mat1.Matrix {
		if mat2.Contains(cd) {
			diff += fmt.Sprintf("%s\n", cd)
			continue
		}

		diff += fmt.Sprintf("+ %s\n", cd)
	}

	for _, cd := range mat2.Matrix {
		// Skip "rpc.statd" ports, these are randomly open ports on the node,
		// no need to mention them in the matrix diff
		if cd.Service == "rpc.statd" {
			continue
		}

		if !mat1.Contains(cd) {
			diff += fmt.Sprintf("- %s\n", cd)
		}
	}

	return diff
}
