package cluster

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/onsi/gomega"
	"github.com/openshift-kni/commatrix/pkg/client"
	"github.com/openshift-kni/commatrix/pkg/utils"

	machineconfigurationv1 "github.com/openshift/api/machineconfiguration/v1"
	ocpoperatorv1 "github.com/openshift/api/operator/v1"
	mcoac "github.com/openshift/client-go/operator/applyconfigurations/operator/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery"
	controllersClient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/yaml"
)

const (
	thresholdVersion = "4.16" // above this version the nft service must be added to the NodeDisruptionPolicy in the MachineConfiguration.
	timeout          = 20 * time.Minute
	interval         = 5 * time.Second
)

// GetMachineConfigPool returns the MachineConfigPool with the given name.
func GetMachineConfigPool(cs *client.ClientSet, name string) (*machineconfigurationv1.MachineConfigPool, error) {
	mcp := &machineconfigurationv1.MachineConfigPool{}
	if err := cs.Get(context.TODO(), controllersClient.ObjectKey{Name: name}, mcp); err != nil {
		return nil, fmt.Errorf("failed to get %q MachineConfigPool: %w", name, err)
	}
	return mcp, nil
}

// WaitForMCPUpdate waits for the MCO to render a new MachineConfig (by comparing
// the rendered MC name to previousRenderedMC) and then waits for all machines in the
// pool to be ready with the new config (timeout: 20m, polling interval: 5s).
// This avoids polling for transient status changes (UpdatedMachineCount != MachineCount)
// which can be missed on SNO where NodeDisruptionPolicy completes in seconds.
func WaitForMCPUpdate(cs *client.ClientSet, name, previousRenderedMC string) {
	gomega.Eventually(func() (bool, error) {
		mcp, err := GetMachineConfigPool(cs, name)
		if err != nil {
			return false, err
		}

		currentRenderedMC := mcp.Status.Configuration.Name
		if currentRenderedMC == previousRenderedMC {
			log.Printf("MCP %s: rendered MC unchanged (%s), waiting for MCO to process", name, currentRenderedMC)
			return false, nil
		}

		log.Printf("MCP %s: rendered MC changed from %q to %q", name, previousRenderedMC, currentRenderedMC)

		if mcp.Status.ReadyMachineCount == mcp.Status.MachineCount &&
			mcp.Status.UpdatedMachineCount == mcp.Status.MachineCount {
			log.Printf("MCP %s: all machines ready and updated", name)
			return true, nil
		}

		log.Printf("MCP %s: still updating (ready=%d, updated=%d, total=%d)",
			name, mcp.Status.ReadyMachineCount, mcp.Status.UpdatedMachineCount, mcp.Status.MachineCount)
		return false, nil
	}, timeout, interval).Should(gomega.BeTrue(), "Timed out waiting for MCP %s to complete update", name)
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
		return fmt.Errorf("updating cluster node disruption policy failed: %w", err)
	}

	log.Println("MachineConfiguration updated successfully!")
	return nil
}

// ApplyMachineConfig applies the MachineConfig and returns true if created or updated.
// False if unchanged, and an error if there is error.
func ApplyMachineConfig(yamlInput []byte, c *client.ClientSet) (bool, error) {
	obj := &machineconfigurationv1.MachineConfig{}
	if err := yaml.Unmarshal(yamlInput, obj); err != nil {
		return false, fmt.Errorf("failed to unmarshal YAML: %w", err)
	}

	modifiedConfig := obj.Spec.Config.DeepCopy()
	operationResult, err := controllerutil.CreateOrUpdate(context.TODO(), c, obj, func() error {
		obj.Spec.Config = *modifiedConfig
		return nil
	})

	if err != nil {
		return false, fmt.Errorf("failed to apply MachineConfig: %w", err)
	}

	if operationResult == controllerutil.OperationResultNone {
		return false, nil
	}

	return true, nil
}

func ValidateClusterVersionAndMachineConfiguration(cs *client.ClientSet) error {
	thresholdVersionSemver := semver.MustParse(thresholdVersion)
	utilsHelpers := utils.New(cs)
	clusterVersion, err := utilsHelpers.GetClusterVersion()
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

func WaitForAPIGroupAvailable(cs *client.ClientSet, groupVersion string) {
	disco := discovery.NewDiscoveryClientForConfigOrDie(cs.Config)
	gomega.Eventually(func() bool {
		_, err := disco.ServerResourcesForGroupVersion(groupVersion)
		return err == nil
	}, timeout, interval).Should(gomega.BeTrue(), "API group %s not available", groupVersion)
}
