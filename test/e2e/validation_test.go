package e2e

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"

	"os"
	"strconv"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	configv1 "github.com/openshift/api/config/v1"
	clientOptions "sigs.k8s.io/controller-runtime/pkg/client"

	matrixdiff "github.com/openshift-kni/commatrix/pkg/matrix-diff"
	"github.com/openshift-kni/commatrix/pkg/types"
)

var (
	// Entries which are open on the worker node instead of master in standard cluster.
	// Will be excluded in diff generatation between documented and generated comMatrix.
	StandardExcludedMasterComDetails = []types.ComDetails{
		{
			Direction: "Ingress",
			Protocol:  "TCP",
			Port:      80,
			NodeRole:  "master",
			Service:   "router-default",
			Namespace: "openshift-ingress",
			Pod:       "router-default",
			Container: "router",
			Optional:  false,
		}, {
			Direction: "Ingress",
			Protocol:  "TCP",
			Port:      443,
			NodeRole:  "master",
			Service:   "router-default",
			Namespace: "openshift-ingress",
			Pod:       "router-default",
			Container: "router",
			Optional:  false,
		},
	}
	commatrix *types.ComMatrix
)

const (
	docCommatrixBaseFilePath = "../../docs/stable/raw/%s.csv"
	diffFileComments         = "// `+` indicates a port that isn't in the current documented matrix, and has to be added.\n" +
		"// `-` indicates a port that has to be removed from the documented matrix.\n"
	commatrixFile      = "communication-matrix.csv"
	matrixdiffFile     = "matrix-diff-ss"
	serviceNodePortMin = 30000
	serviceNodePortMax = 32767
)

type EPSStatus string

const (
	NotUsed EPSStatus = "+"
	Missing EPSStatus = "-"
)

