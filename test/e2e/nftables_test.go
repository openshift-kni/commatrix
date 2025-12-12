package e2e

import (
	"bytes"
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
	"github.com/openshift-kni/commatrix/test/pkg/cluster"
	"github.com/openshift-kni/commatrix/test/pkg/firewall"
	"github.com/openshift-kni/commatrix/test/pkg/node"
	corev1 "k8s.io/api/core/v1"
)

var (
	imageAPIGroupVersion = "image.openshift.io/v1"
	tableName            = "table inet openshift_filter"
	chainName            = "chain OPENSHIFT"
	poolToNFTables       map[string][]byte
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
		cmd := exec.Command("oc", "commatrix", "generate", "--format", "nft", "--destDir", artifactsDir)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err = cmd.Run()
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf(
			"Failed to run command: %s\nstdout:\n%s\nstderr:\n%s",
			cmd.String(), stdout.String(), stderr.String(),
		))

		By("Reading the generated commatrix nft files per pool")
		poolToNFTables = make(map[string][]byte)
		entries, err := os.ReadDir(artifactsDir)
		Expect(err).ToNot(HaveOccurred())
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if !strings.HasPrefix(name, consts.CommatrixFileNamePrefix+"-") || !strings.HasSuffix(name, ".nft") {
				continue
			}
			base := strings.TrimSuffix(name, ".nft")
			pool := strings.TrimPrefix(base, consts.CommatrixFileNamePrefix+"-")
			content, readErr := os.ReadFile(filepath.Join(artifactsDir, name))
			Expect(readErr).ToNot(HaveOccurred(), fmt.Sprintf("Failed to read generated %s file", name))

			upperPool := strings.NewReplacer("-", "_", ".", "_", ":", "_").Replace(strings.ToUpper(pool))
			extraEnv := fmt.Sprintf("EXTRA_NFTABLES_%s_FILE", upperPool)
			if extraPath, ok := os.LookupEnv(extraEnv); ok && extraPath != "" {
				content, err = AddPortsToNFTables(content, extraPath)
				Expect(err).NotTo(HaveOccurred())
			}

			poolToNFTables[pool] = content
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

		for pool, nftablesConfig := range poolToNFTables {
			By(fmt.Sprintf("Applying firewall on pool %s", pool))

			machineConfig, err := firewall.CreateMachineConfig(cs, nftablesConfig, artifactsDir,
				pool, utilsHelpers)
			Expect(err).ToNot(HaveOccurred())

			updated, err := cluster.ApplyMachineConfig(machineConfig, cs)
			Expect(err).ToNot(HaveOccurred())

			if updated {
				// wait to MCP to start the update.
				cluster.WaitForMCPUpdateToStart(cs, pool)

				// Wait for MCP update to be ready.
				cluster.WaitForMCPReadyState(cs, pool)

				log.Println("MCP update completed successfully.")
			} else {
				log.Println("No update needed. MCP update skipped.")
			}
		}

		nodeName := nodeList.Items[0].Name
		By("Rebooting first node: " + nodeName + "and waiting for disconnect \n")
		err = node.SoftRebootNodeAndWaitForDisconnect(utilsHelpers, cs, nodeName, testNS, controlPlaneTopology)
		Expect(err).ToNot(HaveOccurred())

		By("Waiting for node to be ready")
		node.WaitForNodeReady(nodeName, cs)

		By("Waiting for image API to be available")
		cluster.WaitForAPIGroupAvailable(cs, imageAPIGroupVersion)

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
