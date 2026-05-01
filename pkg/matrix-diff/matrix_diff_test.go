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
	It("returns an empty diff when both matrices are empty", func() {
		primary := &types.ComMatrix{}
		secondary := &types.ComMatrix{}

		diff := Generate(primary, secondary)

		Expect(diff.Ports).To(BeEmpty())
		Expect(diff.GetUniquePrimary().Ports).To(BeEmpty())
		Expect(diff.GetUniqueSecondary().Ports).To(BeEmpty())
		Expect(diff.GetSharedEntries().Ports).To(BeEmpty())
	})

	It("marks all entries as uniquePrimary when secondary is empty", func() {
		primary := &types.ComMatrix{
			Ports: []types.ComDetails{cdIngressTCP443Master, cdEgressUDP53Worker},
		}
		secondary := &types.ComMatrix{}

		diff := Generate(primary, secondary)

		Expect(diff.GetUniquePrimary().Ports).To(HaveLen(2))
		Expect(diff.GetUniqueSecondary().Ports).To(BeEmpty())
		Expect(diff.GetSharedEntries().Ports).To(BeEmpty())
	})

	It("marks all entries as uniqueSecondary when primary is empty", func() {
		primary := &types.ComMatrix{}
		secondary := &types.ComMatrix{
			Ports: []types.ComDetails{cdIngressTCP443Master, cdEgressUDP53Worker},
		}

		diff := Generate(primary, secondary)

		Expect(diff.GetUniquePrimary().Ports).To(BeEmpty())
		Expect(diff.GetUniqueSecondary().Ports).To(HaveLen(2))
		Expect(diff.GetSharedEntries().Ports).To(BeEmpty())
	})

	It("correctly categorizes shared and unique entries", func() {
		primary := &types.ComMatrix{
			Ports: []types.ComDetails{cdIngressTCP443Master, cdEgressUDP53Worker},
		}
		secondary := &types.ComMatrix{
			Ports: []types.ComDetails{cdEgressUDP53Worker, cdIngressTCP8080Worker},
		}

		diff := Generate(primary, secondary)

		Expect(diff.GetSharedEntries().Ports).To(HaveLen(1))
		Expect(diff.GetSharedEntries().Ports[0].Port).To(Equal(53))

		Expect(diff.GetUniquePrimary().Ports).To(HaveLen(1))
		Expect(diff.GetUniquePrimary().Ports[0].Port).To(Equal(443))

		Expect(diff.GetUniqueSecondary().Ports).To(HaveLen(1))
		Expect(diff.GetUniqueSecondary().Ports[0].Port).To(Equal(8080))
	})

	It("marks all entries as shared when matrices are identical", func() {
		ports := []types.ComDetails{cdIngressTCP443Master, cdEgressUDP53Worker}
		primary := &types.ComMatrix{Ports: ports}
		secondary := &types.ComMatrix{Ports: ports}

		diff := Generate(primary, secondary)

		Expect(diff.GetSharedEntries().Ports).To(HaveLen(2))
		Expect(diff.GetUniquePrimary().Ports).To(BeEmpty())
		Expect(diff.GetUniqueSecondary().Ports).To(BeEmpty())
	})

	It("deduplicates entries in the combined matrix", func() {
		primary := &types.ComMatrix{
			Ports: []types.ComDetails{cdIngressTCP443Master, cdIngressTCP443Master},
		}
		secondary := &types.ComMatrix{
			Ports: []types.ComDetails{cdIngressTCP443Master},
		}

		diff := Generate(primary, secondary)

		Expect(diff.Ports).To(HaveLen(1))
		Expect(diff.GetSharedEntries().Ports).To(HaveLen(1))
	})
})

var _ = Describe("String", func() {
	It("returns only the header for an empty diff", func() {
		diff := Generate(&types.ComMatrix{}, &types.ComMatrix{})

		str, err := diff.String()
		Expect(err).ToNot(HaveOccurred())

		lines := strings.Split(strings.TrimSpace(str), "\n")
		Expect(lines).To(HaveLen(1))
		Expect(lines[0]).To(ContainSubstring("Direction"))
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

		lines := strings.Split(strings.TrimSpace(str), "\n")
		var plusLines, minusLines, plainLines int
		for _, line := range lines[1:] {
			switch {
			case strings.HasPrefix(line, "+ "):
				plusLines++
			case strings.HasPrefix(line, "- "):
				minusLines++
			default:
				plainLines++
			}
		}

		Expect(plusLines).To(Equal(1))
		Expect(minusLines).To(Equal(1))
		Expect(plainLines).To(Equal(1))
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