var _ = Describe("Validation", func() {
	BeforeEach(func() {
		By("Generating communication matrix using oc command")
		cmd := exec.Command("oc", "commatrix", "generate", "--host-open-ports", "--destDir", artifactsDir, "--platform-type", platformType)
		err := cmd.Run()
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to run command: %s", cmd.String()))

		By("Reading the generated commatrix files")
		commatrixFilePath := filepath.Join(artifactsDir, commatrixFile)
		commatrixFileContent, err := os.ReadFile(commatrixFilePath)
		Expect(err).ToNot(HaveOccurred(), "Failed to read generated commatrix file")

		ComDetailsMatrix, err := types.ParseToComDetailsList(commatrixFileContent, types.FormatCSV)
		Expect(err).ToNot(HaveOccurred(), "Failed to parse generated commatrix")
		commatrix = &types.ComMatrix{Matrix: ComDetailsMatrix}
	})

	It("generated communication matrix should be equal to documented communication matrix", func() {
		By("generate documented commatrix file path")
		docType := "aws"
		if isBM {
			docType = "bm"
		}
		if isSNO {
			docType += "-sno"
		}
		docCommatrixFilePath := fmt.Sprintf(docCommatrixBaseFilePath, docType)

		By(fmt.Sprintf("Filter documented commatrix type %s for diff generation", docType))
		docCommatrixFileContent, err := os.ReadFile(docCommatrixFilePath)
		Expect(err).ToNot(HaveOccurred(), "Failed to read documented communication matrix file")

		docComDetailsList, err := types.ParseToComDetailsList(docCommatrixFileContent, types.FormatCSV)
		Expect(err).ToNot(HaveOccurred(), "Failed to parse documented communication matrix")

		if isSNO {
			// Exclude all worker nodes static entries.
			docComDetailsList = excludeStaticEntriesWithGivenNodeRole(docComDetailsList, &types.ComMatrix{Matrix: docComDetailsList}, "worker")
			// Exclude static entries of standard deployment type.
			docComDetailsList = excludeStaticEntriesWithGivenNodeRole(docComDetailsList, &types.ComMatrix{Matrix: types.StandardStaticEntries}, "master")
		} else {
			// Exclude specific master entries (see StandardExcludedMasterComDetails var description)
			docComDetailsList = excludeStaticEntriesWithGivenNodeRole(docComDetailsList, &types.ComMatrix{Matrix: StandardExcludedMasterComDetails}, "master")
		}

		// if cluster is running on BM exclude Cloud static entries in diff generation
		// else cluster is running on Cloud and exclude BM static entries in diff generation.
		if isBM {
			docComDetailsList = excludeStaticEntriesWithGivenNodeRole(docComDetailsList, &types.ComMatrix{Matrix: types.CloudStaticEntriesWorker}, "worker")
			docComDetailsList = excludeStaticEntriesWithGivenNodeRole(docComDetailsList, &types.ComMatrix{Matrix: types.CloudStaticEntriesMaster}, "master")
		} else {
			docComDetailsList = excludeStaticEntriesWithGivenNodeRole(docComDetailsList, &types.ComMatrix{Matrix: types.BaremetalStaticEntriesWorker}, "worker")
			docComDetailsList = excludeStaticEntriesWithGivenNodeRole(docComDetailsList, &types.ComMatrix{Matrix: types.BaremetalStaticEntriesMaster}, "master")
		}

		docComMatrix := &types.ComMatrix{Matrix: docComDetailsList}

		By("generating diff between matrices for testing purposes")
		endpointslicesDiffWithDocMat := matrixdiff.Generate(commatrix, docComMatrix)
		diffStr, err := endpointslicesDiffWithDocMat.String()
		Expect(err).ToNot(HaveOccurred())

		err = os.WriteFile(filepath.Join(artifactsDir, "doc-diff-new"), []byte(diffFileComments+diffStr), 0644)
		Expect(err).ToNot(HaveOccurred())

		By("comparing new and documented commatrices")
		// Get ports that are in the documented commatrix but not in the generated commatrix.
		notUsedPortsMat := endpointslicesDiffWithDocMat.GenerateUniqueSecondary()
		if len(notUsedPortsMat.Matrix) > 0 {
			logrus.Warningf("the following ports are documented but are not used:\n%s", notUsedPortsMat)
		}

		var portsToIgnoreMat *types.ComMatrix

		openPortsToIgnoreFile, _ := os.LookupEnv("OPEN_PORTS_TO_IGNORE_IN_DOC_TEST_FILE")
		openPortsToIgnoreFormat, _ := os.LookupEnv("OPEN_PORTS_TO_IGNORE_IN_DOC_TEST_FORMAT")

		// Get ports that are in the generated commatrix but not in the documented commatrix,
		// and ignore the ports in given file (if exists)
		missingPortsMat := endpointslicesDiffWithDocMat.GenerateUniquePrimary()
		if openPortsToIgnoreFile != "" && openPortsToIgnoreFormat != "" {
			portsToIgnoreFileContent, err := os.ReadFile(openPortsToIgnoreFile)
			Expect(err).ToNot(HaveOccurred())

			portsToIgnoreComDetails, err := types.ParseToComDetailsList(portsToIgnoreFileContent, openPortsToIgnoreFormat)
			Expect(err).ToNot(HaveOccurred())

			portsToIgnoreMat = &types.ComMatrix{Matrix: portsToIgnoreComDetails}

			// generate the diff matrix between the open ports to ignore matrix and the missing ports in the documented commatrix (based on the diff between the enpointslice and the doc matrix)
			nonDocumentedEndpointslicesMat := endpointslicesDiffWithDocMat.GenerateUniquePrimary()
			endpointslicesDiffWithIgnoredPorts := matrixdiff.Generate(nonDocumentedEndpointslicesMat, portsToIgnoreMat)
			missingPortsMat = endpointslicesDiffWithIgnoredPorts.GenerateUniquePrimary()
		}

		if len(missingPortsMat.Matrix) > 0 {
			err := fmt.Errorf("the following ports are used but are not documented:\n%s", missingPortsMat)
			Expect(err).ToNot(HaveOccurred())
		}
	})

	It("should validate the communication matrix ports match the node's open ports", func() {
		By("Reading the generated diff commatrix files")
		matDiffSSFilePath := filepath.Join(artifactsDir, matrixdiffFile)
		matDiffSSFileContent, err := os.ReadFile(matDiffSSFilePath)
		Expect(err).ToNot(HaveOccurred(), "Failed to read matrix-diff-ss file")

		notUsedEPSMat, err := extractEPSMatByStatus(matDiffSSFileContent, NotUsed)
		Expect(err).ToNot(HaveOccurred(), "Failed to extract not used EPS Matrix")

		missingEPSMat, err := extractEPSMatByStatus(matDiffSSFileContent, Missing)
		Expect(err).ToNot(HaveOccurred(), "Failed to extract missing EPS Matrix")

		if len(notUsedEPSMat.Matrix) > 0 {
			logrus.Warningf("the following ports are not used: \n %s", notUsedEPSMat)
		}

		missingEPSMat, err = filterKnownPorts(missingEPSMat)
		Expect(err).ToNot(HaveOccurred(), "Failed to filter the known ports")
		if len(missingEPSMat.Matrix) > 0 {
			err := fmt.Errorf("the following ports are used but don't have an endpointslice: \n %s", missingEPSMat)
			Expect(err).ToNot(HaveOccurred())
		}
	})
})

