package firewall

import (
	"fmt"
	"strings"

	butaneConfig "github.com/coreos/butane/config"
	"github.com/coreos/butane/config/common"
)

const butaneVersion = "4.17.0"

// NFTablesToButane converts nftables rules into a Butane YAML configuration
// without transpiling it to a MachineConfig. The resulting Butane config:
//   - Embeds the nftables rules into /etc/sysconfig/nftables.conf
//   - Enables and configures the nftables.service systemd unit
//   - Is labelled for the given node pool (e.g. "master", "worker")
func NFTablesToButane(nftRules []byte, nodePool string) []byte {
	return buildButaneConfig(string(nftRules), nodePool)
}

// NFTablesToMachineConfig converts nftables rules into a MachineConfig YAML
// by building a Butane config and translating it via the Butane library.
// The resulting MachineConfig:
//   - Embeds the nftables rules into /etc/sysconfig/nftables.conf
//   - Enables and configures the nftables.service systemd unit
//   - Is labelled for the given node pool (e.g. "master", "worker")
func NFTablesToMachineConfig(nftRules []byte, nodePool string) ([]byte, error) {
	butaneCfg := buildButaneConfig(string(nftRules), nodePool)
	options := common.TranslateBytesOptions{}

	machineConfig, _, err := butaneConfig.TranslateBytes(butaneCfg, options)
	if err != nil {
		return nil, fmt.Errorf("failed to convert Butane config to MachineConfig YAML: %v", err)
	}

	return machineConfig, nil
}

// buildButaneConfig constructs a Butane YAML configuration that:
//   - Creates a MachineConfig named 98-nftables-commatrix-{pool}
//   - Deploys nftables rules to /etc/sysconfig/nftables.conf
//   - Enables and starts the nftables.service systemd unit
func buildButaneConfig(nftablesRules, nodePool string) []byte {
	// Strip the shebang line (#!/usr/sbin/nft -f) since the file is loaded
	// by the nftables service directly.
	lines := strings.Split(nftablesRules, "\n")
	nftablesRulesWithoutFirstLine := ""
	if len(lines) > 1 {
		nftablesRulesWithoutFirstLine = strings.Join(lines[1:], "\n")
	}
	indentedRules := indentContent(nftablesRulesWithoutFirstLine, 10)

	butaneCfg := fmt.Sprintf(`variant: openshift
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
        `, butaneVersion, nodePool, nodePool, indentedRules)
	butaneCfg = strings.ReplaceAll(butaneCfg, "\t", "  ")
	return []byte(butaneCfg)
}

func indentContent(content string, indentSize int) string {
	lines := strings.Split(content, "\n")
	indent := strings.Repeat(" ", indentSize)
	for i, line := range lines {
		lines[i] = indent + line
	}
	return strings.Join(lines, "\n")
}
