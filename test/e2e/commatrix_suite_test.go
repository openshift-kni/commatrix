package e2e

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"

	configv1 "github.com/openshift/api/config/v1"
	corev1 "k8s.io/api/core/v1"
	clientOptions "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift-kni/commatrix/pkg/client"
	commatrixcreator "github.com/openshift-kni/commatrix/pkg/commatrix-creator"
	"github.com/openshift-kni/commatrix/pkg/consts"
	"github.com/openshift-kni/commatrix/pkg/endpointslices"
	listeningsockets "github.com/openshift-kni/commatrix/pkg/listening-sockets"
	matrixdiff "github.com/openshift-kni/commatrix/pkg/matrix-diff"
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
	deployment   types.Deployment
)

const (
	workerNodeRole     = "worker"
	tableName          = "table inet openshift_filter"
	chainName          = "chain OPENSHIFT"
	testNS             = "openshift-commatrix-test"
	serviceNodePortMin = 30000
	serviceNodePortMax = 32767
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

	By("Creating test namespace")
	err = utilsHelpers.CreateNamespace(testNS)
	Expect(err).ToNot(HaveOccurred())

	nodeList = &corev1.NodeList{}
	err = cs.List(context.TODO(), nodeList)
	Expect(err).ToNot(HaveOccurred())
})

var _ = AfterSuite(func() {
	By("Deleting Namespace")
	err := utilsHelpers.DeleteNamespace(testNS)
	Expect(err).ToNot(HaveOccurred())
})

var _ = Describe("commatrix", func() {
	It("should validate the communication matrix ports match the node's listening ports", func() {
		err := matrix.WriteMatrixToFileByType(utilsHelpers, "communication-matrix", types.FormatCSV, deployment, artifactsDir)
		Expect(err).ToNot(HaveOccurred())

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

		diff := matrixdiff.Generate(matrix, ssFilteredMat)
		diffStr, err := diff.String()
		Expect(err).ToNot(HaveOccurred())

		err = utilsHelpers.WriteFile(filepath.Join(artifactsDir, "matrix-diff-ss"), []byte(diffStr))
		Expect(err).ToNot(HaveOccurred())

		notUsedEPSMat := diff.GenerateUniquePrimary()
		if len(notUsedEPSMat.Matrix) > 0 {
			logrus.Warningf("the following ports are not used: \n %s", notUsedEPSMat)
		}

		missingEPSMat := diff.GenerateUniqueSecondary()
		if len(missingEPSMat.Matrix) > 0 {
			err := fmt.Errorf("the following ports are used but don't have an endpointslice: \n %s", missingEPSMat)
			Expect(err).ToNot(HaveOccurred())
		}
	})

	It("should apply firewall by blocking all ports except the ones OCP is listening on", func() {
		masterMat, workerMat := matrix.SeparateMatrixByRole()
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
				err := firewall.ApplyRulesToNode(nftTable, nodeName, testNS, artifactsDir, utilsHelpers)
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

		err = node.SoftRebootNodeAndWaitForDisconnect(utilsHelpers, cs, nodeName, testNS)
		Expect(err).ToNot(HaveOccurred())

		By("Waiting for node to be ready")
		node.WaitForNodeReady(nodeName, cs)

		debugPod, err := utilsHelpers.CreatePodOnNode(nodeName, testNS, consts.DefaultDebugPodImage)
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

		res = append(res, cd)
	}

	return &types.ComMatrix{Matrix: res}, nil
}
