package cluster

import (
	"context"
	"fmt"
	"log"
	"reflect"
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

func WaitForMCPUpdateToStart(cs *client.ClientSet, role string) {
	gomega.Eventually(func() (bool, error) {
		mcp := &machineconfigurationv1.MachineConfigPool{}
		err := cs.Get(context.TODO(), controllersClient.ObjectKey{Name: role}, mcp)
		if err != nil {
			return false, fmt.Errorf("failed to list MachineConfigPools: %v", err)
		}

		if mcp.Status.UpdatedMachineCount != mcp.Status.MachineCount {
			log.Printf("MCP %s has started updating", mcp.Name)
			return true, nil
		}

		return false, nil
	}, timeout, 30*time.Second).Should(gomega.BeTrue(), "Timed out waiting for MCP to start updating")
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

func ApplyMachineConfigAndCheckChange(yamlInput []byte, c *client.ClientSet) (bool, error) {
	obj := &machineconfigurationv1.MachineConfig{}
	if err := yaml.Unmarshal(yamlInput, obj); err != nil {
		return false, fmt.Errorf("failed to unmarshal YAML: %w", err)
	}

	err := c.Create(context.TODO(), obj)
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return false, fmt.Errorf("failed to create MachineConfig: %w", err)
		}

		// If it already exists, retrieve the current version to update.
		existingMC := &machineconfigurationv1.MachineConfig{}
		if err = c.Get(context.TODO(), controllersClient.ObjectKey{Name: obj.Name}, existingMC); err != nil {
			return false, fmt.Errorf("failed to get existing MachineConfig: %w", err)
		}

		// Compare if there is a real change
		if !reflect.DeepEqual(existingMC.Spec, obj.Spec) {
			obj.ResourceVersion = existingMC.ResourceVersion
			if err := c.Update(context.TODO(), obj); err != nil {
				return false, fmt.Errorf("failed to update MachineConfig: %w", err)
			}

			log.Println("MachineConfig updated successfully.")
			return true, nil
		}

		log.Println("No changes detected in MachineConfig. Skipping update.")
		return false, nil
	}

	log.Println("MachineConfig created successfully.")
	return true, nil
}

func WaitForMCPReadyState(c *client.ClientSet, role string) {
	gomega.Eventually(func() (bool, error) {
		mcp := &machineconfigurationv1.MachineConfigPool{}
		err := c.Get(context.TODO(), controllersClient.ObjectKey{Name: role}, mcp)
		if err != nil {
			return false, fmt.Errorf("failed to list MachineConfigPools: %v", err)
		}

		if mcp.Status.ReadyMachineCount != mcp.Status.MachineCount {
			log.Printf("MCP %s is still updating or degraded\n", mcp.Name)
			return false, nil
		}

		log.Println("All MCPs are ready and updated")
		return true, nil
	}, timeout, 30*time.Second).Should(gomega.BeTrue(), "Timed out waiting for MCPs to reach the desired state")
}

func ValidateClusterVersionAndMachineConfiguration(cs *client.ClientSet) error {
	thresholdVersionSemver := semver.MustParse(thresholdVersion)
	clusterVersion, err := GetClusterVersion(cs)
	if err != nil {
		return err
	}

	currentVersion, err := semver.NewVersion(clusterVersion)
	if err != nil {
		return err
	}

	if currentVersion.GreaterThan(thresholdVersionSemver) {
		log.Printf("Version Greater Than " + thresholdVersion + " - Updating Machine Configuration")
		err = AddNFTSvcToNodeDisruptionPolicy(cs)
		if err != nil {
			return err
		}
	}
	return nil
}
