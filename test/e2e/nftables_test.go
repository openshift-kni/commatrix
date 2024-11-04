package e2e

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/Masterminds/semver/v3"
	"golang.org/x/sync/errgroup"

	"github.com/openshift-kni/commatrix/pkg/client"
	"github.com/openshift-kni/commatrix/pkg/consts"
	"github.com/openshift-kni/commatrix/test/pkg/firewall"

	testnode "github.com/openshift-kni/commatrix/test/pkg/node"
	machineconfigurationv1 "github.com/openshift/api/machineconfiguration/v1"
	ocpoperatorv1 "github.com/openshift/api/operator/v1"
	mcoac "github.com/openshift/client-go/operator/applyconfigurations/operator/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	controllersClient "sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	workerNodeRole = "worker"
	tableName      = "table inet openshift_filter"
	chainName      = "chain OPENSHIFT"
)

const (
	waitDuration = 2 * time.Minute
	mcTimeout    = 20 * time.Minute
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

			if extraNFTablesFile != "" {
				workerNFT, err = AddPortsToNFTables(workerNFT, extraNFTablesFile)
				Expect(err).NotTo(HaveOccurred())
			}
		}

		if extraNFTablesFile != "" {
			masterNFT, err = AddPortsToNFTables(masterNFT, extraNFTablesFile)
			Expect(err).NotTo(HaveOccurred())
		}
		g := new(errgroup.Group)
		clusterVersion, err := utilsHelpers.GetClusterVersion()
		Expect(err).ToNot(HaveOccurred())

		currentVersion, err := semver.NewVersion(clusterVersion)
		Expect(err).NotTo(HaveOccurred())

		thresholdVersion := semver.MustParse("4.16")
		if currentVersion.GreaterThan(thresholdVersion) { // if version more than 4.16 need to change cluster MachineConfiguration.
			By("Version Greater Than 4.16 Need To Update Machine Configuration")
			if err = updateMachineConfiguration(cs); err != nil {
				Expect(err).ToNot(HaveOccurred())
			}
		}

		for _, role := range nodeRoles {
			nodeRole := role
			g.Go(func() error {
				By(fmt.Sprintf("Applying firewall on %s nodes", nodeRole))
				nftTable := masterNFT
				if nodeRole == workerNodeRole {
					nftTable = workerNFT
				}
				err := firewall.Apply(cs, nftTable, artifactsDir, nodeRole, clusterVersion, utilsHelpers)
				if err != nil {
					return err
				}
				return nil
			})
		}

		// Wait for all goroutines to finish
		err = g.Wait()
		Expect(err).ToNot(HaveOccurred())

		fmt.Printf("Waiting for %s after applying MachineConfiguration...\n", waitDuration)
		time.Sleep(waitDuration)
		waitForMCPReady(cs, mcTimeout)

		g = new(errgroup.Group)
		for _, node := range nodeList.Items {
			nodeName := node.Name
			g.Go(func() error {
				By("Waiting for node to be ready " + nodeName)
				testnode.WaitForNodeReady(nodeName, cs)
				return nil
			})
		}

		err = g.Wait()
		Expect(err).ToNot(HaveOccurred())
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

func updateMachineConfiguration(cs *client.ClientSet) error {
	machineConfigurationClient := cs.MCInterface
	reloadApplyConfiguration := mcoac.ReloadService().WithServiceName("nftables.service")
	restartApplyConfiguration := mcoac.RestartService().WithServiceName("nftables.service")

	serviceName := "nftables.service"
	serviceApplyConfiguration := mcoac.NodeDisruptionPolicySpecUnit().WithName(ocpoperatorv1.NodeDisruptionPolicyServiceName(serviceName)).WithActions(
		mcoac.NodeDisruptionPolicySpecAction().WithType(ocpoperatorv1.ReloadSpecAction).WithReload(reloadApplyConfiguration),
	)
	fileApplyConfiguration := mcoac.NodeDisruptionPolicySpecFile().WithPath("/etc/sysconfig/nftables.conf").WithActions(
		mcoac.NodeDisruptionPolicySpecAction().WithType(ocpoperatorv1.RestartSpecAction).WithRestart(restartApplyConfiguration),
	)

	applyConfiguration := mcoac.MachineConfiguration("cluster").WithSpec(mcoac.MachineConfigurationSpec().
		WithManagementState("Managed").WithNodeDisruptionPolicy(mcoac.NodeDisruptionPolicyConfig().
		WithUnits(serviceApplyConfiguration).WithFiles(fileApplyConfiguration)))

	_, err := machineConfigurationClient.OperatorV1().MachineConfigurations().Apply(context.TODO(), applyConfiguration,
		metav1.ApplyOptions{FieldManager: "machine-config-operator", Force: true})
	if err != nil {
		return fmt.Errorf("updating cluster node disruption policy failed %v", err)
	}

	fmt.Println("MachineConfiguration updated successfully!")
	return nil
}

func waitForMCPReady(c *client.ClientSet, timeout time.Duration) {
	checkMCPReady := func() (bool, error) {
		mcpList := &machineconfigurationv1.MachineConfigPoolList{}
		err := c.List(context.TODO(), mcpList, &controllersClient.ListOptions{})
		if err != nil {
			return false, fmt.Errorf("failed to list MachineConfigPools: %v", err)
		}

		allReady := true
		for _, mcp := range mcpList.Items {
			fmt.Printf("MCP: %s\n", mcp.Name)
			fmt.Printf("  MachineCount: %d\n", mcp.Status.MachineCount)
			fmt.Printf("  ReadyMachineCount: %d\n", mcp.Status.ReadyMachineCount)
			fmt.Printf("  UpdatedMachineCount: %d\n", mcp.Status.UpdatedMachineCount)
			fmt.Printf("  DegradedMachineCount: %d\n", mcp.Status.DegradedMachineCount)

			// Check if the MCP is not ready according to the required conditions
			if mcp.Status.ReadyMachineCount != mcp.Status.MachineCount ||
				mcp.Status.UpdatedMachineCount != mcp.Status.MachineCount ||
				mcp.Status.DegradedMachineCount != 0 {
				allReady = false
				fmt.Printf("  MCP %s is still updating or degraded\n", mcp.Name)
				break
			}
		}

		if allReady {
			fmt.Println("All MCPs are ready and updated")
		}

		return allReady, nil
	}

	Eventually(func() (bool, error) {
		return checkMCPReady()
	}, timeout, 30*time.Second).Should(BeTrue(), "Timed out waiting for MCPs to reach the desired state")
}

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

	// Append extra nftables values if provided
	newRules := ""
	if string(extraNFTablesValue) != "" {
		newRules = fmt.Sprintf("            %s\n", string(extraNFTablesValue))
	}

	nftStr = strings.Replace(nftStr, insertPoint, newRules+insertPoint, 1)

	return []byte(nftStr), nil
}
