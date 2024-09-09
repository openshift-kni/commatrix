package e2e

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"golang.org/x/sync/errgroup"

	corev1 "k8s.io/api/core/v1"

	"github.com/openshift-kni/commatrix/pkg/client"
	commatrixcreator "github.com/openshift-kni/commatrix/pkg/commatrix-creator"
	"github.com/openshift-kni/commatrix/pkg/consts"
	"github.com/openshift-kni/commatrix/pkg/endpointslices"
	"github.com/openshift-kni/commatrix/pkg/types"
	"github.com/openshift-kni/commatrix/pkg/utils"
	"github.com/openshift-kni/commatrix/test/pkg/firewall"
	node "github.com/openshift-kni/commatrix/test/pkg/node"
)

var (
	cs           *client.ClientSet
	matrix       *types.ComMatrix
	isSNO        bool
	utilsHelpers utils.UtilsInterface
	nodeList     *corev1.NodeList
	artifactsDir string
)

const (
	workerNodeRole = "worker"
	tableName      = "table inet openshift_filter"
	chainName      = "chain OPENSHIFT"
	portRange      = "\ntcp dport { 30000-32767, } accept\nudp dport { 30000-32767, } accept"
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

	deployment := types.Standard
	isSNO, err = utilsHelpers.IsSNOCluster()
	Expect(err).NotTo(HaveOccurred())

	if isSNO {
		deployment = types.SNO
	}

	infra := types.Cloud
	isBM, err := utilsHelpers.IsBMInfra()
	Expect(err).NotTo(HaveOccurred())

	if isBM {
		infra = types.Baremetal
	}

	epExporter, err := endpointslices.New(cs)
	Expect(err).ToNot(HaveOccurred())

	By("Generating comMatrix")
	commMatrix, err := commatrixcreator.New(epExporter, "", "", infra, deployment)
	Expect(err).NotTo(HaveOccurred())

	matrix, err = commMatrix.CreateEndpointMatrix()
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
	It("should apply firewall by blocking all ports except the ones OCP is listening on", func() {
		masterMat, workerMat := matrix.SeparateMatrixByRole()
		var workerNFT []byte

		By("Creating NFT output for each role")
		masterNFT, err := masterMat.ToNFTables()
		Expect(err).NotTo(HaveOccurred())
		// add the k8s port range
		masterNFT = addRange(masterNFT)

		if !isSNO {
			workerNFT, err = workerMat.ToNFTables()
			Expect(err).NotTo(HaveOccurred())
			// add the k8s port range
			workerNFT = addRange(workerNFT)
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

func addRange(matrixNFT []byte) []byte {
	matrixNFTStr := string(matrixNFT)
	re := regexp.MustCompile(`udp dport.*accept`)
	loc := re.FindStringIndex(matrixNFTStr)

	if loc != nil {
		matrixNFTStr = matrixNFTStr[:loc[1]] + portRange + matrixNFTStr[loc[1]:]
	}

	matrixNFT = []byte(matrixNFTStr)
	return matrixNFT
}
