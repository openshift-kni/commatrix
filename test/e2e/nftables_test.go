package e2e

import (
	"fmt"
	"log"
	"os"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/openshift-kni/commatrix/pkg/consts"
	"github.com/openshift-kni/commatrix/pkg/types"
	"github.com/openshift-kni/commatrix/test/pkg/cluster"
	"github.com/openshift-kni/commatrix/test/pkg/firewall"
)

var (
	workerNodeRole = "worker"
	tableName      = "table inet openshift_filter"
	chainName      = "chain OPENSHIFT"
)

var _ = Describe("Nftables", func() {
	It("should apply firewall by blocking all ports except the ones OCP is listening on", func() {
		masterMat, workerMat := commatrix.SeparateMatrixByRole()
		nodeRoleToNFTables := make(map[string][]byte)

		By("Creating NFT output for each role")
		for _, node := range nodeList.Items {
			role, err := types.GetNodeRole(&node)
			Expect(err).NotTo(HaveOccurred())

			if _, exists := nodeRoleToNFTables[role]; !exists {
				var nftablesConfig []byte

				if role == workerNodeRole {
					nftablesConfig, err = workerMat.ToNFTables()
					Expect(err).NotTo(HaveOccurred())

					if extraNFTablesWorkerFile != "" {
						nftablesConfig, err = AddPortsToNFTables(nftablesConfig, extraNFTablesWorkerFile)
						Expect(err).NotTo(HaveOccurred())
					}
				} else {
					nftablesConfig, err = masterMat.ToNFTables()
					Expect(err).NotTo(HaveOccurred())

					if extraNFTablesMasterFile != "" {
						nftablesConfig, err = AddPortsToNFTables(nftablesConfig, extraNFTablesMasterFile)
						Expect(err).NotTo(HaveOccurred())
					}
				}

				nodeRoleToNFTables[role] = nftablesConfig
			}
		}

		err := cluster.ValidateClusterVersionAndMachineConfiguration(cs)
		Expect(err).ToNot(HaveOccurred())

		for role, nftablesConfig := range nodeRoleToNFTables {
			By(fmt.Sprintf("Applying firewall on %s nodes", role))

			machineConfig, err := firewall.CreateMachineConfig(cs, nftablesConfig, artifactsDir,
				role, utilsHelpers)
			Expect(err).ToNot(HaveOccurred())

			err = cluster.ApplyMachineConfig(machineConfig, cs)
			Expect(err).ToNot(HaveOccurred())

		}

		// waiting for mcp start updating
		cluster.WaitForMCPUpdateToStart(cs)

		// waiting for MCP to finish updating
		cluster.WaitForMCPReadyState(cs)

		nodeName := nodeList.Items[0].Name

		debugPod, err := utilsHelpers.CreatePodOnNode(nodeName, testNS, consts.DefaultDebugPodImage)
		Expect(err).ToNot(HaveOccurred())

		defer func() {
			err := utilsHelpers.DeletePod(debugPod)
			Expect(err).ToNot(HaveOccurred())
		}()

		By("Listing nftables rules")
		output, err := firewall.NftListAndWriteToFile(debugPod, utilsHelpers, artifactsDir, "nftables-after-reboot-"+nodeName)
		Expect(err).ToNot(HaveOccurred())

		By("Checking if nftables contain the chain OPENSHIFT")
		if strings.Contains(string(output), tableName) &&
			strings.Contains(string(output), chainName) {
			log.Println("OPENSHIFT chain found in nftables.")
		} else {
			Fail("OPENSHIFT chain not found in nftables")
		}
	})
})

func AddPortsToNFTables(nftables []byte, extraNFTablesFile string) ([]byte, error) {
	nftStr := string(nftables)

	insertPoint := "# Logging and default drop"
	if !strings.Contains(nftStr, insertPoint) {
		return nftables, fmt.Errorf("insert point not found in nftables configuration")
	}

	extraNFTablesValue, err := os.ReadFile(extraNFTablesFile)
	if err != nil {
		return nftables, fmt.Errorf("failed to read extra nftables from file: %v", err)
	}

	nftStr = strings.Replace(nftStr, insertPoint, string(extraNFTablesValue)+"\n"+insertPoint, 1)

	return []byte(nftStr), nil
}