// excludeStaticEntriesWithGivenNodeRole excludes from comDetails, static entries from staticEntriesMatrix with the given nodeRole
// The function returns filtered ComDetails without the excluded entries.
func excludeStaticEntriesWithGivenNodeRole(comDetails []types.ComDetails, staticEntriesMatrix *types.ComMatrix, nodeRole string) []types.ComDetails {
	filteredComDetails := []types.ComDetails{}
	for _, cd := range comDetails {
		switch cd.NodeRole {
		case nodeRole:
			if !staticEntriesMatrix.Contains(cd) {
				filteredComDetails = append(filteredComDetails, cd)
			}
		default:
			filteredComDetails = append(filteredComDetails, cd)
		}
	}
	return filteredComDetails
}

// Filter known ports to skip on checking.
func filterKnownPorts(mat *types.ComMatrix) (*types.ComMatrix, error) {
	nodePortMin := serviceNodePortMin
	nodePortMax := serviceNodePortMax

	clusterNetwork := &configv1.Network{}
	err := cs.Get(context.Background(), clientOptions.ObjectKey{Name: "cluster"}, clusterNetwork)
	if err != nil {
		return nil, err
	}

	serviceNodePortRange := clusterNetwork.Spec.ServiceNodePortRange
	if serviceNodePortRange != "" {
		rangeStr := strings.Split(serviceNodePortRange, "-")

		nodePortMin, err = strconv.Atoi(rangeStr[0])
		if err != nil {
			return nil, err
		}

		nodePortMax, err = strconv.Atoi(rangeStr[1])
		if err != nil {
			return nil, err
		}
	}

	res := []types.ComDetails{}
	for _, cd := range mat.Matrix {
		// Skip "ovnkube" ports in the nodePort range, these are dynamic open ports on the node,
		// no need to mention them in the matrix diff
		if cd.Service == "ovnkube" && cd.Port >= nodePortMin && cd.Port <= nodePortMax {
			continue
		}

		// Skip "rpc.statd" ports, these are randomly open ports on the node,
		// no need to mention them in the matrix diff
		if cd.Service == "rpc.statd" {
			continue
		}

		// Skip crio stream server port, allocated to a random free port number,
		// shouldn't be exposed to the public Internet for security reasons,
		// no need to mention it in the matrix diff
		if cd.Service == "crio" && cd.Port > nodePortMax {
			continue
		}

		// Skip dns ports used during provisioning for dhcp and tftp,
		// not used for external traffic
		if cd.Service == "dnsmasq" || cd.Service == "dig" {
			continue
		}

		res = append(res, cd)
	}

	return &types.ComMatrix{Matrix: res}, nil
}

// extractEPSMatByStatus extracts and returns ComMatrix by filtering lines of a CSV content based on a EPS status.
func extractEPSMatByStatus(csvContent []byte, status EPSStatus) (*types.ComMatrix, error) {
	filteredCSV := extractDiffByStatus(csvContent, status)

	prefixEPSMat, err := types.ParseToComDetailsList(filteredCSV, types.FormatCSV)
	if err != nil {
		return nil, err
	}

	return &types.ComMatrix{Matrix: prefixEPSMat}, nil
}

// extractDiffByStatus filter the lines of the csv content based on the EPS status
// Example:
// CSV content:
// Direction,Protocol,Port,Namespace,Service,Pod,Container,Node Role,Optional
// Ingress, TCP, 80, Namespace1, service1, pod1, container1, worker, true
// + Ingress, TCP, 8080, Namespace2, service2, pod2, container2, worker, true
// - Ingress, UDP, 9090, Namespace2, service3, pod3, container3, master, false
//
// Calling extractDiffByStatus(csvContent, NotUsed) will return the filtered CSV content:
// Direction,Protocol,Port,Namespace,Service,Pod,Container,Node Role,Optional
// Ingress, TCP, 8080, Namespace2, service2, pod2, container2, worker, true
//
// Calling extractDiffByStatus(csvContent, Missing) will return the filtered CSV content:
// Direction,Protocol,Port,Namespace,Service,Pod,Container,Node Role,Optional
// Ingress, UDP, 9090, Namespace2, service3, pod3, container3, master, false.
func extractDiffByStatus(csvContent []byte, status EPSStatus) []byte {
	prefix := []byte(status)
	var filteredLines [][]byte
	lines := bytes.Split(csvContent, []byte("\n"))

	// take headers
	if len(lines) > 0 {
		filteredLines = append(filteredLines, lines[0])
	}

	// filter by prefix (+ or -)
	for _, line := range lines[1:] {
		line = bytes.TrimSpace(line)
		if bytes.HasPrefix(line, prefix) {
			filteredLines = append(filteredLines, line[2:])
		}
	}

	return bytes.Join(filteredLines, []byte("\n"))
}
