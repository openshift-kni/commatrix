package client

import (
	"testing"

	"k8s.io/apimachinery/pkg/runtime"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var testScheme *runtime.Scheme

var _ = BeforeSuite(func() {
	var err error
	testScheme, err = NewScheme()
	Expect(err).NotTo(HaveOccurred())
})

func TestClient(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Client Suite")
}
