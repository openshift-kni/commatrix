package firewall

import (
	"fmt"
	"path/filepath"
	"strings"

	butaneConfig "github.com/coreos/butane/config"

	"github.com/coreos/butane/config/common"
	"github.com/openshift-kni/commatrix/pkg/client"
	"github.com/openshift-kni/commatrix/pkg/utils"
	v1 "k8s.io/api/core/v1"
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

func CreateMachineConfig(c *client.ClientSet, NFTtable []byte, artifactsDir, nodeRolde, clusterVersion string,
	utilsHelpers utils.UtilsInterface) (machineConfig []byte, err error) {
	butaneConfigVar := createButaneConfig(string(NFTtable), nodeRolde, clusterVersion)
	options := common.TranslateBytesOptions{}

	machineConfig, _, err = butaneConfig.TranslateBytes(butaneConfigVar, options)
	if err != nil {
		return nil, fmt.Errorf("failed to covert the ButaneConfig to yaml %v: ", err)
	}

	fileName := fmt.Sprintf("mc-%s.yaml", nodeRolde)
	filePath := filepath.Join(artifactsDir, fileName)
	err = utilsHelpers.WriteFile(filePath, machineConfig)
	if err != nil {
		return nil, err
	}

	return machineConfig, nil
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
