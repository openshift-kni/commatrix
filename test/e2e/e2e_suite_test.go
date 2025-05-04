package e2e

import (
	"log"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/openshift-kni/commatrix/cmd/generate"
	"github.com/openshift-kni/commatrix/pkg/client"
	"github.com/openshift-kni/commatrix/pkg/utils"
	corev1 "k8s.io/api/core/v1"
)

var (
	cs           *client.ClientSet
	isSNO        bool
	isBM         bool
	utilsHelpers utils.UtilsInterface
	nodeList     *corev1.NodeList
	artifactsDir string
	platformType string
)

const (
	testNS = "openshift-commatrix-test"
)

var _ = BeforeSuite(func() {
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		Fail("KUBECONFIG not set")
	}

	By("Creating output folder")
	artifactsDir = os.Getenv("ARTIFACT_DIR")
	if artifactsDir == "" {
		log.Println("env var ARTIFACT_DIR is not set, using default value")
	}

	artifactsDir = filepath.Join(artifactsDir, "commatrix-e2e")
	err := os.MkdirAll(artifactsDir, 0755)
	Expect(err).NotTo(HaveOccurred())

	By("Creating New Commatrix Client")
	cs, err = client.New()
	Expect(err).NotTo(HaveOccurred())

	utilsHelpers = utils.New(cs)
	isSNO, err = utilsHelpers.IsSNOCluster()
	Expect(err).NotTo(HaveOccurred())

	isBM = false
	platformType, _ = os.LookupEnv("PLATFORM_TYPE")
	if platformType == "" || (platformType != string(generate.PlatformBaremetal) && platformType != string(generate.PlatformAWS)) { // if not set or value is wrong will fail
		log.Println("Environment variable PLATFORM_TYPE is not set or has an invalid value; please set it to either 'baremetal' or 'aws'")
		Fail("Invalid or unset PLATFORM_TYPE environment variable")
	}

	if platformType == string(generate.PlatformBaremetal) { // if it set
		isBM = true
	}
})

func TestE2e(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "E2e Suite")
}
