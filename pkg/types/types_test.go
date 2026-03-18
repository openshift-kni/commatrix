package types

import (
	"github.com/openshift-kni/commatrix/pkg/utils"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
)

// fakeUtils embeds UtilsInterface and overrides only GetClusterVersion
// for use in unit tests that don't need a real cluster connection.
type fakeUtils struct {
	utils.UtilsInterface
	version string
}

func (f fakeUtils) GetClusterVersion() (string, error) {
	return f.version, nil
}

var _ = g.Describe("Dynamic range parsing and helpers", func() {
	g.Describe("ParsePortRangeHyphen", func() {
		g.It("parses valid hyphen-separated ranges", func() {
			min, max, err := ParsePortRangeHyphen("1000-2000")
			o.Expect(err).ToNot(o.HaveOccurred())
			o.Expect(min).To(o.Equal(1000))
			o.Expect(max).To(o.Equal(2000))
		})

		g.It("trims spaces around values", func() {
			min, max, err := ParsePortRangeHyphen("  30000 -  30999 ")
			o.Expect(err).ToNot(o.HaveOccurred())
			o.Expect(min).To(o.Equal(30000))
			o.Expect(max).To(o.Equal(30999))
		})

		g.It("errors on malformed input", func() {
			_, _, err := ParsePortRangeHyphen("1000-")
			o.Expect(err).To(o.HaveOccurred())

			_, _, err = ParsePortRangeHyphen("-2000")
			o.Expect(err).To(o.HaveOccurred())

			_, _, err = ParsePortRangeHyphen("a-b")
			o.Expect(err).To(o.HaveOccurred())

			_, _, err = ParsePortRangeHyphen("1000-2000-3000")
			o.Expect(err).To(o.HaveOccurred())
		})
	})

	g.Describe("ParsePortRangeSpace", func() {
		g.It("parses valid space-separated ranges", func() {
			min, max, err := ParsePortRangeSpace("40000 50000")
			o.Expect(err).ToNot(o.HaveOccurred())
			o.Expect(min).To(o.Equal(40000))
			o.Expect(max).To(o.Equal(50000))
		})

		g.It("trims extra whitespace", func() {
			min, max, err := ParsePortRangeSpace("   12345   23456  ")
			o.Expect(err).ToNot(o.HaveOccurred())
			o.Expect(min).To(o.Equal(12345))
			o.Expect(max).To(o.Equal(23456))
		})

		g.It("errors on malformed input", func() {
			_, _, err := ParsePortRangeSpace("12345")
			o.Expect(err).To(o.HaveOccurred())

			_, _, err = ParsePortRangeSpace("a b")
			o.Expect(err).To(o.HaveOccurred())
		})
	})

	g.Describe("parseDynamicRangeFromCSVRow", func() {
		g.It("creates DynamicRange from a valid CSV row fields", func() {
			dr, err := parseDynamicRangeFromCSVRow("Ingress", "TCP", "NodePort range", true, "30000-32767")
			o.Expect(err).ToNot(o.HaveOccurred())
			o.Expect(dr.Direction).To(o.Equal("Ingress"))
			o.Expect(dr.Protocol).To(o.Equal("TCP"))
			o.Expect(dr.MinPort).To(o.Equal(30000))
			o.Expect(dr.MaxPort).To(o.Equal(32767))
			o.Expect(dr.Description).To(o.Equal("NodePort range"))
			o.Expect(dr.Optional).To(o.BeTrue())
		})

		g.It("errors on invalid port range field", func() {
			_, err := parseDynamicRangeFromCSVRow("Egress", "UDP", "bad", false, "foo-bar")
			o.Expect(err).To(o.HaveOccurred())
		})
	})

	g.Describe("parseCSVToComMatrix", func() {
		g.It("parses CSV with a mix of ports and dynamic ranges", func() {
			csvContent := []byte(
				"Direction,Protocol,Port,Namespace,Service,Pod,Container,NodeGroup,Optional\n" +
					"Ingress,TCP,443,ns1,svc1,pod1,ctr1,master,false\n" +
					"Egress,UDP,30000-30100,,NodePort range,,,,true\n" +
					"Ingress,TCP,49152-65535,,Linux ephemeral range,,,,true\n" +
					"Egress,TCP,,,,,,worker,false\n", // empty Port should be skipped
			)

			cm, err := parseCSVToComMatrix(csvContent)
			o.Expect(err).ToNot(o.HaveOccurred())
			o.Expect(cm).ToNot(o.BeNil())

			// Ports
			o.Expect(cm.Ports).To(o.HaveLen(1))
			o.Expect(cm.Ports[0].Direction).To(o.Equal("Ingress"))
			o.Expect(cm.Ports[0].Protocol).To(o.Equal("TCP"))
			o.Expect(cm.Ports[0].Port).To(o.Equal(443))
			o.Expect(cm.Ports[0].Namespace).To(o.Equal("ns1"))
			o.Expect(cm.Ports[0].Service).To(o.Equal("svc1"))
			o.Expect(cm.Ports[0].Pod).To(o.Equal("pod1"))
			o.Expect(cm.Ports[0].Container).To(o.Equal("ctr1"))
			o.Expect(cm.Ports[0].NodeGroup).To(o.Equal("master"))
			o.Expect(cm.Ports[0].Optional).To(o.BeFalse())

			// Dynamic ranges
			o.Expect(cm.DynamicRanges).To(o.HaveLen(2))

			// First range
			o.Expect(cm.DynamicRanges[0].Direction).To(o.Equal("Egress"))
			o.Expect(cm.DynamicRanges[0].Protocol).To(o.Equal("UDP"))
			o.Expect(cm.DynamicRanges[0].MinPort).To(o.Equal(30000))
			o.Expect(cm.DynamicRanges[0].MaxPort).To(o.Equal(30100))
			o.Expect(cm.DynamicRanges[0].Description).To(o.Equal("NodePort range"))
			o.Expect(cm.DynamicRanges[0].Optional).To(o.BeTrue())

			// Second range
			o.Expect(cm.DynamicRanges[1].Direction).To(o.Equal("Ingress"))
			o.Expect(cm.DynamicRanges[1].Protocol).To(o.Equal("TCP"))
			o.Expect(cm.DynamicRanges[1].MinPort).To(o.Equal(49152))
			o.Expect(cm.DynamicRanges[1].MaxPort).To(o.Equal(65535))
			o.Expect(cm.DynamicRanges[1].Description).To(o.Equal("Linux ephemeral range"))
			o.Expect(cm.DynamicRanges[1].Optional).To(o.BeTrue())
		})

		g.It("errors on malformed dynamic range in CSV", func() {
			csvContent := []byte(
				"Direction,Protocol,Port,Namespace,Service,Pod,Container,NodeGroup,Optional\n" +
					"Ingress,TCP,10-20-30,,,,,,false\n",
			)
			_, err := parseCSVToComMatrix(csvContent)
			o.Expect(err).To(o.HaveOccurred())
		})
	})
})

