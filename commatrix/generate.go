package commatrix

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sync"

	"golang.org/x/sync/errgroup"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clientutil "github.com/openshift-kni/commatrix/client"
	"github.com/openshift-kni/commatrix/consts"
	"github.com/openshift-kni/commatrix/debug"
	"github.com/openshift-kni/commatrix/ss"
	"github.com/openshift-kni/commatrix/types"
)

func GenerateMatrix(kubeconfig, customEntriesPath, customEntriesFormat, format string, env Env, deployment Deployment, destDir string) (m1, m2 *types.ComMatrix, err error) {
	mat, err := New(kubeconfig, customEntriesPath, customEntriesFormat, env, deployment)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create the communication matrix: %s", err)
	}

	cs, err := clientutil.New(kubeconfig)
	if err != nil {
		return nil, nil, err
	}

	tcpFile, udpFile, err := createOutputFiles(destDir)
	if err != nil {
		return nil, nil, err
	}
	defer tcpFile.Close()
	defer udpFile.Close()

	nodesList, err := cs.CoreV1Interface.Nodes().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, nil, err
	}

	ssComMat, err := getSSmatrix(cs, nodesList, tcpFile, udpFile)
	if err != nil {
		return nil, nil, err
	}
	return mat, ssComMat, err
}

// WriteMatsToFiles write all data to files(ss and endpoint slice matrix and the diff matrix).
func WriteMatsToFiles(mat, ssComMat *types.ComMatrix, format string, env Env, deployment Deployment, destDir string) error {
	printFn, err := getPrintFunction(format)
	if err != nil {
		return err
	}

	err = writeMatrixToFileByType(*mat, "communication-matrix", format, deployment, printFn, destDir)
	if err != nil {
		return err
	}

	err = writeMatrixToFileByType(*ssComMat, "ss-generated-matrix", format, deployment, printFn, destDir)
	if err != nil {
		return err
	}

	return writeMatrixDiff(*mat, *ssComMat, "matrix-diff-ss", destDir)
}

func getPrintFunction(format string) (func(m types.ComMatrix) ([]byte, error), error) {
	switch format {
	case "json":
		return types.ToJSON, nil
	case "csv":
		return types.ToCSV, nil
	case "yaml":
		return types.ToYAML, nil
	case "nft":
		return types.ToNFTables, nil
	default:
		return nil, fmt.Errorf("invalid format: %s. Please specify json, csv, yaml, or nft", format)
	}
}

func createOutputFiles(destDir string) (*os.File, *os.File, error) {
	tcpFile, err := os.OpenFile(path.Join(destDir, "raw-ss-tcp"), os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, nil, err
	}

	udpFile, err := os.OpenFile(path.Join(destDir, "raw-ss-udp"), os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		tcpFile.Close()
		return nil, nil, err
	}

	return tcpFile, udpFile, nil
}

func getSSmatrix(cs *clientutil.ClientSet, nodesList *v1.NodeList, tcpFile, udpFile *os.File) (*types.ComMatrix, error) {
	nodesComDetails := []types.ComDetails{}

	err := debug.CreateNamespace(cs, consts.DefaultDebugNamespace)
	if err != nil {
		return nil, err
	}
	defer func() {
		err := debug.DeleteNamespace(cs, consts.DefaultDebugNamespace)
		if err != nil {
			fmt.Printf("failed to delete debug namespace: %v", err)
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
		return nil, err
	}
	cleanedComDetails := types.CleanComDetails(nodesComDetails)
	ssComMat := types.ComMatrix{Matrix: cleanedComDetails}
	return &ssComMat, nil
}

func writeMatrixDiff(mat types.ComMatrix, ssComMat types.ComMatrix, fileName, destDir string) error {
	diff := buildMatrixDiff(mat, ssComMat)
	return os.WriteFile(filepath.Join(destDir, fileName), []byte(diff), 0644)
}

func writeMatrixToFileByType(mat types.ComMatrix, fileNamePrefix, format string, deployment Deployment, printFn func(m types.ComMatrix) ([]byte, error), destDir string) error {
	if format == types.FormatNFT {
		masterMatrix, workerMatrix := separateMatrixByRole(mat)
		err := writeMatrixToFile(masterMatrix, fileNamePrefix+"-master", format, printFn, destDir)
		if err != nil {
			return err
		}
		if deployment == MNO {
			err := writeMatrixToFile(workerMatrix, fileNamePrefix+"-worker", format, printFn, destDir)
			if err != nil {
				return err
			}
		}
	} else {
		err := writeMatrixToFile(mat, fileNamePrefix, format, printFn, destDir)
		if err != nil {
			return err
		}
	}
	return nil
}

func writeMatrixToFile(matrix types.ComMatrix, fileName, format string, printFn func(m types.ComMatrix) ([]byte, error), destDir string) error {
	res, err := printFn(matrix)
	if err != nil {
		return err
	}

	comMatrixFileName := filepath.Join(destDir, fmt.Sprintf("%s.%s", fileName, format))
	return os.WriteFile(comMatrixFileName, res, 0644)
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

func separateMatrixByRole(matrix types.ComMatrix) (types.ComMatrix, types.ComMatrix) {
	var masterMatrix, workerMatrix types.ComMatrix
	for _, entry := range matrix.Matrix {
		if entry.NodeRole == "master" {
			masterMatrix.Matrix = append(masterMatrix.Matrix, entry)
		} else if entry.NodeRole == "worker" {
			workerMatrix.Matrix = append(workerMatrix.Matrix, entry)
		}
	}

	return masterMatrix, workerMatrix
}
