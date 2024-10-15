package e2e

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

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
		versionMajorMinor, err := utilsHelpers.GetClusterVersiona()
		Expect(err).ToNot(HaveOccurred())

		if firewall.IsVersionGreaterThan(versionMajorMinor, "4.16") {
			if err = firewall.UpdateMachineConfiguration(cs); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				Expect(err).ToNot(HaveOccurred())
			}
		}
		Expect(err).ToNot(HaveOccurred())
		for _, role := range nodeRoles {
			noderole := role
			g.Go(func() error {
				By(fmt.Sprintf("Applying firewall on %s nodes", noderole))
				nftTable := masterNFT
				if noderole == workerNodeRole {
					nftTable = workerNFT
				}
				err := firewall.MachineconfigWay(cs, nftTable, artifactsDir, noderole, utilsHelpers)
				if err != nil {
					return err
				}
				return nil
			})
		}

		// Wait for all goroutines to finish
		err = g.Wait()
		Expect(err).ToNot(HaveOccurred())
		waitDuration := 1 * time.Minute
		fmt.Printf("Waiting for %s after applying MachineConfiguration...\n", waitDuration)
		time.Sleep(waitDuration)
		err = firewall.WaitForMCPReady(cs, 20*time.Minute)
		Expect(err).ToNot(HaveOccurred())

		g = new(errgroup.Group)
		nodeName := nodeList.Items[0].Name
		for _, node := range nodeList.Items {
			nodename := node.Name
			g.Go(func() error {
				By("Waiting for node to be ready " + nodename)
				testnode.WaitForNodeReady(nodename, cs)
				return nil
			})
		}

		err = g.Wait()
		Expect(err).ToNot(HaveOccurred())

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
