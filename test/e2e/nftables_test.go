package e2e

import (
	"fmt"
	"log"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"golang.org/x/sync/errgroup"

	"github.com/openshift-kni/commatrix/pkg/consts"

	"github.com/openshift-kni/commatrix/test/pkg/firewall"
	testnode "github.com/openshift-kni/commatrix/test/pkg/node"
)

var (
	workerNodeRole = "worker"
	tableName      = "table inet openshift_filter"
	chainName      = "chain OPENSHIFT"
)

var _ = Describe("Nftables", func() {
	It("should apply firewall by blocking all ports except the ones OCP is listening on", func() {
		masterMat, workerMat := commatrix.SeparateMatrixByRole()
		var workerNFT []byte
		nodeRoles := []string{"master"}
		By("Creating NFT output for each role")
		masterNFT, err := masterMat.ToNFTables()
		Expect(err).NotTo(HaveOccurred())
		if !isSNO {
			nodeRoles = append(nodeRoles, "worker")
			workerNFT, err = workerMat.ToNFTables()
			Expect(err).NotTo(HaveOccurred())
		}
		g := new(errgroup.Group)
		Expect(err).ToNot(HaveOccurred())
		for _, role := range nodeRoles {
			g.Go(func() error {
				By(fmt.Sprintf("Applying firewall on %s nodes", role))
				nftTable := masterNFT
				if role == workerNodeRole {
					nftTable = workerNFT
				}
				err := firewall.MachineconfigWay(nftTable, artifactsDir, role, utilsHelpers)
				if err != nil {
					return err
				}
				return nil
			})
		}

		// Wait for all goroutines to finish
		err = g.Wait()
		Expect(err).ToNot(HaveOccurred())

		g = new(errgroup.Group)
		nodeName := nodeList.Items[0].Name
		for _, node := range nodeList.Items {
			nodeName = node.Name
			g.Go(func() error {
				By("Waiting for node to be ready " + nodeName)
				testnode.WaitForNodeReady(nodeName, cs)
				return nil
			})
		}
		err = g.Wait()
		Expect(err).ToNot(HaveOccurred())
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