var _ = g.Describe("Butane and MachineConfig output formats", func() {
	var mat ComMatrix

	g.BeforeEach(func() {
		mat = ComMatrix{
			Ports: []ComDetails{
				{Direction: "Ingress", Protocol: "TCP", Port: 443, Namespace: "ns1", Service: "svc1", Pod: "pod1", Container: "ctr1", NodeGroup: "master", Optional: false},
				{Direction: "Ingress", Protocol: "UDP", Port: 53, Namespace: "ns2", Service: "svc2", Pod: "pod2", Container: "ctr2", NodeGroup: "master", Optional: false},
			},
		}
	})

	g.Describe("ToButane", func() {
		g.It("produces valid Butane YAML with the correct pool name and nftables rules", func() {
			out, err := mat.ToButane("master", fakeUtils{version: "4.17"})
			o.Expect(err).ToNot(o.HaveOccurred())

			result := string(out)
			o.Expect(result).To(o.ContainSubstring("variant: openshift"))
			o.Expect(result).To(o.ContainSubstring("version: 4.17.0"))
			o.Expect(result).To(o.ContainSubstring("name: 98-nftables-commatrix-master"))
			o.Expect(result).To(o.ContainSubstring("machineconfiguration.openshift.io/role: master"))
			o.Expect(result).To(o.ContainSubstring("nftables.service"))
			o.Expect(result).To(o.ContainSubstring("/etc/sysconfig/nftables.conf"))
			o.Expect(result).To(o.ContainSubstring("tcp dport { 443 }"))
			o.Expect(result).To(o.ContainSubstring("udp dport { 53 }"))
		})

		g.It("uses the correct pool name for worker", func() {
			mat.Ports[0].NodeGroup = "worker"
			mat.Ports[1].NodeGroup = "worker"

			out, err := mat.ToButane("worker", fakeUtils{version: "4.17"})
			o.Expect(err).ToNot(o.HaveOccurred())

			result := string(out)
			o.Expect(result).To(o.ContainSubstring("name: 98-nftables-commatrix-worker"))
			o.Expect(result).To(o.ContainSubstring("machineconfiguration.openshift.io/role: worker"))
		})

		g.It("derives the butane version from the cluster version", func() {
			out, err := mat.ToButane("master", fakeUtils{version: "4.21"})
			o.Expect(err).ToNot(o.HaveOccurred())
			o.Expect(string(out)).To(o.ContainSubstring("version: 4.21.0"))
		})

		g.It("strips the shebang line from nftables rules", func() {
			out, err := mat.ToButane("master", fakeUtils{version: "4.17"})
			o.Expect(err).ToNot(o.HaveOccurred())
			o.Expect(string(out)).ToNot(o.ContainSubstring("#!/usr/sbin/nft"))
		})
	})

	g.Describe("ToMachineConfig", func() {
		g.It("produces valid MachineConfig YAML with the correct pool name", func() {
			out, err := mat.ToMachineConfig("master", fakeUtils{version: "4.17"})
			o.Expect(err).ToNot(o.HaveOccurred())

			result := string(out)
			o.Expect(result).To(o.ContainSubstring("kind: MachineConfig"))
			o.Expect(result).To(o.ContainSubstring("name: 98-nftables-commatrix-master"))
			o.Expect(result).To(o.ContainSubstring("machineconfiguration.openshift.io/role: master"))
		})

		g.It("embeds the nftables rules in the ignition storage section", func() {
			out, err := mat.ToMachineConfig("master", fakeUtils{version: "4.17"})
			o.Expect(err).ToNot(o.HaveOccurred())

			result := string(out)
			o.Expect(result).To(o.ContainSubstring("storage:"))
			o.Expect(result).To(o.ContainSubstring("/etc/sysconfig/nftables.conf"))
		})

		g.It("includes the nftables systemd unit", func() {
			out, err := mat.ToMachineConfig("master", fakeUtils{version: "4.17"})
			o.Expect(err).ToNot(o.HaveOccurred())

			result := string(out)
			o.Expect(result).To(o.ContainSubstring("systemd:"))
			o.Expect(result).To(o.ContainSubstring("nftables.service"))
		})
	})

	g.Describe("ToButane and ToMachineConfig with dynamic ranges", func() {
		g.It("includes dynamic port ranges in the output", func() {
			mat.DynamicRanges = []DynamicRange{
				{Direction: "Ingress", Protocol: "TCP", MinPort: 30000, MaxPort: 32767, Description: "NodePort range", Optional: true},
			}

			butaneOut, err := mat.ToButane("master", fakeUtils{version: "4.17"})
			o.Expect(err).ToNot(o.HaveOccurred())
			o.Expect(string(butaneOut)).To(o.ContainSubstring("443, 30000-32767"))

			mcOut, err := mat.ToMachineConfig("master", fakeUtils{version: "4.17"})
			o.Expect(err).ToNot(o.HaveOccurred())
			o.Expect(mcOut).ToNot(o.BeEmpty())
		})
	})

	g.Describe("print dispatches butane and mc formats", func() {
		g.It("returns butane output for FormatButane", func() {
			out, err := mat.print(FormatButane, "master", fakeUtils{version: "4.17"})
			o.Expect(err).ToNot(o.HaveOccurred())
			o.Expect(string(out)).To(o.ContainSubstring("variant: openshift"))
		})

		g.It("returns MachineConfig output for FormatMC", func() {
			out, err := mat.print(FormatMC, "master", fakeUtils{version: "4.17"})
			o.Expect(err).ToNot(o.HaveOccurred())
			o.Expect(string(out)).To(o.ContainSubstring("kind: MachineConfig"))
		})

		g.It("returns error for invalid format", func() {
			_, err := mat.print("invalid", "", nil)
			o.Expect(err).To(o.HaveOccurred())
			o.Expect(err.Error()).To(o.ContainSubstring("invalid format"))
		})
	})
})

