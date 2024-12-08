package firewall

import (
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"

	"github.com/onsi/gomega"

	butaneConfig "github.com/coreos/butane/config"

	"github.com/coreos/butane/config/common"
	"github.com/openshift-kni/commatrix/pkg/client"
	"github.com/openshift-kni/commatrix/pkg/utils"
	v1 "k8s.io/api/core/v1"
)

const butaneVersion = "4.17.0"

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

func NftListAndWriteToFile(debugPod *v1.Pod, utilsHelpers utils.UtilsInterface, artifactsDir, fileName, tableName, chainName string) (bool, error) {
	var output []byte
	var cmdErr error

	command := "nft list ruleset"
	gomega.Eventually(func() bool {
		output, cmdErr = RunRootCommandOnPod(debugPod, command, true, utilsHelpers)
		if cmdErr != nil {
			log.Printf("Error listing nft ruleset: %v", cmdErr)
			return false
		}

		if len(output) == 0 {
			log.Println("No nft rules found in output.")
			return false
		}

		if !strings.Contains(string(output), tableName) || !strings.Contains(string(output), chainName) {
			log.Printf("Required table %s or chain %s not found in nftables output.", tableName, chainName)
			return false
		}

		return true
	}, 2*time.Minute, 10*time.Second).Should(gomega.BeTrue(), "Timeout while waiting for nftables to contain table %s and chain %s", tableName, chainName)

	if cmdErr != nil {
		return false, fmt.Errorf("failed to list nft ruleset: %w", cmdErr)
	}

	if !strings.Contains(string(output), tableName) || !strings.Contains(string(output), chainName) {
		log.Printf("Retrying with journalctl for diagnostics.")
		journalCommand := "journalctl -u nftables"
		journalOutput, err := RunRootCommandOnPod(debugPod, journalCommand, true, utilsHelpers)
		if err != nil {
			return false, fmt.Errorf("failed to run journalctl command: %w", err)
		}

		err = utilsHelpers.WriteFile(filepath.Join(artifactsDir, "journalctl"), journalOutput)
		if err != nil {
			log.Printf("Failed to write journalctl to file: %v", err)
			return false, fmt.Errorf("failed to write journalctl to file: %w", err)
		}

		return false, fmt.Errorf("required table %s or chain %s not found in nftables output", tableName, chainName)
	}

	err := utilsHelpers.WriteFile(filepath.Join(artifactsDir, fileName), output)
	if err != nil {
		log.Printf("Failed to write nft ruleset to file: %v", err)
		return false, fmt.Errorf("failed to write nft ruleset to file: %w", err)
	}

	log.Printf("nft output is %s", output)
	return true, nil
}

func CreateMachineConfig(c *client.ClientSet, NFTtable []byte, artifactsDir, nodeRolde string,
	utilsHelpers utils.UtilsInterface) (machineConfig []byte, err error) {
	butaneConfigVar := createButaneConfig(string(NFTtable), nodeRolde)
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

func createButaneConfig(nftablesRules, nodeRole string) []byte {
	lines := strings.Split(nftablesRules, "\n")
	nftablesRulesWithoutFirstLine := ""
	if len(lines) > 1 {
		nftablesRulesWithoutFirstLine = strings.Join(lines[1:], "\n")
	}
	indentedRules := indentContent(nftablesRulesWithoutFirstLine, 10)

	butaneConfig := fmt.Sprintf(`variant: openshift
version: %s
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
        `, butaneVersion, nodeRole, nodeRole, indentedRules)
	butaneConfig = strings.ReplaceAll(butaneConfig, "\t", "  ")
	return []byte(butaneConfig)
}

func indentContent(content string, indentSize int) string {
	lines := strings.Split(content, "\n")
	indent := strings.Repeat(" ", indentSize)
	for i, line := range lines {
		lines[i] = indent + line
	}
	return strings.Join(lines, "\n")
}
