package e2e

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/openshift-kni/commatrix/pkg/client"
	"github.com/openshift-kni/commatrix/pkg/types"
	"github.com/openshift-kni/commatrix/pkg/utils"
	configv1 "github.com/openshift/api/config/v1"
	corev1 "k8s.io/api/core/v1"
)

var (
	cs                   *client.ClientSet
	platformType         configv1.PlatformType
	utilsHelpers         utils.UtilsInterface
	nodeList             *corev1.NodeList
	artifactsDir         string
	controlPlaneTopology configv1.TopologyMode
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
	platformType, err = utilsHelpers.GetPlatformType()
	Expect(err).NotTo(HaveOccurred())

	// if cluster's type is not supported by the commatrix app, skip tests
	if !slices.Contains(types.SupportedPlatforms, platformType) {
		Skip(fmt.Sprintf("unsupported platform type: %s. Supported platform types are: %v", platformType, types.SupportedPlatforms))
	}

	// also validate control plane topology (HA, SNO, HyperShift External)
	controlPlaneTopology, err = utilsHelpers.GetControlPlaneTopology()
	Expect(err).NotTo(HaveOccurred())

	if !types.IsSupportedTopology(controlPlaneTopology) {
		Skip(fmt.Sprintf("unsupported control plane topology: %s. Supported topologies are: %v",
			controlPlaneTopology, types.SupportedTopologiesList()))
	}
})

func TestE2e(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "E2e Suite")
}
