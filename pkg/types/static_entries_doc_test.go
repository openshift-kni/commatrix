package types

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	configv1 "github.com/openshift/api/config/v1"
)

func readDocCSV(filename string) (*ComMatrix, error) {
	_, testFile, _, ok := runtime.Caller(0)
	if !ok {
		return nil, fmt.Errorf("failed to determine test file path")
	}
	repoRoot := filepath.Join(filepath.Dir(testFile), "..", "..")
	csvPath := filepath.Join(repoRoot, "docs", "stable", "raw", filename)

	content, err := os.ReadFile(csvPath)
	if err != nil {
		return nil, err
	}

	// Doc CSVs use "Node Role" as header; the CSV parser expects "NodeGroup".
	fixed := strings.Replace(string(content), "Node Role", "NodeGroup", 1)
	return parseCSVToComMatrix([]byte(fixed))
}

var _ = g.Describe("Static entries documentation validation", func() {
	type docTestCase struct {
		filename     string
		platformType configv1.PlatformType
		topology     configv1.TopologyMode
		ipv6Enabled  bool
		hasDHCP      bool
	}

	testCases := []docTestCase{
		{
			filename:     "bm.csv",
			platformType: configv1.BareMetalPlatformType,
			topology:     configv1.HighlyAvailableTopologyMode,
			hasDHCP:      true,
		},
		{
			filename:     "none-sno.csv",
			platformType: configv1.NonePlatformType,
			topology:     configv1.SingleReplicaTopologyMode,
			hasDHCP:      true,
		},
		{
			filename:     "aws.csv",
			platformType: configv1.AWSPlatformType,
			topology:     configv1.HighlyAvailableTopologyMode,
		},
		{
			filename:     "aws-sno.csv",
			platformType: configv1.AWSPlatformType,
			topology:     configv1.SingleReplicaTopologyMode,
		},
	}

	for _, tc := range testCases {
		g.Context(tc.filename, func() {
			g.It("contains all required static entries", func() {
				docMatrix, err := readDocCSV(tc.filename)
				o.Expect(err).ToNot(o.HaveOccurred())

				staticEntries, err := GetStaticEntries(tc.platformType, tc.topology, tc.ipv6Enabled, tc.hasDHCP)
				o.Expect(err).ToNot(o.HaveOccurred())

				// Verify each static entry is documented. Contains matches on
				// NodeGroup, Port and Protocol.
				for _, entry := range staticEntries {
					o.Expect(docMatrix.Contains(entry)).To(o.BeTrue(),
						"missing static entry in %s: %s", tc.filename, entry.String())
				}
			})
		})
	}
})
