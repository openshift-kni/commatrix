package matrixdiff

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestMatrixDiff(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "MatrixDiff Suite")
}
