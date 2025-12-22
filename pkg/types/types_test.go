package types

import (
	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
)

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
