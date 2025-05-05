package e2e

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
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
	workerNodeRole     = "worker"
	tableName          = "table inet openshift_filter"
	chainName          = "chain OPENSHIFT"
	workerNFTFile      = "communication-matrix-worker.nft"
	masterNFTFile      = "communication-matrix-master.nft"
	masterNFTconfig    []byte
	workerNFTconfig    []byte
	nodeRoleToNFTables map[string][]byte
)

var _ = Describe("Nftables", func() {
	BeforeEach(func() {
		By("Creating test namespace")
		err := utilsHelpers.CreateNamespace(testNS)
		Expect(err).ToNot(HaveOccurred())

		nodeList = &corev1.NodeList{}
		err = cs.List(context.TODO(), nodeList)
		Expect(err).ToNot(HaveOccurred())

		By("Generating nft communication matrix using oc command")
		cmd := exec.Command("oc", "commatrix", "generate", "--format", "nft", "--destDir", artifactsDir, "--platform-type", platformType)
		err = cmd.Run()
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to run command: %s", cmd.String()))

		By("Reading the generated commatrix nft files")
		masterFilePath := filepath.Join(artifactsDir, masterNFTFile)
		masterNFTconfig, err = os.ReadFile(masterFilePath)
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to read generated %s file", masterNFTFile))

		// creare the nodeRoleToNFTables map[string][]byte
		By("Creating NFT output for each role")
		nodeRoleToNFTables = make(map[string][]byte)
		for _, node := range nodeList.Items {
			role, err := types.GetNodeRole(&node)
			Expect(err).NotTo(HaveOccurred())

			if _, exists := nodeRoleToNFTables[role]; !exists {
				var nftablesConfig []byte

				nftablesConfig = masterNFTconfig
				extraNftablesFileEnv := "EXTRA_NFTABLES_MASTER_FILE"
				if role == workerNodeRole {
					workerFilePath := filepath.Join(artifactsDir, workerNFTFile)
					workerNFTconfig, err = os.ReadFile(workerFilePath)
					Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to read generated %s file", workerNFTFile))

					nftablesConfig = workerNFTconfig
					extraNftablesFileEnv = "EXTRA_NFTABLES_WORKER_FILE"
				}

				extraNFTablesFile, _ := os.LookupEnv(extraNftablesFileEnv)
				if extraNFTablesFile != "" {
					nftablesConfig, err = AddPortsToNFTables(nftablesConfig, extraNFTablesFile)
					Expect(err).NotTo(HaveOccurred())
				}

				nodeRoleToNFTables[role] = nftablesConfig
			}
		}
	})

	AfterEach(func() {
		By("Deleting Namespace")
		err := utilsHelpers.DeleteNamespace(testNS)
		Expect(err).ToNot(HaveOccurred())
	})

	It("should apply firewall by blocking all ports except the ones OCP is listening on", func() {
		err := cluster.ValidateClusterVersionAndMachineConfiguration(cs)
		Expect(err).ToNot(HaveOccurred())

		for role, nftablesConfig := range nodeRoleToNFTables {
			By(fmt.Sprintf("Applying firewall on %s nodes", role))

			machineConfig, err := firewall.CreateMachineConfig(cs, nftablesConfig, artifactsDir,
				role, utilsHelpers)
			Expect(err).ToNot(HaveOccurred())

			updated, err := cluster.ApplyMachineConfig(machineConfig, cs)
			Expect(err).ToNot(HaveOccurred())

			if updated {
				// wait to MCP to start the update.
				cluster.WaitForMCPUpdateToStart(cs, role)

				// Wait for MCP update to be ready.
				cluster.WaitForMCPReadyState(cs, role)

				log.Println("MCP update completed successfully.")
			} else {
				log.Println("No update needed. MCP update skipped.")
			}
		}

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