var _ = g.Describe("ApplyCustomNodeGroupOverrides", func() {
	g.It("reassigns nodes to the specified custom group", func() {
		nodeToGroup := map[string]string{
			"master1": "master",
			"worker1": "worker",
			"worker2": "worker",
			"worker3": "worker",
		}
		customNodeGroups := map[string][]string{
			"mc-ingress": {"worker1", "worker2"},
		}
		err := ApplyCustomNodeGroupOverrides(nodeToGroup, customNodeGroups)
		o.Expect(err).ToNot(o.HaveOccurred())
		o.Expect(nodeToGroup["master1"]).To(o.Equal("master"))
		o.Expect(nodeToGroup["worker1"]).To(o.Equal("mc-ingress"))
		o.Expect(nodeToGroup["worker2"]).To(o.Equal("mc-ingress"))
		o.Expect(nodeToGroup["worker3"]).To(o.Equal("worker"))
	})

	g.It("supports multiple custom groups", func() {
		nodeToGroup := map[string]string{
			"worker1": "worker",
			"worker2": "worker",
			"worker3": "worker",
		}
		customNodeGroups := map[string][]string{
			"mc-ingress": {"worker1"},
			"mc-storage": {"worker2"},
		}
		err := ApplyCustomNodeGroupOverrides(nodeToGroup, customNodeGroups)
		o.Expect(err).ToNot(o.HaveOccurred())
		o.Expect(nodeToGroup["worker1"]).To(o.Equal("mc-ingress"))
		o.Expect(nodeToGroup["worker2"]).To(o.Equal("mc-storage"))
		o.Expect(nodeToGroup["worker3"]).To(o.Equal("worker"))
	})

	g.It("returns error when node is not in the cluster", func() {
		nodeToGroup := map[string]string{
			"worker1": "worker",
		}
		customNodeGroups := map[string][]string{
			"mc-ingress": {"nonexistent-node"},
		}
		err := ApplyCustomNodeGroupOverrides(nodeToGroup, customNodeGroups)
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(err.Error()).To(o.ContainSubstring("not found in cluster"))
	})

	g.It("is a no-op when customNodeGroups is nil", func() {
		nodeToGroup := map[string]string{
			"worker1": "worker",
		}
		err := ApplyCustomNodeGroupOverrides(nodeToGroup, nil)
		o.Expect(err).ToNot(o.HaveOccurred())
		o.Expect(nodeToGroup["worker1"]).To(o.Equal("worker"))
	})

	g.It("is a no-op when customNodeGroups is empty", func() {
		nodeToGroup := map[string]string{
			"worker1": "worker",
		}
		err := ApplyCustomNodeGroupOverrides(nodeToGroup, map[string][]string{})
		o.Expect(err).ToNot(o.HaveOccurred())
		o.Expect(nodeToGroup["worker1"]).To(o.Equal("worker"))
	})
})

var _ = g.Describe("SeparateMatrixByGroup", func() {
	g.It("separates entries by node group and preserves dynamic ranges", func() {
		mat := ComMatrix{
			Ports: []ComDetails{
				{Port: 443, Protocol: "TCP", NodeGroup: "master"},
				{Port: 80, Protocol: "TCP", NodeGroup: "worker"},
				{Port: 53, Protocol: "UDP", NodeGroup: "master"},
				{Port: 0, Protocol: "TCP", NodeGroup: ""},
			},
			DynamicRanges: []DynamicRange{
				{Protocol: "TCP", MinPort: 30000, MaxPort: 32767},
			},
		}

		pools := mat.SeparateMatrixByGroup()
		o.Expect(pools).To(o.HaveLen(2))
		o.Expect(pools["master"].Ports).To(o.HaveLen(2))
		o.Expect(pools["worker"].Ports).To(o.HaveLen(1))
		o.Expect(pools["master"].DynamicRanges).To(o.HaveLen(1))
		o.Expect(pools["worker"].DynamicRanges).To(o.HaveLen(1))
	})
})
