package firewall

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"

	"gopkg.in/yaml.v2" // For parsing YAML

	"github.com/openshift-kni/commatrix/pkg/client"
	machineconfigurationv1 "github.com/openshift/api/machineconfiguration/v1" // Adjust the import path as needed
	mcoac "github.com/openshift/client-go/operator/applyconfigurations/operator/v1"
	mcopclientset "github.com/openshift/client-go/operator/clientset/versioned"

	ocpoperatorv1 "github.com/openshift/api/operator/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	butaneConfig "github.com/coreos/butane/config"
	"github.com/coreos/butane/config/common"
	"github.com/openshift-kni/commatrix/pkg/utils"
	controllersClient "sigs.k8s.io/controller-runtime/pkg/client"
)

func RunRootCommandOnPod(debugPod *v1.Pod, command string, chrootDir bool, utilsHelpers utils.UtilsInterface) ([]byte, error) {
	if chrootDir {
		// Format the command for chroot
		command = fmt.Sprintf("chroot /host /bin/bash -c '%s'", command)
	}

	output, err := utilsHelpers.RunCommandOnPod(debugPod, []string{"bash", "-c", command})
	if err != nil {
		return nil, fmt.Errorf("failed to execute command '%s' on node %s: %w", command, debugPod.Spec.NodeName, err)
	}
	return output, nil
}

func NftListAndWriteToFile(debugPod *v1.Pod, utilsHelpers utils.UtilsInterface, artifactsDir, fileName string) ([]byte, error) {
	command := "nft list ruleset"
	output, err := RunRootCommandOnPod(debugPod, command, true, utilsHelpers)
	if err != nil {
		return nil, fmt.Errorf("failed to list NFT ruleset on node %s: %w", debugPod.Spec.NodeName, err)
	}

	if len(output) == 0 {
		return nil, fmt.Errorf("no nft rules on node %s: ", debugPod.Spec.NodeName)
	}

	err = utilsHelpers.WriteFile(filepath.Join(artifactsDir, fileName), output)
	if err != nil {
		return nil, err
	}

	return output, nil
}

func Apply(c *client.ClientSet, NFTtable []byte, artifactsDir, nodeRolde string, utilsHelpers utils.UtilsInterface) error {
	fmt.Println("MachineConfiguration way!")

	output := createButaneConfig(string(NFTtable), nodeRolde)

	fileName := fmt.Sprintf("bu-%s.bu", nodeRolde)
	fmt.Printf("fileName the bu MachineConfiguration %s", fileName)

	filePath := filepath.Join(artifactsDir, fileName)

	fmt.Printf("write the bu MachineConfiguration %s", filePath)

	err := utilsHelpers.WriteFile(filePath, output)
	if err != nil {
		return err
	}
	fmt.Println("suc write the bu MachineConfiguration ")

	output, err = ConvertButaneToYAML(output)
	if err != nil {
		return fmt.Errorf("failed to list NFT ruleset on node %s: ", err)
	}
	fmt.Println("convert the bu MachineConfiguration ")
	fileName = fmt.Sprintf("bu-%s.yaml", nodeRolde)
	filePath = filepath.Join(artifactsDir, fileName)
	err = utilsHelpers.WriteFile(filePath, output)
	if err != nil {
		return err
	}

	fmt.Println("apply the yaml MachineConfiguration ")

	if err = applyYAMLWithOC(output, c, utilsHelpers, artifactsDir, nodeRolde); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return err
	}

	return nil
}

// isVersionGreaterThan compares two version strings (major.minor) and returns true if v1 > v2.
func IsVersionGreaterThan(v1, v2 string) bool {
	parts1 := strings.Split(v1, ".")
	parts2 := strings.Split(v2, ".")

	// Convert string parts to integers for comparison
	for i := 0; i < 2; i++ {
		if len(parts1) > i && len(parts2) > i {
			if parts1[i] > parts2[i] {
				return true
			} else if parts1[i] < parts2[i] {
				return false
			}
		} else if len(parts1) > i {
			// If v2 has no more parts, v1 is greater
			return true
		}
	}

	// If they are equal or v1 is less than v2
	return false
}

