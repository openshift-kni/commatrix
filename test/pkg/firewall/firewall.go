package firewall

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/api/errors"

	"gopkg.in/yaml.v2" // For parsing YAML

	"github.com/openshift-kni/commatrix/pkg/client"
	machineconfigurationv1 "github.com/openshift/api/machineconfiguration/v1" // Adjust the import path as needed

	ocpoperatorv1 "github.com/openshift/api/operator/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	butaneConfig "github.com/coreos/butane/config"
	"github.com/coreos/butane/config/common"
	"github.com/openshift-kni/commatrix/pkg/consts"
	"github.com/openshift-kni/commatrix/pkg/utils"
)

// Apply the firewall rules on the node.
func ApplyRulesToNode(NFTtable []byte, nodeName, namespace, artifactsDir string, utilsHelpers utils.UtilsInterface) error {
	debugPod, err := utilsHelpers.CreatePodOnNode(nodeName, namespace, consts.DefaultDebugPodImage)
	if err != nil {
		return fmt.Errorf("failed to create debug pod on node %s: %w", nodeName, err)
	}

	defer func() {
		err := utilsHelpers.DeletePod(debugPod)
		if err != nil {
			log.Printf("failed cleaning debug pod %s: %v", debugPod, err)
		}
	}()

	// save the nftables on  /host/etc/nftables/firewall.nft
	_, err = RunRootCommandOnPod(debugPod, fmt.Sprintf("echo '%s' > /host/etc/nftables/firewall.nft", string(NFTtable)), false, utilsHelpers)
	if err != nil {
		return fmt.Errorf("failed to save rule set to file on node %s: %w", nodeName, err)
	}

	// run apply nft command for the rules
	_, err = RunRootCommandOnPod(debugPod, "nft -f /etc/nftables/firewall.nft", true, utilsHelpers)
	if err != nil {
		return fmt.Errorf("failed to apply rule set on node %s: %w", nodeName, err)
	}

	_, err = NftListAndWriteToFile(debugPod, utilsHelpers, artifactsDir, "nftables-"+nodeName)
	if err != nil {
		return err
	}

	// edit the sys conf to make sure it will be on the list after reboot
	err = editNftablesConf(debugPod, utilsHelpers)
	if err != nil {
		return err
	}

	_, err = RunRootCommandOnPod(debugPod, "/usr/sbin/nft list ruleset > /etc/nftables.conf", true, utilsHelpers)
	if err != nil {
		return fmt.Errorf("failed to save NFT ruleset to file on node %s: %w", nodeName, err)
	}

	_, err = RunRootCommandOnPod(debugPod, "systemctl enable nftables", true, utilsHelpers)
	if err != nil {
		return fmt.Errorf("failed to enable nftables on node %s: %w", nodeName, err)
	}

	return nil
}

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

// Checks if the include statement for the new nftables configuration,
// Is present in the nftables.conf file. If the include statement is not present,
// Add it the statement to the file.
func editNftablesConf(debugPod *v1.Pod, utilsHelpers utils.UtilsInterface) error {
	checkCommand := `grep 'include "/etc/nftables/firewall.nft"' /host/etc/sysconfig/nftables.conf | wc -l`
	output, err := RunRootCommandOnPod(debugPod, checkCommand, false, utilsHelpers)
	if err != nil {
		return fmt.Errorf("failed to check nftables.conf on debug pod: %w", err)
	}

	// Convert output to an integer and check if the include statement exists
	includeCount, err := strconv.Atoi(strings.TrimSpace(string(output)))
	if err != nil {
		return fmt.Errorf("failed to parse grep output: %w", err)
	}

	if includeCount > 0 {
		log.Println("Include statement already exists, no need to add it")
		return nil
	}

	addCommand := `echo 'include "/etc/nftables/firewall.nft"' >> /host/etc/sysconfig/nftables.conf`
	_, err = RunRootCommandOnPod(debugPod, addCommand, false, utilsHelpers)
	if err != nil {
		return fmt.Errorf("failed to edit nftables.conf on debug pod: %w", err)
	}

	return nil
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

func MachineconfigWay(c *client.ClientSet, NFTtable []byte, artifactsDir, nodeRolde string, utilsHelpers utils.UtilsInterface) error {

	fmt.Println("MachineConfiguration way!")

	output, err := createButaneConfig(string(NFTtable), nodeRolde)
	if err != nil {
		return fmt.Errorf("failed to create Butane Config on nodeRole: %s %s: ", nodeRolde, err)
	}
	fileName := fmt.Sprintf("bu-%s.bu", nodeRolde)
	fmt.Printf("fileName the bu MachineConfiguration %s", fileName)

	filePath := filepath.Join(artifactsDir, fileName)

	fmt.Printf("write the bu MachineConfiguration %s", filePath)

	err = utilsHelpers.WriteFile(filePath, output)
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

func createButaneConfig(nftablesRules, nodeRole string) ([]byte, error) {
	lines := strings.Split(nftablesRules, "\n")
	nftablesRulesWithoutFirstLine := ""
	if len(lines) > 1 {
		nftablesRulesWithoutFirstLine = strings.Join(lines[1:], "\n")
	}
	//nftablesRulesWithoutFirstLine = strings.ReplaceAll(nftablesRulesWithoutFirstLine, "\t", "  ")

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
	return []byte(butaneConfig), nil
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

	fmt.Printf("Unmarshaled MachineConfig: %+v\n", obj)

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

// Helper function to convert map[interface{}]interface{} to map[string]interface{}
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
	// Define the command to edit the MachineConfiguration

	mc := &ocpoperatorv1.MachineConfiguration{}
	err := c.Get(context.TODO(), types.NamespacedName{Name: "cluster", Namespace: "openshift-machine-config-operator"}, mc)
	if err != nil {
		log.Fatalf("error getting MachineConfiguration: %v", err)
	}
	mc.Spec.OperatorLogLevel = ocpoperatorv1.Normal
	mc.Spec.ManagementState = ocpoperatorv1.Managed
	mc.Spec.LogLevel = ocpoperatorv1.Normal
	mc.Spec.NodeDisruptionPolicy = ocpoperatorv1.NodeDisruptionPolicyConfig{
		SSHKey: ocpoperatorv1.NodeDisruptionPolicySpecSSHKey{
			Actions: []ocpoperatorv1.NodeDisruptionPolicySpecAction{},
		},
		Files: []ocpoperatorv1.NodeDisruptionPolicySpecFile{
			{
				Path: "/etc/sysconfig/nftables.conf",
				Actions: []ocpoperatorv1.NodeDisruptionPolicySpecAction{
					{
						Type: ocpoperatorv1.RestartSpecAction,
						Restart: &ocpoperatorv1.RestartService{
							ServiceName: "nftables.service",
						},
					},
				},
			},
		},
		Units: []ocpoperatorv1.NodeDisruptionPolicySpecUnit{
			{
				Name: "nftables.service",
				Actions: []ocpoperatorv1.NodeDisruptionPolicySpecAction{
					{
						Type: ocpoperatorv1.DaemonReloadSpecAction,
					},
					{
						Type: ocpoperatorv1.ReloadSpecAction,
						Reload: &ocpoperatorv1.ReloadService{
							ServiceName: "nftables.service",
						},
					},
				},
			},
		},
	}
	if err := c.Update(context.TODO(), mc); err != nil {
		log.Fatalf("failed to update MachineConfiguration: %v", err)
	}

	fmt.Println("MachineConfiguration updated successfully!")
	return nil
}
