package e2e

import (
	"fmt"
	"log"
	"os"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"golang.org/x/sync/errgroup"

	"github.com/openshift-kni/commatrix/pkg/consts"

	"github.com/openshift-kni/commatrix/pkg/types"
	"github.com/openshift-kni/commatrix/test/pkg/firewall"
	node "github.com/openshift-kni/commatrix/test/pkg/node"
)

var (
	workerNodeRole     = "worker"
	tableName          = "table inet openshift_filter"
	chainName          = "chain OPENSHIFT"
	extraNFTablesValue = getEnv("EXTRA_NFTABLES_VALUE", "")
)

func getEnv(envVar string, defaultVal string) string {
	val, exists := os.LookupEnv(envVar)
	if !exists {
		return defaultVal
	}
	return val
}

var _ = Describe("Nftables", func() {
	It("should apply firewall by blocking all ports except the ones OCP is listening on", func() {
		masterMat, workerMat := commatrix.SeparateMatrixByRole()
		var updatedMasterNFT, updatedworkerNFT, workerNFT []byte

		By("Creating NFT output for each role")
		masterNFT, err := masterMat.ToNFTables()
		Expect(err).NotTo(HaveOccurred())
		if !isSNO {
			workerNFT, err = workerMat.ToNFTables()
			Expect(err).NotTo(HaveOccurred())

			updatedworkerNFT, err = AddPortsToNFTables(workerNFT, extraNFTablesValue)
			Expect(err).NotTo(HaveOccurred())
		}

		updatedMasterNFT, err = AddPortsToNFTables(masterNFT, extraNFTablesValue)
		Expect(err).NotTo(HaveOccurred())

		g := new(errgroup.Group)
		for _, node := range nodeList.Items {
			nodeName := node.Name
			nodeRole, err := types.GetNodeRole(&node)
			Expect(err).ToNot(HaveOccurred())
			g.Go(func() error {
				nftTable := updatedMasterNFT
				if nodeRole == workerNodeRole {
					nftTable = updatedworkerNFT
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

func AddPortsToNFTables(nftables []byte, extraNFTablesValue string) ([]byte, error) {
	nftStr := string(nftables)

	insertPoint := "# Logging and default drop"
	if !strings.Contains(nftStr, insertPoint) {
		return nftables, fmt.Errorf("insert point not found in nftables configuration")
	}

	// Append extra nftables values if provided
	newRules := ""
	if extraNFTablesValue != "" {
		newRules = fmt.Sprintf("            %s\n", extraNFTablesValue)
	}

	nftStr = strings.Replace(nftStr, insertPoint, newRules+insertPoint, 1)

	return []byte(nftStr), nil
}
