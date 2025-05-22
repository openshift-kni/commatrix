package e2e

import (
	"log"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/openshift-kni/commatrix/pkg/client"
	"github.com/openshift-kni/commatrix/pkg/utils"
	configv1 "github.com/openshift/api/config/v1"
	corev1 "k8s.io/api/core/v1"
)

var (
	cs           *client.ClientSet
	isSNO        bool
	isBM         bool
	utilsHelpers utils.UtilsInterface
	nodeList     *corev1.NodeList
	artifactsDir string
)

const testNS = "openshift-commatrix-test"

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

	platformType, err := utilsHelpers.GetPlatformType()
	Expect(err).NotTo(HaveOccurred())

	isBM = false
	// Assuming Telco partners use 'None' platform type just on Bare Metal.
	// Mark as Bare Metal if the platform type is either 'BareMetal' (multi-node BM) or 'None' (SNO BM).
	if platformType == configv1.BareMetalPlatformType || platformType == configv1.NonePlatformType {
		isBM = true
	}
})

func TestE2e(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "E2e Suite")
}
