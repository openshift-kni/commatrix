package firewall

import (
	"testing"

	"github.com/openshift-kni/commatrix/pkg/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeUtils struct {
	utils.UtilsInterface
	version    string
	versionErr error
}

func (f fakeUtils) GetClusterVersion() (string, error) {
	return f.version, f.versionErr
}

func TestResolveButaneVersion(t *testing.T) {
	testCases := []struct {
		name           string
		clusterVersion string
		expected       string
		wantErr        bool
	}{
		{
			name:           "standard version below cap",
			clusterVersion: "4.17",
			expected:       "4.17.0",
		},
		{
			name:           "version at cap boundary",
			clusterVersion: "4.21",
			expected:       "4.21.0",
		},
		{
			name:           "version above cap is capped",
			clusterVersion: "4.22",
			expected:       "4.21.0",
		},
		{
			name:           "much higher version is capped",
			clusterVersion: "4.30",
			expected:       "4.21.0",
		},
		{
			name:           "major version bump is capped",
			clusterVersion: "5.0",
			expected:       "4.21.0",
		},
		{
			name:           "minimum supported version",
			clusterVersion: "4.12",
			expected:       "4.12.0",
		},
		{
			name:           "invalid version string",
			clusterVersion: "not-a-version",
			wantErr:        true,
		},
		{
			name:           "empty version string",
			clusterVersion: "",
			wantErr:        true,
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			result, err := resolveButaneVersion(tt.clusterVersion)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNFTablesToButane(t *testing.T) {
	nftRules := []byte(`#!/usr/sbin/nft -f
table inet openshift_filter {
  chain input {
    type filter hook input priority 0; policy accept;
    tcp dport { 443 } accept
  }
}
`)

	t.Run("produces valid Butane YAML with correct metadata", func(t *testing.T) {
		out, err := NFTablesToButane(nftRules, "master", fakeUtils{version: "4.17"})
		require.NoError(t, err)

		result := string(out)
		assert.Contains(t, result, "variant: openshift")
		assert.Contains(t, result, "version: 4.17.0")
		assert.Contains(t, result, "name: 98-nftables-commatrix-master")
		assert.Contains(t, result, "machineconfiguration.openshift.io/role: master")
		assert.Contains(t, result, "nftables.service")
		assert.Contains(t, result, "/etc/sysconfig/nftables.conf")
	})

	t.Run("embeds the nftables rules", func(t *testing.T) {
		out, err := NFTablesToButane(nftRules, "master", fakeUtils{version: "4.17"})
		require.NoError(t, err)

		result := string(out)
		assert.Contains(t, result, "tcp dport { 443 } accept")
	})

	t.Run("strips the shebang line", func(t *testing.T) {
		out, err := NFTablesToButane(nftRules, "master", fakeUtils{version: "4.17"})
		require.NoError(t, err)
		assert.NotContains(t, string(out), "#!/usr/sbin/nft")
	})

	t.Run("uses correct pool label for worker", func(t *testing.T) {
		out, err := NFTablesToButane(nftRules, "worker", fakeUtils{version: "4.17"})
		require.NoError(t, err)

		result := string(out)
		assert.Contains(t, result, "name: 98-nftables-commatrix-worker")
		assert.Contains(t, result, "machineconfiguration.openshift.io/role: worker")
	})

	t.Run("caps version for clusters above maxButaneVersion", func(t *testing.T) {
		out, err := NFTablesToButane(nftRules, "master", fakeUtils{version: "4.25"})
		require.NoError(t, err)
		assert.Contains(t, string(out), "version: 4.21.0")
	})

	t.Run("returns error when GetClusterVersion fails", func(t *testing.T) {
		_, err := NFTablesToButane(nftRules, "master", fakeUtils{
			versionErr: assert.AnError,
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get cluster version")
	})

	t.Run("includes systemd unit configuration", func(t *testing.T) {
		out, err := NFTablesToButane(nftRules, "master", fakeUtils{version: "4.17"})
		require.NoError(t, err)

		result := string(out)
		assert.Contains(t, result, "enabled: true")
		assert.Contains(t, result, "ExecStart=/sbin/nft -f /etc/sysconfig/nftables.conf")
		assert.Contains(t, result, "ExecReload=/sbin/nft -f /etc/sysconfig/nftables.conf")
		assert.Contains(t, result, "RemainAfterExit=yes")
	})
}

func TestNFTablesToMachineConfig(t *testing.T) {
	nftRules := []byte(`#!/usr/sbin/nft -f
table inet openshift_filter {
  chain input {
    type filter hook input priority 0; policy accept;
    tcp dport { 443 } accept
  }
}
`)

	t.Run("produces valid MachineConfig YAML", func(t *testing.T) {
		out, err := NFTablesToMachineConfig(nftRules, "master", fakeUtils{version: "4.17"})
		require.NoError(t, err)

		result := string(out)
		assert.Contains(t, result, "kind: MachineConfig")
		assert.Contains(t, result, "name: 98-nftables-commatrix-master")
		assert.Contains(t, result, "machineconfiguration.openshift.io/role: master")
	})

	t.Run("includes storage and systemd sections", func(t *testing.T) {
		out, err := NFTablesToMachineConfig(nftRules, "master", fakeUtils{version: "4.17"})
		require.NoError(t, err)

		result := string(out)
		assert.Contains(t, result, "storage:")
		assert.Contains(t, result, "/etc/sysconfig/nftables.conf")
		assert.Contains(t, result, "systemd:")
		assert.Contains(t, result, "nftables.service")
	})

	t.Run("returns error when GetClusterVersion fails", func(t *testing.T) {
		_, err := NFTablesToMachineConfig(nftRules, "master", fakeUtils{
			versionErr: assert.AnError,
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get cluster version")
	})
}

func TestIndentContent(t *testing.T) {
	testCases := []struct {
		name       string
		content    string
		indentSize int
		expected   string
	}{
		{
			name:       "indents single line",
			content:    "hello",
			indentSize: 4,
			expected:   "    hello",
		},
		{
			name:       "indents multiple lines",
			content:    "line1\nline2\nline3",
			indentSize: 2,
			expected:   "  line1\n  line2\n  line3",
		},
		{
			name:       "zero indent is a no-op",
			content:    "line1\nline2",
			indentSize: 0,
			expected:   "line1\nline2",
		},
		{
			name:       "handles empty string",
			content:    "",
			indentSize: 4,
			expected:   "    ",
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			result := indentContent(tt.content, tt.indentSize)
			assert.Equal(t, tt.expected, result)
		})
	}
}
