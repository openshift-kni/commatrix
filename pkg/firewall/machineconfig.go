package firewall

import (
	"fmt"
	"strings"

	"github.com/Masterminds/semver/v3"
	butaneConfig "github.com/coreos/butane/config"
	"github.com/coreos/butane/config/common"
	"github.com/openshift-kni/commatrix/pkg/utils"
)

const maxButaneVersion = "4.21"

// NFTablesToButane converts nftables rules into a Butane YAML configuration
// without transpiling it to a MachineConfig. The resulting Butane config:
//   - Embeds the nftables rules into /etc/sysconfig/nftables.conf
//   - Enables and configures the nftables.service systemd unit
//   - Is labelled for the given node pool (e.g. "master", "worker")
func NFTablesToButane(nftRules []byte, nodePool string, utilsHelpers utils.UtilsInterface) ([]byte, error) {
	return buildButaneConfig(string(nftRules), nodePool, utilsHelpers)
}

// NFTablesToMachineConfig converts nftables rules into a MachineConfig YAML
// by building a Butane config and translating it via the Butane library.
// The resulting MachineConfig:
//   - Embeds the nftables rules into /etc/sysconfig/nftables.conf
//   - Enables and configures the nftables.service systemd unit
//   - Is labelled for the given node pool (e.g. "master", "worker")
func NFTablesToMachineConfig(nftRules []byte, nodePool string, utilsHelpers utils.UtilsInterface) ([]byte, error) {
	butaneCfg, err := buildButaneConfig(string(nftRules), nodePool, utilsHelpers)
	if err != nil {
		return nil, err
	}

	machineConfig, _, err := butaneConfig.TranslateBytes(butaneCfg, common.TranslateBytesOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to convert Butane config to MachineConfig YAML: %w", err)
	}

	return machineConfig, nil
}

// buildButaneConfig constructs a Butane YAML configuration that:
//   - Creates a MachineConfig named 98-nftables-commatrix-{pool}
//   - Deploys nftables rules to /etc/sysconfig/nftables.conf
//   - Enables and starts the nftables.service systemd unit
//
// The Butane spec version is derived from the cluster's OCP version.
func buildButaneConfig(nftablesRules, nodePool string, utilsHelpers utils.UtilsInterface) ([]byte, error) {
	clusterVersion, err := utilsHelpers.GetClusterVersion()
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster version for Butane spec: %w", err)
	}
	butaneVersion, err := resolveButaneVersion(clusterVersion)
	if err != nil {
		return nil, err
	}

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
	return []byte(butaneCfg), nil
}

// resolveButaneVersion maps a cluster version (e.g. "4.17") to a stable
// Butane openshift spec version. Cluster versions above the latest stable
// spec are capped to maxButaneVersion.
func resolveButaneVersion(clusterVersion string) (string, error) {
	cv, err := semver.NewVersion(clusterVersion)
	if err != nil {
		return "", fmt.Errorf("invalid cluster version %q: %w", clusterVersion, err)
	}
	max, _ := semver.NewVersion(maxButaneVersion)
	if cv.GreaterThan(max) {
		return maxButaneVersion + ".0", nil
	}
	return clusterVersion + ".0", nil
}

func indentContent(content string, indentSize int) string {
	lines := strings.Split(content, "\n")
	indent := strings.Repeat(" ", indentSize)
	for i, line := range lines {
		lines[i] = indent + line
	}
	return strings.Join(lines, "\n")
}
