package e2e

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"

	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	configv1 "github.com/openshift/api/config/v1"

	commatrixcreator "github.com/openshift-kni/commatrix/pkg/commatrix-creator"
	"github.com/openshift-kni/commatrix/pkg/endpointslices"
	matrixdiff "github.com/openshift-kni/commatrix/pkg/matrix-diff"
	"github.com/openshift-kni/commatrix/pkg/mcp"
	"github.com/openshift-kni/commatrix/pkg/types"
)

var commatrix *types.ComMatrix

const (
	docCommatrixBaseFilePath = "../../docs/stable/raw/%s.csv"
	diffFileComments         = "// `+` indicates a port that isn't in the current documented matrix, and has to be added.\n" +
		"// `-` indicates a port that has to be removed from the documented matrix.\n"
	commatrixFile      = "communication-matrix.csv"
	ssCommatrixFile    = "ss-generated-matrix.csv"
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
		cmd := exec.Command("oc", "commatrix", "generate", "--host-open-ports", "--destDir", artifactsDir)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf(
			"Failed to run command: %s\nstdout:\n%s\nstderr:\n%s",
			cmd.String(), stdout.String(), stderr.String(),
		))

		By("Reading the generated commatrix files")
		commatrixFilePath := filepath.Join(artifactsDir, commatrixFile)
		commatrixFileContent, err := os.ReadFile(commatrixFilePath)
		Expect(err).ToNot(HaveOccurred(), "Failed to read generated commatrix file")

		commatrix, err = types.ParseToComMatrix(commatrixFileContent, types.FormatCSV)
		Expect(err).ToNot(HaveOccurred(), "Failed to parse generated commatrix")
	})

	It("generated communication matrix should be equal to documented communication matrix", func() {
		By("generate documented commatrix file path")

		// clusters with unsupported platform types had skip the test, so we assume the platform type is supported
		var docType string
		switch platformType {
		case configv1.AWSPlatformType:
			docType = "aws"
		case configv1.BareMetalPlatformType:
			docType = "bm"
		case configv1.NonePlatformType:
			docType = "none"
		}

		// Asuming telco partenrs are using only none type and aws platform types for sno clusters.
		if controlPlaneTopology == configv1.SingleReplicaTopologyMode {
			docType += "-sno"
		}
		docCommatrixFilePath := fmt.Sprintf(docCommatrixBaseFilePath, docType)

		By(fmt.Sprintf("Filter documented commatrix type %s for diff generation", docType))
		docCommatrixFileContent, err := os.ReadFile(docCommatrixFilePath)
		Expect(err).ToNot(HaveOccurred(), "Failed to read documented communication matrix file")

		// Normalize header: rename "Node Role" to our csv tag "NodeGroup"
		docCommatrixFileContent = bytes.Replace(docCommatrixFileContent, []byte("Node Role"), []byte("NodeGroup"), 1)

		docComMatrix, err := types.ParseToComMatrix(docCommatrixFileContent, types.FormatCSV)
		Expect(err).ToNot(HaveOccurred(), "Failed to parse documented communication matrix")
		docComMatrix.DynamicRanges = append(docComMatrix.DynamicRanges, types.KubeletNodePortDefaultDynamicRange...)

		By("generating diff between matrices for testing purposes")
		endpointslicesDiffWithDocMat := matrixdiff.Generate(commatrix, docComMatrix)
		diffStr, err := endpointslicesDiffWithDocMat.String()
		Expect(err).ToNot(HaveOccurred())

		err = os.WriteFile(filepath.Join(artifactsDir, "doc-diff-commatrix"), []byte(diffFileComments+diffStr), 0644)
		Expect(err).ToNot(HaveOccurred())

		By("comparing new and documented commatrices")
		// Get ports that are in the documented commatrix but not in the generated commatrix.
		notUsedPortsMat := endpointslicesDiffWithDocMat.GetUniqueSecondary()
		if len(notUsedPortsMat.Ports) > 0 {
			logrus.Warningf("the following ports are documented but are not used:\n%s", notUsedPortsMat)
		}

		var portsToIgnoreMat *types.ComMatrix

		openPortsToIgnoreFile, _ := os.LookupEnv("OPEN_PORTS_TO_IGNORE_IN_DOC_TEST_FILE")
		openPortsToIgnoreFormat, _ := os.LookupEnv("OPEN_PORTS_TO_IGNORE_IN_DOC_TEST_FORMAT")

		// Get ports that are in the generated commatrix but not in the documented commatrix,
		// and ignore the ports in given file (if exists)
		missingPortsMat := endpointslicesDiffWithDocMat.GetUniquePrimary()
		if openPortsToIgnoreFile != "" && openPortsToIgnoreFormat != "" {
			portsToIgnoreFileContent, err := os.ReadFile(openPortsToIgnoreFile)
			Expect(err).ToNot(HaveOccurred())

			portsToIgnoreMat, err = types.ParseToComMatrix(portsToIgnoreFileContent, openPortsToIgnoreFormat)
			Expect(err).ToNot(HaveOccurred())

			// generate the diff matrix between the open ports to ignore matrix and the missing ports in the documented commatrix (based on the diff between the enpointslice and the doc matrix)
			nonDocumentedEndpointslicesMat := endpointslicesDiffWithDocMat.GetUniquePrimary()
			endpointslicesDiffWithIgnoredPorts := matrixdiff.Generate(nonDocumentedEndpointslicesMat, portsToIgnoreMat)
			missingPortsMat = endpointslicesDiffWithIgnoredPorts.GetUniquePrimary()
		}

		// Don't include in the missing ports matrix ports that are in dynamic ranges of the documented commatrix.
		missingPortsMat = filterOutPortsInDynamicRanges(missingPortsMat, docComMatrix.DynamicRanges)
		if len(missingPortsMat.Ports) > 0 {
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

		if len(notUsedEPSMat.Ports) > 0 {
			logrus.Warningf("the following ports are not used: \n %s", notUsedEPSMat)
		}

		// Don't include in the missing EPS matrix ports that are in dynamic ranges of the generated commatrix.
		missingEPSMat = filterOutPortsInDynamicRanges(missingEPSMat, commatrix.DynamicRanges)
		if len(missingEPSMat.Ports) > 0 {
			err := fmt.Errorf("the following ports are used but don't have an endpointslice: \n %s", missingEPSMat)
			Expect(err).ToNot(HaveOccurred(), "Failed to filter the known ports")
		}
	})

	It("should validate all static entries suitable to the cluster are actually open ports", func() {
		By("Reading the ss-generated matrix (actual open ports from the cluster)")
		ssCommatrixFilePath := filepath.Join(artifactsDir, ssCommatrixFile)
		ssCommatrixFileContent, err := os.ReadFile(ssCommatrixFilePath)
		Expect(err).ToNot(HaveOccurred(), "Failed to read ss-generated matrix file")

		ssCommatrix, err := types.ParseToComMatrix(ssCommatrixFileContent, types.FormatCSV)
		Expect(err).ToNot(HaveOccurred(), "Failed to parse ss-generated matrix")

		By("Getting IPv6 enabled status")
		ipv6Enabled, err := utilsHelpers.IsIPv6Enabled()
		Expect(err).ToNot(HaveOccurred(), "Failed to get IPv6 enabled status")

		By("Creating communication matrix creator to get static entries")
		exporter, err := endpointslices.New(cs)
		Expect(err).ToNot(HaveOccurred(), "Failed to create endpointslices exporter")

		cm, err := commatrixcreator.New(exporter, "", "", platformType, controlPlaneTopology, ipv6Enabled, utilsHelpers)
		Expect(err).ToNot(HaveOccurred(), "Failed to create communication matrix creator")

		By("Getting static entries suitable to the cluster")
		staticEntries, err := cm.GetStaticEntries()
		Expect(err).ToNot(HaveOccurred(), "Failed to get static entries")

		By("Expand static entries for all MCPs based on their roles")
		PoolRolesForStaticEntriesExpansion, err := mcp.GetPoolRolesForStaticEntriesExpansion(exporter.ClientSet, exporter.NodeToGroup())
		Expect(err).ToNot(HaveOccurred(), "Failed to get pool roles for static entries expansion")
		staticEntries = commatrixcreator.ExpandStaticEntriesByPool(staticEntries, PoolRolesForStaticEntriesExpansion)

		By("Checking that all static entries are present in the ss (open ports) matrix")
		var missingStaticEntries []types.ComDetails
		for _, staticEntry := range staticEntries {
			if !ssCommatrix.Contains(staticEntry) {
				missingStaticEntries = append(missingStaticEntries, staticEntry)
			}
		}

		if len(missingStaticEntries) > 0 {
			missingMatrix := &types.ComMatrix{Ports: missingStaticEntries}
			Fail(fmt.Sprintf("the following static entries are not open ports:\n%s", missingMatrix))
		}
	})
})

func filterOutPortsInDynamicRanges(mat *types.ComMatrix, dynamicRanges []types.DynamicRange) *types.ComMatrix {
	res := []types.ComDetails{}
	for _, cd := range mat.Ports {
		skip := false
		for _, dr := range dynamicRanges {
			if cd.Protocol == dr.Protocol {
				if cd.Port >= dr.MinPort && cd.Port <= dr.MaxPort {
					skip = true
					break
				}
			}
		}
		if !skip {
			res = append(res, cd)
		}
	}

	return &types.ComMatrix{Ports: res}
}

// extractEPSMatByStatus extracts and returns ComMatrix by filtering lines of a CSV content based on a EPS status.
func extractEPSMatByStatus(csvContent []byte, status EPSStatus) (*types.ComMatrix, error) {
	filteredCSV := extractDiffByStatus(csvContent, status)

	prefixEPSMat, err := types.ParseToComMatrix(filteredCSV, types.FormatCSV)
	if err != nil {
		return nil, err
	}

	return prefixEPSMat, nil
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
