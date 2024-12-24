package e2e

import (
	"context"
	"fmt"
	"io"

	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	configv1 "github.com/openshift/api/config/v1"
	clientOptions "sigs.k8s.io/controller-runtime/pkg/client"

	commatrixcreator "github.com/openshift-kni/commatrix/pkg/commatrix-creator"
	"github.com/openshift-kni/commatrix/test/pkg/cluster"

	listeningsockets "github.com/openshift-kni/commatrix/pkg/listening-sockets"
	matrixdiff "github.com/openshift-kni/commatrix/pkg/matrix-diff"
	"github.com/openshift-kni/commatrix/pkg/types"
)

var (
	extraEndpointSlicesFilePath   = ""
	extraEndpointSlicesFileFormat = ""
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
)

const (
	minimalDocCommatrixVersion = 4.16
	docCommatrixBaseURL        = "https://raw.githubusercontent.com/openshift/openshift-docs/enterprise-VERSION/snippets/network-flow-matrix.csv"
	diffFileComments           = "// `+` indicates a port that isn't in the current documented matrix, and has to be added.\n" +
		"// `-` indicates a port that has to be removed from the documented matrix.\n"
)

const (
	serviceNodePortMin = 30000
	serviceNodePortMax = 32767
)

var _ = Describe("Validation", func() {
	It("generated communication matrix should be equal to documented communication matrix", func() {
		By("get cluster's version and check if it's suitable for test")
		clusterVersion, err := cluster.GetClusterVersion(cs)
		Expect(err).NotTo(HaveOccurred())
		floatClusterVersion, err := strconv.ParseFloat(clusterVersion, 64)
		Expect(err).ToNot(HaveOccurred())

		if floatClusterVersion < minimalDocCommatrixVersion {
			Skip(fmt.Sprintf("If the cluster version is lower than the lowest version that "+
				"has a documented communication matrix (%v), skip test", minimalDocCommatrixVersion))
		}

		By(fmt.Sprintf("get documented commatrix version %s", clusterVersion))
		// get documented commatrix from URL
		resp, err := http.Get(strings.Replace(docCommatrixBaseURL, "VERSION", clusterVersion, 1))
		Expect(err).ToNot(HaveOccurred())
		defer resp.Body.Close()
		// if response status code equals to "status not found", compare generated commatrix to the main documented commatrix
		if resp.StatusCode == http.StatusNotFound {
			resp, err = http.Get(strings.Replace(docCommatrixBaseURL, "enterprise-VERSION", "main", 1))
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode).ToNot(Equal(http.StatusNotFound))
		}

		By("write documented commatrix to artifact file")
		docCommatrixContent, err := io.ReadAll(resp.Body)
		Expect(err).ToNot(HaveOccurred())
		docFilePath := filepath.Join(artifactsDir, "doc-commatrix.csv")
		err = os.WriteFile(docFilePath, docCommatrixContent, 0644)
		Expect(err).ToNot(HaveOccurred())

		By("Filter documented commatrix for diff generation")
		// get origin documented commatrix details
		docComMatrixCreator, err := commatrixcreator.New(epExporter, docFilePath, types.FormatCSV, infra, deployment)
		Expect(err).ToNot(HaveOccurred())
		docComDetailsList, err := docComMatrixCreator.GetComDetailsListFromFile()
		Expect(err).ToNot(HaveOccurred())

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
		diff := matrixdiff.Generate(commatrix, docComMatrix)
		diffStr, err := diff.String()
		Expect(err).ToNot(HaveOccurred())
		err = os.WriteFile(filepath.Join(artifactsDir, "doc-diff-new"), []byte(diffFileComments+diffStr), 0644)
		Expect(err).ToNot(HaveOccurred())

		By("comparing new and documented commatrices")
		// Get ports that are in the documented commatrix but not in the generated commatrix.
		notUsedPortsMat := diff.GenerateUniqueSecondary()
		if len(notUsedPortsMat.Matrix) > 0 {
			logrus.Warningf("the following ports are documented but are not used:\n%s", notUsedPortsMat)
		}

		// Get ports that are in the generated commatrix but not in the documented commatrix.
		missingPortsMat := diff.GenerateUniquePrimary()
		if len(missingPortsMat.Matrix) > 0 {
			err := fmt.Errorf("the following ports are used but are not documented:\n%s", missingPortsMat)
			Expect(err).ToNot(HaveOccurred())
		}
	})

	It("should validate the communication matrix ports match the node's listening ports", func() {
		listeningCheck, err := listeningsockets.NewCheck(cs, utilsHelpers, artifactsDir)
		Expect(err).ToNot(HaveOccurred())

		By("generate the ss matrix and ss raws")
		ssMat, ssOutTCP, ssOutUDP, err := listeningCheck.GenerateSS(testNS)
		Expect(err).ToNot(HaveOccurred())

		err = listeningCheck.WriteSSRawFiles(ssOutTCP, ssOutUDP)
		Expect(err).ToNot(HaveOccurred())

		err = ssMat.WriteMatrixToFileByType(utilsHelpers, "ss-generated-matrix", types.FormatCSV, deployment, artifactsDir)
		Expect(err).ToNot(HaveOccurred())

		// generate the diff matrix between the enpointslice and the ss matrix
		ssFilteredMat, err := filterSSMatrix(ssMat)
		Expect(err).ToNot(HaveOccurred())

		diff := matrixdiff.Generate(commatrix, ssFilteredMat)
		diffStr, err := diff.String()
		Expect(err).ToNot(HaveOccurred())

		err = utilsHelpers.WriteFile(filepath.Join(artifactsDir, "matrix-diff-ss"), []byte(diffStr))
		Expect(err).ToNot(HaveOccurred())

		notUsedEPSMat := diff.GenerateUniquePrimary()
		if len(notUsedEPSMat.Matrix) > 0 {
			logrus.Warningf("the following ports are not used: \n %s", notUsedEPSMat)
		}

		// generate the diff matrix between the open ports to ignore matrix and the missing endpoint slice matrix (based on the diff between the enpointslice and the ss matrix)
		diffIgonre := matrixdiff.Generate(diff.GenerateUniqueSecondary(), portsToIgnoreCommatrix)
		missingEPSMat := diffIgonre.GenerateUniquePrimary()
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

// Filter ss known ports to skip in matrix diff.
func filterSSMatrix(mat *types.ComMatrix) (*types.ComMatrix, error) {
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
