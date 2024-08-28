package listeningsockets_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestListeningSockets(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "ListeningSockets Suite")
}
