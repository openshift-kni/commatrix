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

		var podsToIgnoreMasterMat, podsToIgnoreWorkerMat types.ComMatrix
		if portsToIgnoreCommatrix != nil {
			podsToIgnoreMasterMat, podsToIgnoreWorkerMat = portsToIgnoreCommatrix.SeparateMatrixByRole()
		}

		nodeRoleToNFTables := make(map[string][]byte)

		By("Creating NFT output for each role")
		for _, node := range nodeList.Items {
			role, err := types.GetNodeRole(&node)
			Expect(err).NotTo(HaveOccurred())

			var roleMat, podsToIgnoreMat types.ComMatrix
			var extraNftablesFileEnv string

			if _, exists := nodeRoleToNFTables[role]; !exists {
				if role == workerNodeRole {
					roleMat = workerMat
					podsToIgnoreMat = podsToIgnoreWorkerMat
					extraNftablesFileEnv = "EXTRA_NFTABLES_WORKER_FILE"
				} else {
					roleMat = masterMat
					podsToIgnoreMat = podsToIgnoreMasterMat
					extraNftablesFileEnv = "EXTRA_NFTABLES_MASTER_FILE"
				}

				nftablesRules := roleMat.ToNFTablesRules()
				Expect(err).NotTo(HaveOccurred())

				extraNFTablesFile, _ := os.LookupEnv(extraNftablesFileEnv)
				if extraNFTablesFile != "" {
					extraNFTablesValue, err := os.ReadFile(extraNFTablesFile)
					Expect(err).NotTo(HaveOccurred())
					nftablesRules += "\n\t\t\t\t\t" + string(extraNFTablesValue)
					Expect(err).NotTo(HaveOccurred())
				}

				if podsToIgnoreMat.Matrix != nil {
					nftablesRules += "\n\t\t\t\t\t" + podsToIgnoreMat.ToNFTablesRules()
				}
				nftableConfig := createNftableFromRules(nftablesRules)
				nodeRoleToNFTables[role] = []byte(nftableConfig)
			}
		}

		err := cluster.ValidateClusterVersionAndMachineConfiguration(cs)
		Expect(err).ToNot(HaveOccurred())

		for role, nftablesConfig := range nodeRoleToNFTables {
			By(fmt.Sprintf("Applying firewall on %s nodes", role))

			err = os.WriteFile(filepath.Join(artifactsDir, "config"), nftablesConfig, 0644)
			Expect(err).ToNot(HaveOccurred())

			machineConfig, err := firewall.CreateMachineConfig(cs, nftablesConfig, artifactsDir,
				role, utilsHelpers)
			Expect(err).ToNot(HaveOccurred())

			err = cluster.ApplyMachineConfig(machineConfig, cs)
			Expect(err).ToNot(HaveOccurred())

		}

		// waiting for mcp start updating
		// cluster.WaitForMCPUpdateToStart(cs)

		// waiting for MCP to finish updating
		cluster.WaitForMCPReadyState(cs)

		nodeName := nodeList.Items[0].Name

		By("Rebooting first node: " + nodeName + "and waiting for disconnect \n")
		err = node.SoftRebootNodeAndWaitForDisconnect(utilsHelpers, cs, nodeName, testNS)
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

func appendNftableRulesFromFile(nftablesRules string, extraNFTablesFile string) (string, error) {
	extraNFTablesValue, err := os.ReadFile(extraNFTablesFile)
	if err != nil {
		return "", fmt.Errorf("failed to read extra nftables from file: %v", err)
	}

	return nftablesRules + "\n" + string(extraNFTablesValue), nil
}

func createNftableFromRules(rules string) string {
	return fmt.Sprintf(`#!/usr/sbin/nft -f
      table inet openshift_filter {
          chain OPENSHIFT {
					type filter hook input priority 1; policy accept;
			
					# Allow loopback traffic
					iif lo accept
			
					# Allow established and related traffic
					ct state established,related accept
			
					# Allow ICMP on ipv4
					ip protocol icmp accept
					# Allow ICMP on ipv6
					ip6 nexthdr ipv6-icmp accept
			
					# Allow specific TCP and UDP ports
					%s
			
					# Logging and default drop
					log prefix "firewall " drop
				  }
			    }`, rules)
}
