package e2e

import (
	"context"
	"log"
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
	utilsTest "github.com/openshift-kni/commatrix/test/pkg/utils"
)

var (
	cs           *client.ClientSet
	matrix       *types.ComMatrix
	isSNO        bool
	utilsHelpers utils.UtilsInterface
	nodeList     *corev1.NodeList
)

var _ = BeforeSuite(func() {
	By("generating the commatrix")
	var err error
	cs, err = client.New()
	Expect(err).NotTo(HaveOccurred())

	deployment := types.Standard
	isSNO, err := utilsTest.IsSNOCluster(cs)
	Expect(err).NotTo(HaveOccurred())

	if isSNO {
		deployment = types.SNO
	}

	infra := types.Cloud
	isBM, err := utilsTest.IsBMInfra(cs)
	Expect(err).NotTo(HaveOccurred())

	if isBM {
		infra = types.Baremetal
	}

	epExporter, err := endpointslices.New(cs)
	Expect(err).ToNot(HaveOccurred())

	By("Create commMatrix")
	commMatrix, err := commatrixcreator.New(epExporter, "", "", infra, deployment)
	Expect(err).NotTo(HaveOccurred())

	matrix, err = commMatrix.CreateEndpointMatrix()
	Expect(err).NotTo(HaveOccurred())
	utilsHelpers = utils.New(cs)

	By("Create Namespace")
	err = utilsHelpers.CreateNamespace(consts.DefaultDebugNamespace)
	Expect(err).ToNot(HaveOccurred())

	nodeList = &corev1.NodeList{}
	err = cs.List(context.TODO(), nodeList)
	Expect(err).ToNot(HaveOccurred())
})

var _ = AfterSuite(func() {
	By("Delete Namespace")
	err := utilsHelpers.DeleteNamespace(consts.DefaultDebugNamespace)
	Expect(err).ToNot(HaveOccurred())
})

var _ = Describe("commatrix", func() {
	It("should apply firewall by blocking all ports except the ones OCP is listening on", func() {
		By("apply firewall on nodes")
		masterMat, workerMat := matrix.SeparateMatrixByRole()
		var workerNFT []byte

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
				var nftTable []byte
				if nodeRole == "master" {
					nftTable = masterNFT
				} else {
					nftTable = workerNFT
				}

				err := firewall.ApplyRulesToNode(nftTable, nodeName, utilsHelpers)
				if err != nil {
					return err
				}
				return nil
			})
		}

		// Wait for all goroutines to finish
		err = g.Wait()
		Expect(err).ToNot(HaveOccurred())

		By("reboot first node")

		debugPod, err := utilsHelpers.CreatePodOnNode(nodeList.Items[0].Name, consts.DefaultDebugNamespace, consts.DefaultDebugPodImage)
		Expect(err).ToNot(HaveOccurred())

		defer func() {
			err := utilsHelpers.DeletePod(debugPod)
			Expect(err).ToNot(HaveOccurred())
		}()

		err = node.SoftRebootNodeAndWaitForDisconnect(debugPod, cs)
		Expect(err).ToNot(HaveOccurred())

		node.WaitForNodeReady(nodeList.Items[0].Name, cs)

		output, err := firewall.RulesList(nodeList.Items[0].Name, utilsHelpers)
		Expect(err).ToNot(HaveOccurred())

		By("check if nftables contain the chain OPENSHIFT after reboot")
		if strings.Contains(string(output), "table inet openshift_filter") &&
			strings.Contains(string(output), "chain OPENSHIFT") {
			log.Println("The byte slices are identical.")
		} else {
			Fail("The byte slices are different")
		}
	})
})