func createButaneConfig(nftablesRules, nodeRole string) []byte {
	lines := strings.Split(nftablesRules, "\n")
	nftablesRulesWithoutFirstLine := ""
	if len(lines) > 1 {
		nftablesRulesWithoutFirstLine = strings.Join(lines[1:], "\n")
	}

	butaneConfig := fmt.Sprintf(`variant: openshift
version: 4.16.0
metadata:
  name: 98-nftables-cnf-%s
  labels:
    machineconfiguration.openshift.io/role: %s
systemd:
  units:
    - name: "nftables.service"
      enabled: true
      contents: |
        [Unit]
        Description=Netfilter Tables
        Documentation=man:nft(8)
        Wants=network-pre.target
        Before=network-pre.target
        [Service]
        Type=oneshot
        ProtectSystem=full
        ProtectHome=true
        ExecStart=/sbin/nft -f /etc/sysconfig/nftables.conf
        ExecReload=/sbin/nft -f /etc/sysconfig/nftables.conf
        ExecStop=/sbin/nft 'add table inet openshift_filter; delete table inet openshift_filter'
        RemainAfterExit=yes
        [Install]
        WantedBy=multi-user.target
storage:
  files:
    - path: /etc/sysconfig/nftables.conf
      mode: 0600
      overwrite: true
      contents:
        inline: |
          table inet openshift_filter
          delete table inet openshift_filter
		%s
        `, nodeRole, nodeRole, nftablesRulesWithoutFirstLine)
	butaneConfig = strings.ReplaceAll(butaneConfig, "\t", "  ")
	return []byte(butaneConfig)
}

func fail(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format, args...)
	os.Exit(1)
}

func ConvertButaneToYAML(butaneContent []byte) ([]byte, error) {
	options := common.TranslateBytesOptions{}

	dataOut, _, err := butaneConfig.TranslateBytes(butaneContent, options)
	if err != nil {
		fail("failed to  : %v\n", err)
	}

	return dataOut, nil
}

func applyYAMLWithOC(output []byte, c *client.ClientSet, utilsHelpers utils.UtilsInterface, artifactsDir, role string) error {
	var data map[interface{}]interface{}
	err := yaml.Unmarshal(output, &data)
	if err != nil {
		return fmt.Errorf("failed to unmarshal YAML: %w", err)
	}

	// Convert map[interface{}]interface{} to map[string]interface{}
	convertedData := convertMapInterfaceToString(data)
	// Marshal the converted map into JSON
	jsonData, err := json.MarshalIndent(convertedData, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal to JSON: %w", err)
	}

	fileName := fmt.Sprintf("convertedData-%s.json", role)
	filePath := filepath.Join(artifactsDir, fileName)
	err = utilsHelpers.WriteFile(filePath, jsonData)
	if err != nil {
		return err
	}

	obj := &machineconfigurationv1.MachineConfig{}
	if err := json.Unmarshal(jsonData, obj); err != nil {
		return fmt.Errorf("failed to unmarshal JSON: %w", err)
	}

	// Check if the MachineConfig already exists
	existingMC := &machineconfigurationv1.MachineConfig{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: obj.Name}, existingMC)
	if err != nil {
		if errors.IsNotFound(err) {
			// If it doesn't exist, create it
			if err := c.Create(context.TODO(), obj); err != nil {
				return fmt.Errorf("failed to create MachineConfig: %w", err)
			}
			fmt.Println("MachineConfig created successfully.")
		} else {
			return fmt.Errorf("failed to get MachineConfig: %w", err)
		}
	} else {
		// If it exists, update it
		// Set the resourceVersion from the existing object
		obj.ResourceVersion = existingMC.ResourceVersion

		if err := c.Update(context.TODO(), obj); err != nil {
			return fmt.Errorf("failed to update MachineConfig: %w", err)
		}
		fmt.Println("MachineConfig updated successfully.")
	}

	return nil
}

// Helper function to convert map[interface{}]interface{} to map[string]interface{}.
func convertMapInterfaceToString(data interface{}) interface{} {
	switch v := data.(type) {
	case map[interface{}]interface{}:
		newMap := make(map[string]interface{})
		for key, value := range v {
			newMap[fmt.Sprintf("%v", key)] = convertMapInterfaceToString(value)
		}
		return newMap
	case []interface{}:
		for i, item := range v {
			v[i] = convertMapInterfaceToString(item)
		}
	}
	return data
}

func UpdateMachineConfiguration(c *client.ClientSet) error {
	machineConfigurationClient := mcopclientset.NewForConfigOrDie(c.GetRestConfig())
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
		log.Fatalf("updating cluster node disruption policy failed %v", err)
	}

	fmt.Println("MachineConfiguration updated successfully!")
	return nil
}

func WaitForMCPReady(c *client.ClientSet, timeout time.Duration) error {
	start := time.Now()

	for {
		mcpList := &machineconfigurationv1.MachineConfigPoolList{}

		err := c.List(context.TODO(), mcpList, &controllersClient.ListOptions{})
		if err != nil {
			return fmt.Errorf("failed to list MachineConfigPools: %v", err)
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
			}
		}

		// If all MachineConfigPools are in the desired state, we can exit the loop
		if allReady {
			fmt.Println("All MCPs are ready and updated")
			break
		}

		// If timeout exceeded, return an error
		if time.Since(start) > timeout {
			return fmt.Errorf("timed out waiting for MCPs to reach the desired state")
		}

		// Wait for a while before checking again
		time.Sleep(30 * time.Second) // Adjust sleep time as needed
	}

	return nil
}
