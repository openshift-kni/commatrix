package cluster

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/onsi/gomega"
	"github.com/openshift-kni/commatrix/pkg/client"

	machineconfigurationv1 "github.com/openshift/api/machineconfiguration/v1"
	ocpoperatorv1 "github.com/openshift/api/operator/v1"
	configv1client "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"
	mcoac "github.com/openshift/client-go/operator/applyconfigurations/operator/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sTypes "k8s.io/apimachinery/pkg/types"
	controllersClient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

const (
	thresholdVersion = "4.16" // above this version the nft service must be added to the NodeDisruptionPolicy in the MachineConfiguration.
	timeout          = 20 * time.Minute
)

// getClusterVersion return cluster's Y stream version.
func GetClusterVersion(cs *client.ClientSet) (string, error) {
	configClient := configv1client.NewForConfigOrDie(cs.Config)
	clusterVersion, err := configClient.ClusterVersions().Get(context.Background(), "version", metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	clusterVersionParts := strings.SplitN(clusterVersion.Status.Desired.Version, ".", 3)
	return strings.Join(clusterVersionParts[:2], "."), nil
}

func WaitForMCPUpdateToStart(cs *client.ClientSet) {
	gomega.Eventually(func() (bool, error) {
		mcpList := &machineconfigurationv1.MachineConfigPoolList{}
		err := cs.List(context.TODO(), mcpList, &controllersClient.ListOptions{})
		if err != nil {
			return false, fmt.Errorf("failed to list MachineConfigPools: %v", err)
		}

		for _, mcp := range mcpList.Items {
			if mcp.Status.UpdatedMachineCount != mcp.Status.MachineCount {
				log.Printf("MCP %s has started updating", mcp.Name)
				return true, nil // At least one MCP has started updating
			}
		}

		return false, nil
	}, timeout, 30*time.Second).Should(gomega.BeTrue(), "Timed out waiting for MCP to start updating")
}

func WaitForMCPReadyState(c *client.ClientSet) {
	gomega.Eventually(func() (bool, error) {
		mcpList := &machineconfigurationv1.MachineConfigPoolList{}
		err := c.List(context.TODO(), mcpList, &controllersClient.ListOptions{})
		if err != nil {
			return false, fmt.Errorf("failed to list MachineConfigPools: %v", err)
		}

		mcpsReady := true
		for _, mcp := range mcpList.Items {
			if mcp.Status.ReadyMachineCount != mcp.Status.MachineCount {
				mcpsReady = false
				log.Printf("MCP %s is still updating or degraded\n", mcp.Name)
				break
			}
		}

		if mcpsReady {
			log.Println("All MCPs are ready and updated")
		}

		return mcpsReady, nil
	}, timeout, 30*time.Second).Should(gomega.BeTrue(), "Timed out waiting for MCPs to reach the desired state")
}

func AddNFTSvcToNodeDisruptionPolicy(cs *client.ClientSet) error {
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

	log.Println("MachineConfiguration updated successfully!")
	return nil
}

func ApplyMachineConfig(yamlInput []byte, c *client.ClientSet) error {
	obj := &machineconfigurationv1.MachineConfig{}
	if err := yaml.Unmarshal(yamlInput, obj); err != nil {
		return fmt.Errorf("failed to unmarshal YAML: %w", err)
	}

	err := c.Create(context.TODO(), obj)
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create MachineConfig: %w", err)
		}

		// If it already exists, retrieve the current version to update
		existingMC := &machineconfigurationv1.MachineConfig{}
		if err = c.Get(context.TODO(), k8sTypes.NamespacedName{Name: obj.Name}, existingMC); err != nil {
			return fmt.Errorf("failed to get existing MachineConfig: %w", err)
		}

		obj.ResourceVersion = existingMC.ResourceVersion
		if updateErr := c.Update(context.TODO(), obj); updateErr != nil {
			return fmt.Errorf("failed to update MachineConfig: %w", updateErr)
		}
		log.Println("MachineConfig updated successfully.")
	} else {
		log.Println("MachineConfig created successfully.")
	}

	return nil
}

func ValidateClusterVersionAndMachineConfiguration(cs *client.ClientSet) (string, error) {
	thresholdVersionSemver := semver.MustParse(thresholdVersion)
	clusterVersion, err := GetClusterVersion(cs)
	if err != nil {
		return "", err
	}

	currentVersion, err := semver.NewVersion(clusterVersion)
	if err != nil {
		return "", err
	}

	if currentVersion.GreaterThan(thresholdVersionSemver) {
		log.Printf("Version Greater Than " + thresholdVersion + " - Updating Machine Configuration")
		err = AddNFTSvcToNodeDisruptionPolicy(cs)
		if err != nil {
			return "", err
		}
	}
	return clusterVersion, nil
}
