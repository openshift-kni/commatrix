package e2e

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/openshift-kni/commatrix/pkg/consts"
	"github.com/openshift-kni/commatrix/pkg/types"
	"github.com/openshift-kni/commatrix/test/pkg/cluster"
	"github.com/openshift-kni/commatrix/test/pkg/firewall"
	"github.com/openshift-kni/commatrix/test/pkg/node"
	corev1 "k8s.io/api/core/v1"
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

				roleBasedMat := masterMat
				extraNftablesFileEnv := "EXTRA_NFTABLES_MASTER_FILE"
				if role == workerNodeRole {
					roleBasedMat = workerMat
					extraNftablesFileEnv = "EXTRA_NFTABLES_WORKER_FILE"
				}

				nftablesConfig, err = roleBasedMat.ToNFTables()
				Expect(err).NotTo(HaveOccurred())

				extraNFTablesFile, _ := os.LookupEnv(extraNftablesFileEnv)
				if extraNFTablesFile != "" {
					nftablesConfig, err = AddPortsToNFTables(nftablesConfig, extraNFTablesFile)
					Expect(err).NotTo(HaveOccurred())
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

		By("Rebooting first node: " + nodeName + "and waiting for disconnect \n")
		err = node.SoftRebootNodeAndWaitForDisconnect(utilsHelpers, cs, nodeName, testNS, isSNO)
		Expect(err).ToNot(HaveOccurred())

		By("Waiting for node to be ready")
		node.WaitForNodeReady(nodeName, cs)

		By("Listing nftables rules")
		command := []string{
			"chroot", "/host", "/bin/bash", "-c", "nft list ruleset",
		}

		debugPod, err := utilsHelpers.CreatePodOnNode(nodeName, testNS,
			consts.DefaultDebugPodImage, command)
		Expect(err).ToNot(HaveOccurred())

		err = utilsHelpers.WaitForPodStatus(testNS, debugPod, corev1.PodSucceeded)
		Expect(err).ToNot(HaveOccurred())

		podLogs, err := utilsHelpers.GetPodLogs(testNS, debugPod)
		Expect(err).ToNot(HaveOccurred())

		defer func() {
			err := utilsHelpers.DeletePod(debugPod)
			Expect(err).ToNot(HaveOccurred())
		}()

		err = utilsHelpers.WriteFile(filepath.Join(artifactsDir, "nftables-after-reboot-"+nodeName), []byte(podLogs))
		Expect(err).ToNot(HaveOccurred())

		By("Checking if nftables contain the chain OPENSHIFT")
		if strings.Contains(podLogs, tableName) &&
			strings.Contains(podLogs, chainName) {
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
