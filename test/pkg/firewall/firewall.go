package firewall

import (
	"fmt"
	"path/filepath"
	"strings"

	butaneConfig "github.com/coreos/butane/config"

	"github.com/coreos/butane/config/common"
	"github.com/openshift-kni/commatrix/pkg/client"
	"github.com/openshift-kni/commatrix/pkg/utils"
)

const butaneVersion = "4.17.0"

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
