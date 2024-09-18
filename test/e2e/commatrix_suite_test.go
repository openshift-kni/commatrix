package e2e

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"

	configv1client "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift-kni/commatrix/pkg/client"
	commatrixcreator "github.com/openshift-kni/commatrix/pkg/commatrix-creator"
	"github.com/openshift-kni/commatrix/pkg/consts"
	"github.com/openshift-kni/commatrix/pkg/endpointslices"
	matrixdiff "github.com/openshift-kni/commatrix/pkg/matrix-diff"
	"github.com/openshift-kni/commatrix/pkg/types"
	"github.com/openshift-kni/commatrix/pkg/utils"
	"github.com/openshift-kni/commatrix/test/pkg/firewall"
	node "github.com/openshift-kni/commatrix/test/pkg/node"
)

var (
	cs           *client.ClientSet
	commatrix    *types.ComMatrix
	isSNO        bool
	isBM         bool
	deployment   types.Deployment
	infra        types.Env
	utilsHelpers utils.UtilsInterface
	epExporter   *endpointslices.EndpointSlicesExporter
	nodeList     *corev1.NodeList
	artifactsDir string
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
	workerNodeRole = "worker"
	tableName      = "table inet openshift_filter"
	chainName      = "chain OPENSHIFT"
)

var _ = BeforeSuite(func() {
	By("Creating output folder")
	artifactsDir = os.Getenv("ARTIFACT_DIR")
	if artifactsDir == "" {
		log.Println("env var ARTIFACT_DIR is not set, using default value")
	}
	artifactsDir = filepath.Join(artifactsDir, "commatrix-e2e")

	err := os.MkdirAll(artifactsDir, 0755)
	Expect(err).NotTo(HaveOccurred())

	By("Creating the clients for the Generating step")
	cs, err = client.New()
	Expect(err).NotTo(HaveOccurred())

	utilsHelpers = utils.New(cs)

	deployment = types.Standard
	isSNO, err = utilsHelpers.IsSNOCluster()
	Expect(err).NotTo(HaveOccurred())

	if isSNO {
		deployment = types.SNO
	}

	infra = types.Cloud
	isBM, err = utilsHelpers.IsBMInfra()
	Expect(err).NotTo(HaveOccurred())

	if isBM {
		infra = types.Baremetal
	}

	epExporter, err = endpointslices.New(cs)
	Expect(err).ToNot(HaveOccurred())

	By("Generating comMatrix")
	commMatrixCreator, err := commatrixcreator.New(epExporter, "", "", infra, deployment)
	Expect(err).NotTo(HaveOccurred())

	commatrix, err = commMatrixCreator.CreateEndpointMatrix()
	Expect(err).NotTo(HaveOccurred())

	By("Creating Namespace")
	err = utilsHelpers.CreateNamespace(consts.DefaultDebugNamespace)
	Expect(err).ToNot(HaveOccurred())

	nodeList = &corev1.NodeList{}
	err = cs.List(context.TODO(), nodeList)
	Expect(err).ToNot(HaveOccurred())
})

var _ = AfterSuite(func() {
	By("Deleting Namespace")
	err := utilsHelpers.DeleteNamespace(consts.DefaultDebugNamespace)
	Expect(err).ToNot(HaveOccurred())
})

