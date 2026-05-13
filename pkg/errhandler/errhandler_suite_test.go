package errhandler

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestErrhandler(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Errhandler Suite")
}
