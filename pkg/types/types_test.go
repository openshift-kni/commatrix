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
			mat.DynamicRanges = DynamicRangeList{
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

var _ = g.Describe("SeparateMatrixByGroup", func() {
	g.It("separates entries by node group and preserves dynamic ranges", func() {
		mat := ComMatrix{
			Ports: []ComDetails{
				{Port: 443, Protocol: "TCP", NodeGroup: "master"},
				{Port: 80, Protocol: "TCP", NodeGroup: "worker"},
				{Port: 53, Protocol: "UDP", NodeGroup: "master"},
				{Port: 0, Protocol: "TCP", NodeGroup: ""},
			},
			DynamicRanges: DynamicRangeList{
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

var _ = g.Describe("Merge", func() {
	g.It("merges two non-nil matrices with ports and ranges", func() {
		m1 := &ComMatrix{
			Ports: []ComDetails{
				{Port: 443, Protocol: "TCP", NodeGroup: "master"},
				{Port: 80, Protocol: "TCP", NodeGroup: "worker"},
			},
			DynamicRanges: DynamicRangeList{
				{Protocol: "TCP", MinPort: 30000, MaxPort: 32767},
			},
		}
		m2 := &ComMatrix{
			Ports: []ComDetails{
				{Port: 53, Protocol: "UDP", NodeGroup: "master"},
			},
			DynamicRanges: DynamicRangeList{
				{Protocol: "UDP", MinPort: 49152, MaxPort: 65535},
			},
		}

		result := m1.Merge(m2)
		o.Expect(result).ToNot(o.BeNil())
		o.Expect(result.Ports).To(o.ContainElements(
			ComDetails{Port: 443, Protocol: "TCP", NodeGroup: "master"},
			ComDetails{Port: 80, Protocol: "TCP", NodeGroup: "worker"},
			ComDetails{Port: 53, Protocol: "UDP", NodeGroup: "master"},
		))
		o.Expect(result.DynamicRanges).To(o.ContainElements(
			DynamicRange{Protocol: "TCP", MinPort: 30000, MaxPort: 32767},
			DynamicRange{Protocol: "UDP", MinPort: 49152, MaxPort: 65535},
		))
	})

	g.It("returns other when m is nil", func() {
		var m1 *ComMatrix
		m2 := &ComMatrix{
			Ports: []ComDetails{
				{Port: 443, Protocol: "TCP", NodeGroup: "master"},
			},
		}

		result := m1.Merge(m2)
		o.Expect(result).To(o.Equal(m2))
	})

	g.It("returns m when other is nil", func() {
		m1 := &ComMatrix{
			Ports: []ComDetails{
				{Port: 443, Protocol: "TCP", NodeGroup: "master"},
			},
		}
		var m2 *ComMatrix

		result := m1.Merge(m2)
		o.Expect(result).To(o.Equal(m1))
	})

	g.It("returns empty matrix when both are nil", func() {
		var m1 *ComMatrix
		var m2 *ComMatrix

		result := m1.Merge(m2)
		o.Expect(*result).To(o.Equal(ComMatrix{}))
		o.Expect(result.Ports).To(o.BeNil())
		o.Expect(result.DynamicRanges).To(o.BeNil())
	})

	g.It("handles nil slices in m", func() {
		m1 := &ComMatrix{
			Ports:         nil,
			DynamicRanges: nil,
		}
		m2 := &ComMatrix{
			Ports: []ComDetails{
				{Port: 443, Protocol: "TCP", NodeGroup: "master"},
			},
		}

		result := m1.Merge(m2)
		o.Expect(result).ToNot(o.BeNil())
		o.Expect(result.Ports).To(o.Equal([]ComDetails{
			{Port: 443, Protocol: "TCP", NodeGroup: "master"},
		}))
	})

	g.It("removes duplicates after merge", func() {
		m1 := &ComMatrix{
			Ports: []ComDetails{
				{Port: 443, Protocol: "TCP", NodeGroup: "master"},
			},
		}
		m2 := &ComMatrix{
			Ports: []ComDetails{
				{Port: 443, Protocol: "TCP", NodeGroup: "master"},
				{Port: 80, Protocol: "TCP", NodeGroup: "master"},
			},
		}

		result := m1.Merge(m2)
		o.Expect(result).ToNot(o.BeNil())
		o.Expect(result.Ports).To(o.ConsistOf(
			ComDetails{Port: 443, Protocol: "TCP", NodeGroup: "master"},
			ComDetails{Port: 80, Protocol: "TCP", NodeGroup: "master"},
		))
	})
})

var _ = g.Describe("Squash", func() {
	g.It("does nothing with empty list", func() {
		var drl DynamicRangeList
		drl.Squash()
		o.Expect(drl).To(o.BeEmpty())
	})

	g.It("does nothing with single range", func() {
		drl := DynamicRangeList{
			{Direction: "Ingress", Protocol: "TCP", MinPort: 30000, MaxPort: 32767},
		}
		drl.Squash()
		o.Expect(drl).To(o.HaveLen(1))
		o.Expect(drl[0].MinPort).To(o.Equal(30000))
		o.Expect(drl[0].MaxPort).To(o.Equal(32767))
	})

	g.It("merges overlapping ranges with same Direction and Protocol", func() {
		drl := DynamicRangeList{
			{Direction: "Ingress", Protocol: "TCP", MinPort: 30000, MaxPort: 40000},
			{Direction: "Ingress", Protocol: "TCP", MinPort: 35000, MaxPort: 50000},
		}
		drl.Squash()
		o.Expect(drl).To(o.HaveLen(1))
		o.Expect(drl[0].Direction).To(o.Equal("Ingress"))
		o.Expect(drl[0].Protocol).To(o.Equal("TCP"))
		o.Expect(drl[0].MinPort).To(o.Equal(30000))
		o.Expect(drl[0].MaxPort).To(o.Equal(50000))
	})

	g.It("merges adjacent ranges with same Direction and Protocol", func() {
		drl := DynamicRangeList{
			{Direction: "Egress", Protocol: "UDP", MinPort: 10000, MaxPort: 20000},
			{Direction: "Egress", Protocol: "UDP", MinPort: 20001, MaxPort: 30000},
		}
		drl.Squash()
		o.Expect(drl).To(o.HaveLen(1))
		o.Expect(drl[0].Direction).To(o.Equal("Egress"))
		o.Expect(drl[0].Protocol).To(o.Equal("UDP"))
		o.Expect(drl[0].MinPort).To(o.Equal(10000))
		o.Expect(drl[0].MaxPort).To(o.Equal(30000))
	})

	g.It("keeps separate ranges with different Direction", func() {
		drl := DynamicRangeList{
			{Direction: "Ingress", Protocol: "TCP", MinPort: 30000, MaxPort: 40000},
			{Direction: "Egress", Protocol: "TCP", MinPort: 30000, MaxPort: 40000},
		}
		drl.Squash()
		o.Expect(drl).To(o.HaveLen(2))
		o.Expect(drl[0].Direction).To(o.Equal("Egress"))
		o.Expect(drl[1].Direction).To(o.Equal("Ingress"))
	})

	g.It("keeps separate ranges with different Protocol", func() {
		drl := DynamicRangeList{
			{Direction: "Ingress", Protocol: "TCP", MinPort: 30000, MaxPort: 40000},
			{Direction: "Ingress", Protocol: "UDP", MinPort: 30000, MaxPort: 40000},
		}
		drl.Squash()
		o.Expect(drl).To(o.HaveLen(2))
		o.Expect(drl[0].Protocol).To(o.Equal("TCP"))
		o.Expect(drl[1].Protocol).To(o.Equal("UDP"))
	})

	g.It("keeps separate non-overlapping ranges with same Direction and Protocol", func() {
		dra := DynamicRange{Direction: "Ingress", Protocol: "TCP", MinPort: 10000, MaxPort: 20000}
		drb := DynamicRange{Direction: "Ingress", Protocol: "TCP", MinPort: 30000, MaxPort: 40000}
		drl := DynamicRangeList{dra, drb}
		drl.Squash()
		o.Expect(drl).To(o.HaveLen(2))
		o.Expect(drl[0]).To(o.Equal(dra))
		o.Expect(drl[1]).To(o.Equal(drb))
	})

	g.It("merges multiple overlapping ranges", func() {
		drl := DynamicRangeList{
			{Direction: "Ingress", Protocol: "TCP", MinPort: 10000, MaxPort: 20000},
			{Direction: "Ingress", Protocol: "TCP", MinPort: 15000, MaxPort: 25000},
			{Direction: "Ingress", Protocol: "TCP", MinPort: 22000, MaxPort: 30000},
		}
		drl.Squash()
		o.Expect(drl).To(o.HaveLen(1))
		o.Expect(drl[0].MinPort).To(o.Equal(10000))
		o.Expect(drl[0].MaxPort).To(o.Equal(30000))
	})

	g.It("handles complex scenario with mixed ranges", func() {
		drl := DynamicRangeList{
			{Direction: "Ingress", Protocol: "TCP", MinPort: 10000, MaxPort: 20000},
			{Direction: "Ingress", Protocol: "TCP", MinPort: 25000, MaxPort: 30000},
			{Direction: "Ingress", Protocol: "UDP", MinPort: 10000, MaxPort: 15000},
			{Direction: "Ingress", Protocol: "UDP", MinPort: 14000, MaxPort: 20000},
			{Direction: "Egress", Protocol: "TCP", MinPort: 10000, MaxPort: 15000},
		}
		drl.Squash()
		o.Expect(drl).To(o.HaveLen(4))
		// Egress TCP
		o.Expect(drl[0].Direction).To(o.Equal("Egress"))
		o.Expect(drl[0].Protocol).To(o.Equal("TCP"))
		o.Expect(drl[0].MinPort).To(o.Equal(10000))
		o.Expect(drl[0].MaxPort).To(o.Equal(15000))
		// Ingress TCP - first range
		o.Expect(drl[1].Direction).To(o.Equal("Ingress"))
		o.Expect(drl[1].Protocol).To(o.Equal("TCP"))
		o.Expect(drl[1].MinPort).To(o.Equal(10000))
		o.Expect(drl[1].MaxPort).To(o.Equal(20000))
		// Ingress TCP - second range
		o.Expect(drl[2].Direction).To(o.Equal("Ingress"))
		o.Expect(drl[2].Protocol).To(o.Equal("TCP"))
		o.Expect(drl[2].MinPort).To(o.Equal(25000))
		o.Expect(drl[2].MaxPort).To(o.Equal(30000))
		// Ingress UDP - merged
		o.Expect(drl[3].Direction).To(o.Equal("Ingress"))
		o.Expect(drl[3].Protocol).To(o.Equal("UDP"))
		o.Expect(drl[3].MinPort).To(o.Equal(10000))
		o.Expect(drl[3].MaxPort).To(o.Equal(20000))
	})

	g.It("sorts ranges before merging", func() {
		drl := DynamicRangeList{
			{Direction: "Ingress", Protocol: "TCP", MinPort: 25000, MaxPort: 30000},
			{Direction: "Egress", Protocol: "TCP", MinPort: 10000, MaxPort: 15000},
			{Direction: "Ingress", Protocol: "TCP", MinPort: 10000, MaxPort: 20000},
		}
		drl.Squash()
		o.Expect(drl).To(o.HaveLen(3))
		o.Expect(drl[0].Direction).To(o.Equal("Egress"))
		o.Expect(drl[1].MinPort).To(o.Equal(10000))
		o.Expect(drl[2].MinPort).To(o.Equal(25000))
	})

	g.It("merges descriptions when combining ranges", func() {
		drl := DynamicRangeList{
			{Direction: "Ingress", Protocol: "TCP", MinPort: 10000, MaxPort: 20000, Description: "First range"},
			{Direction: "Ingress", Protocol: "TCP", MinPort: 15000, MaxPort: 25000, Description: "Second range"},
		}
		drl.Squash()
		o.Expect(drl).To(o.HaveLen(1))
		o.Expect(drl[0].Description).To(o.Equal("First range Second range"))
		o.Expect(drl[0].MinPort).To(o.Equal(10000))
		o.Expect(drl[0].MaxPort).To(o.Equal(25000))
	})

	g.It("merges multiple descriptions when combining multiple ranges", func() {
		drl := DynamicRangeList{
			{Direction: "Ingress", Protocol: "TCP", MinPort: 10000, MaxPort: 20000, Description: "Range A"},
			{Direction: "Ingress", Protocol: "TCP", MinPort: 18000, MaxPort: 28000, Description: "Range B"},
			{Direction: "Ingress", Protocol: "TCP", MinPort: 25000, MaxPort: 35000, Description: "Range C"},
		}
		drl.Squash()
		o.Expect(drl).To(o.HaveLen(1))
		o.Expect(drl[0].Description).To(o.Equal("Range A Range B Range C"))
	})

	g.It("sets result as optional when merging two optional ranges", func() {
		drl := DynamicRangeList{
			{Direction: "Ingress", Protocol: "TCP", MinPort: 10000, MaxPort: 20000, Optional: true},
			{Direction: "Ingress", Protocol: "TCP", MinPort: 15000, MaxPort: 25000, Optional: true},
		}
		drl.Squash()
		o.Expect(drl).To(o.HaveLen(1))
		o.Expect(drl[0].Optional).To(o.BeTrue())
	})

	g.It("sets result as mandatory when merging one optional and one mandatory range", func() {
		drl := DynamicRangeList{
			{Direction: "Ingress", Protocol: "TCP", MinPort: 10000, MaxPort: 20000, Optional: true},
			{Direction: "Ingress", Protocol: "TCP", MinPort: 15000, MaxPort: 25000, Optional: false},
		}
		drl.Squash()
		o.Expect(drl).To(o.HaveLen(1))
		o.Expect(drl[0].Optional).To(o.BeFalse())
	})

	g.It("sets result as mandatory when merging two mandatory ranges", func() {
		drl := DynamicRangeList{
			{Direction: "Ingress", Protocol: "TCP", MinPort: 10000, MaxPort: 20000, Optional: false},
			{Direction: "Ingress", Protocol: "TCP", MinPort: 15000, MaxPort: 25000, Optional: false},
		}
		drl.Squash()
		o.Expect(drl).To(o.HaveLen(1))
		o.Expect(drl[0].Optional).To(o.BeFalse())
	})
})

var _ = g.Describe("Merge", func() {
	g.It("fails when ranges are out of order (next starts before dr)", func() {
		dr := DynamicRange{Direction: "Ingress", Protocol: "TCP", MinPort: 100, MaxPort: 200, Description: "First"}
		next := DynamicRange{Direction: "Ingress", Protocol: "TCP", MinPort: 50, MaxPort: 150, Description: "Second"}

		result := dr.Merge(next)
		o.Expect(result).To(o.BeFalse())
		o.Expect(dr.MinPort).To(o.Equal(100))
		o.Expect(dr.MaxPort).To(o.Equal(200))
		o.Expect(dr.Description).To(o.Equal("First"))
	})

	g.It("succeeds with valid overlap", func() {
		dr := DynamicRange{Direction: "Ingress", Protocol: "TCP", MinPort: 100, MaxPort: 200, Description: "First", Optional: true}
		next := DynamicRange{Direction: "Ingress", Protocol: "TCP", MinPort: 150, MaxPort: 250, Description: "Second", Optional: false}

		result := dr.Merge(next)
		o.Expect(result).To(o.BeTrue())
		o.Expect(dr.MinPort).To(o.Equal(100))
		o.Expect(dr.MaxPort).To(o.Equal(250))
		o.Expect(dr.Description).To(o.Equal("First Second"))
		o.Expect(dr.Optional).To(o.BeFalse())
	})

	g.It("fails when ranges are apart (no overlap)", func() {
		dr := DynamicRange{Direction: "Ingress", Protocol: "TCP", MinPort: 100, MaxPort: 200, Description: "First"}
		next := DynamicRange{Direction: "Ingress", Protocol: "TCP", MinPort: 300, MaxPort: 400, Description: "Second"}

		result := dr.Merge(next)
		o.Expect(result).To(o.BeFalse())
		o.Expect(dr.MinPort).To(o.Equal(100))
		o.Expect(dr.MaxPort).To(o.Equal(200))
		o.Expect(dr.Description).To(o.Equal("First"))
	})
})