var _ = Describe("commatrix", func() {
	It("generated communication matrix should be equal to documented communication matrix", func() {
		By("get cluster's version and check if it's suitable for test")
		clusterVersion, err := getClusterVersion(cs)
		Expect(err).NotTo(HaveOccurred())
		floatClusterVersion, err := strconv.ParseFloat(clusterVersion, 64)
		Expect(err).ToNot(HaveOccurred())

		if floatClusterVersion < minimalDocCommatrixVersion {
			Skip(fmt.Sprintf("If the cluster version is lower than the lowest version that "+
				"has a documented communication matrix (%v), skip test", minimalDocCommatrixVersion))
		}

		By("write commatrix to artifact file")
		err = commatrix.WriteMatrixToFileByType(utilsHelpers, "new-commatrix", types.FormatCSV, deployment, artifactsDir)
		Expect(err).ToNot(HaveOccurred())

		By(fmt.Sprintf("get documented commatrix version %s", clusterVersion))
		// get documented commatrix from URL
		resp, err := http.Get(strings.Replace(docCommatrixBaseURL, "VERSION", clusterVersion, 1))
		Expect(err).ToNot(HaveOccurred())
		defer resp.Body.Close()
		// if response status code equals to "status not found", compare generated commatrix to the master documented commatrix
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
			logrus.Warningf("the following ports are documented but are not used: \n %s", notUsedPortsMat)
		}

		// Get ports that are in the generated commatrix but not in the documented commatrix.
		missingPortsMat := diff.GenerateUniquePrimary()
		if len(missingPortsMat.Matrix) > 0 {
			err := fmt.Errorf("the following ports are used but are not documented: \n %s", missingPortsMat)
			Expect(err).ToNot(HaveOccurred())
		}
	})

	It("should apply firewall by blocking all ports except the ones OCP is listening on", func() {
		masterMat, workerMat := commatrix.SeparateMatrixByRole()
		var workerNFT []byte

		By("Creating NFT output for each role")
		masterNFT, err := masterMat.ToNFTables()
		Expect(err).NotTo(HaveOccurred())
		if !isSNO {
			workerNFT, err = workerMat.ToNFTables()
			Expect(err).NotTo(HaveOccurred())
		}

		g := new(errgroup.Group)
		for _, node := range nodeList.Items {
			nodeName := node.Name
			nodeRole, err := types.GetNodeRole(&node)
			Expect(err).ToNot(HaveOccurred())
			g.Go(func() error {
				nftTable := masterNFT
				if nodeRole == workerNodeRole {
					nftTable = workerNFT
				}

				By("Applying firewall on node: " + nodeName)
				err := firewall.ApplyRulesToNode(nftTable, nodeName, artifactsDir, utilsHelpers)
				if err != nil {
					return err
				}
				return nil
			})
		}

		// Wait for all goroutines to finish
		err = g.Wait()
		Expect(err).ToNot(HaveOccurred())
		nodeName := nodeList.Items[0].Name

		By("Rebooting first node: " + nodeName + "and waiting for disconnect")

		err = node.SoftRebootNodeAndWaitForDisconnect(utilsHelpers, cs, nodeName)
		Expect(err).ToNot(HaveOccurred())

		By("Waiting for node to be ready")
		node.WaitForNodeReady(nodeName, cs)

		debugPod, err := utilsHelpers.CreatePodOnNode(nodeName, consts.DefaultDebugNamespace, consts.DefaultDebugPodImage)
		Expect(err).ToNot(HaveOccurred())

		defer func() {
			err := utilsHelpers.DeletePod(debugPod)
			Expect(err).ToNot(HaveOccurred())
		}()

		By("Listing nftables rules after reboot")
		output, err := firewall.NftListAndWriteToFile(debugPod, utilsHelpers, artifactsDir, "nftables-after-reboot-"+nodeName)
		Expect(err).ToNot(HaveOccurred())

		By("Checking if nftables contain the chain OPENSHIFT after reboot")
		if strings.Contains(string(output), tableName) &&
			strings.Contains(string(output), chainName) {
			log.Println("OPENSHIFT chain found in nftables.")
		} else {
			Fail("OPENSHIFT chain not found in nftables")
		}
	})
})

// getClusterVersion return cluster's Y stream version.
func getClusterVersion(cs *client.ClientSet) (string, error) {
	configClient := configv1client.NewForConfigOrDie(cs.Config)
	clusterVersion, err := configClient.ClusterVersions().Get(context.Background(), "version", metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	clusterVersionParts := strings.SplitN(clusterVersion.Status.Desired.Version, ".", 3)
	return strings.Join(clusterVersionParts[:2], "."), nil
}

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
