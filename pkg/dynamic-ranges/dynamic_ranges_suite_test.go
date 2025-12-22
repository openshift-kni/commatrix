package dynamicranges

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestDynamicRanges(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "DynamicRanges Suite")
}
