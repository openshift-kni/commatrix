package matrixdiff

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/openshift-kni/commatrix/pkg/types"
)

var (
	cdIngressTCP443Master = types.ComDetails{
		Direction: "Ingress", Protocol: "TCP", Port: 443,
		Namespace: "openshift-apiserver", Service: "apiserver",
		Pod: "apiserver-pod", Container: "apiserver",
		NodeGroup: "master", Optional: false,
	}
	cdEgressUDP53Worker = types.ComDetails{
		Direction: "Egress", Protocol: "UDP", Port: 53,
		Namespace: "openshift-dns", Service: "dns-default",
		Pod: "dns-pod", Container: "dns",
		NodeGroup: "worker", Optional: false,
	}
	cdIngressTCP8080Worker = types.ComDetails{
		Direction: "Ingress", Protocol: "TCP", Port: 8080,
		Namespace: "openshift-ingress", Service: "router",
		Pod: "router-pod", Container: "router",
		NodeGroup: "worker", Optional: true,
	}
)

var _ = Describe("Generate", func() {
	DescribeTable("categorizes entries correctly",
		func(primary, secondary *types.ComMatrix,
			expectedPorts, expectedUniquePrimary, expectedUniqueSecondary, expectedShared []types.ComDetails,
		) {
			diff := Generate(primary, secondary)

			Expect(diff.Ports).To(ConsistOf(expectedPorts))
			Expect(diff.GetUniquePrimary().Ports).To(ConsistOf(expectedUniquePrimary))
			Expect(diff.GetUniqueSecondary().Ports).To(ConsistOf(expectedUniqueSecondary))
			Expect(diff.GetSharedEntries().Ports).To(ConsistOf(expectedShared))
		},
		Entry("both matrices are empty",
			&types.ComMatrix{},
			&types.ComMatrix{},
			nil, nil, nil, nil,
		),
		Entry("all entries unique to primary when secondary is empty",
			&types.ComMatrix{Ports: []types.ComDetails{cdIngressTCP443Master, cdEgressUDP53Worker}},
			&types.ComMatrix{},
			[]types.ComDetails{cdIngressTCP443Master, cdEgressUDP53Worker},
			[]types.ComDetails{cdIngressTCP443Master, cdEgressUDP53Worker},
			nil, nil,
		),
		Entry("all entries unique to secondary when primary is empty",
			&types.ComMatrix{},
			&types.ComMatrix{Ports: []types.ComDetails{cdIngressTCP443Master, cdEgressUDP53Worker}},
			[]types.ComDetails{cdIngressTCP443Master, cdEgressUDP53Worker},
			nil,
			[]types.ComDetails{cdIngressTCP443Master, cdEgressUDP53Worker},
			nil,
		),
		Entry("shared and unique entries are correctly categorized",
			&types.ComMatrix{Ports: []types.ComDetails{cdIngressTCP443Master, cdEgressUDP53Worker}},
			&types.ComMatrix{Ports: []types.ComDetails{cdEgressUDP53Worker, cdIngressTCP8080Worker}},
			[]types.ComDetails{cdIngressTCP443Master, cdEgressUDP53Worker, cdIngressTCP8080Worker},
			[]types.ComDetails{cdIngressTCP443Master},
			[]types.ComDetails{cdIngressTCP8080Worker},
			[]types.ComDetails{cdEgressUDP53Worker},
		),
		Entry("all entries shared when matrices are identical",
			&types.ComMatrix{Ports: []types.ComDetails{cdIngressTCP443Master, cdEgressUDP53Worker}},
			&types.ComMatrix{Ports: []types.ComDetails{cdIngressTCP443Master, cdEgressUDP53Worker}},
			[]types.ComDetails{cdIngressTCP443Master, cdEgressUDP53Worker},
			nil, nil,
			[]types.ComDetails{cdIngressTCP443Master, cdEgressUDP53Worker},
		),
		Entry("duplicates in primary are deduplicated",
			&types.ComMatrix{Ports: []types.ComDetails{cdIngressTCP443Master, cdIngressTCP443Master}},
			&types.ComMatrix{Ports: []types.ComDetails{cdIngressTCP443Master}},
			[]types.ComDetails{cdIngressTCP443Master},
			nil, nil,
			[]types.ComDetails{cdIngressTCP443Master},
		),
	)
})

var _ = Describe("String", func() {
	var header string

	BeforeEach(func() {
		var err error
		header, err = types.GetComMatrixHeadersByFormat(types.FormatCSV)
		Expect(err).ToNot(HaveOccurred())
	})

	It("returns only the header for an empty diff", func() {
		diff := Generate(&types.ComMatrix{}, &types.ComMatrix{})

		str, err := diff.String()
		Expect(err).ToNot(HaveOccurred())
		Expect(str).To(Equal(header + "\n"))
	})

	It("prefixes unique primary entries with '+ ' and unique secondary with '- '", func() {
		primary := &types.ComMatrix{
			Ports: []types.ComDetails{cdIngressTCP443Master, cdEgressUDP53Worker},
		}
		secondary := &types.ComMatrix{
			Ports: []types.ComDetails{cdEgressUDP53Worker, cdIngressTCP8080Worker},
		}

		diff := Generate(primary, secondary)
		str, err := diff.String()
		Expect(err).ToNot(HaveOccurred())

		lines := strings.Split(strings.TrimSuffix(str, "\n"), "\n")
		Expect(lines[0]).To(Equal(header))
		Expect(lines[1:]).To(ConsistOf(
			"+ "+cdIngressTCP443Master.String(),
			"- "+cdIngressTCP8080Worker.String(),
			cdEgressUDP53Worker.String(),
		))
	})
})

var _ = Describe("GetUniquePrimary", func() {
	It("returns an empty matrix when all entries are shared", func() {
		ports := []types.ComDetails{cdIngressTCP443Master}
		diff := Generate(
			&types.ComMatrix{Ports: ports},
			&types.ComMatrix{Ports: ports},
		)

		Expect(diff.GetUniquePrimary().Ports).To(BeEmpty())
	})
})

var _ = Describe("GetUniqueSecondary", func() {
	It("returns an empty matrix when all entries are shared", func() {
		ports := []types.ComDetails{cdIngressTCP443Master}
		diff := Generate(
			&types.ComMatrix{Ports: ports},
			&types.ComMatrix{Ports: ports},
		)

		Expect(diff.GetUniqueSecondary().Ports).To(BeEmpty())
	})
})

var _ = Describe("GetSharedEntries", func() {
	It("returns an empty matrix when no entries are shared", func() {
		diff := Generate(
			&types.ComMatrix{Ports: []types.ComDetails{cdIngressTCP443Master}},
			&types.ComMatrix{Ports: []types.ComDetails{cdEgressUDP53Worker}},
		)

		Expect(diff.GetSharedEntries().Ports).To(BeEmpty())
	})
})
