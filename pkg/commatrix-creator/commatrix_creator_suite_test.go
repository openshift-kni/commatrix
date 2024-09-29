package commatrixcreator

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestCommatrixCreator(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "CommatrixCreator Suite")
}
