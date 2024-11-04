package firewall

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	butaneConfig "github.com/coreos/butane/config"

	"github.com/coreos/butane/config/common"
	"github.com/openshift-kni/commatrix/pkg/client"
	"github.com/openshift-kni/commatrix/pkg/utils"
	machineconfigurationv1 "github.com/openshift/api/machineconfiguration/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/yaml"
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

func Apply(c *client.ClientSet, NFTtable []byte, artifactsDir, nodeRolde, clusterVersion string, utilsHelpers utils.UtilsInterface) error {
	butaneConfig := createButaneConfig(string(NFTtable), nodeRolde, clusterVersion)

	fileName := fmt.Sprintf("bu-%s.bu", nodeRolde)
	filePath := filepath.Join(artifactsDir, fileName)

	err := utilsHelpers.WriteFile(filePath, butaneConfig)
	if err != nil {
		return err
	}

	yamlButaneConfig, err := convertButaneToYAML(butaneConfig)
	if err != nil {
		return fmt.Errorf("failed to covert the  ButaneConfig to yaml %v: ", err)
	}

	fileName = fmt.Sprintf("bu-%s.yaml", nodeRolde)
	filePath = filepath.Join(artifactsDir, fileName)
	err = utilsHelpers.WriteFile(filePath, yamlButaneConfig)
	if err != nil {
		return err
	}

	if err = createMachineConfig(yamlButaneConfig, c); err != nil {
		return err
	}

	return nil
}

func createButaneConfig(nftablesRules, nodeRole, clusterVersion string) []byte {
	lines := strings.Split(nftablesRules, "\n")
	nftablesRulesWithoutFirstLine := ""
	if len(lines) > 1 {
		nftablesRulesWithoutFirstLine = strings.Join(lines[1:], "\n")
	}

	butaneConfig := fmt.Sprintf(`variant: openshift
version: %s.0
metadata:
  name: 98-nftables-commatrix-%s
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
        `, clusterVersion, nodeRole, nodeRole, nftablesRulesWithoutFirstLine)
	butaneConfig = strings.ReplaceAll(butaneConfig, "\t", "  ")
	return []byte(butaneConfig)
}

func convertButaneToYAML(butaneContent []byte) ([]byte, error) {
	options := common.TranslateBytesOptions{}

	dataOut, _, err := butaneConfig.TranslateBytes(butaneContent, options)
	if err != nil {
		return nil, err
	}

	return dataOut, nil
}

func createMachineConfig(yamlInput []byte, c *client.ClientSet) error {
	obj := &machineconfigurationv1.MachineConfig{}
	if err := yaml.Unmarshal(yamlInput, obj); err != nil {
		return fmt.Errorf("failed to unmarshal YAML: %w", err)
	}

	existingMC := &machineconfigurationv1.MachineConfig{}
	err := c.Get(context.TODO(), types.NamespacedName{Name: obj.Name}, existingMC)
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
