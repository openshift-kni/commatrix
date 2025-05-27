package e2e

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/openshift-kni/commatrix/pkg/client"
	commatrixcreator "github.com/openshift-kni/commatrix/pkg/commatrix-creator"
	"github.com/openshift-kni/commatrix/pkg/endpointslices"
	"github.com/openshift-kni/commatrix/pkg/types"
	"github.com/openshift-kni/commatrix/pkg/utils"
	configv1 "github.com/openshift/api/config/v1"
	corev1 "k8s.io/api/core/v1"
)

var (
	cs           *client.ClientSet
	commatrix    *types.ComMatrix
	isSNO        bool
	isBM         bool
	deployment   types.Deployment
	utilsHelpers utils.UtilsInterface
	epExporter   *endpointslices.EndpointSlicesExporter
	nodeList     *corev1.NodeList
	artifactsDir string
	platformType configv1.PlatformType
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

	By("Creating the clients for the Generating step")
	cs, err = client.New()
	Expect(err).NotTo(HaveOccurred())

	utilsHelpers = utils.New(cs)

	deployment = types.Standard
	isSNO, err = utilsHelpers.IsSNOCluster()
	Expect(err).NotTo(HaveOccurred())

	if isSNO {
		deployment = types.SNO
	}

	platformType, err = utilsHelpers.GetPlatformType()
	Expect(err).NotTo(HaveOccurred())

	isBM = false
	// Assuming Telco partners use 'None' platform type just on Bare Metal.
	// Mark as Bare Metal if the platform type is either 'BareMetal' (multi-node BM) or 'None' (SNO BM).
	if platformType == configv1.BareMetalPlatformType || platformType == configv1.NonePlatformType {
		isBM = true
	}

	epExporter, err = endpointslices.New(cs)
	Expect(err).ToNot(HaveOccurred())

	By("Generating comMatrix")
	commMatrixCreator, err := commatrixcreator.New(epExporter, "", "", platformType, deployment)
	Expect(err).NotTo(HaveOccurred())

	commatrix, err = commMatrixCreator.CreateEndpointMatrix()
	Expect(err).NotTo(HaveOccurred())

	err = commatrix.WriteMatrixToFileByType(utilsHelpers, "communication-matrix", types.FormatCSV, deployment, artifactsDir)
	Expect(err).ToNot(HaveOccurred())

	By("Creating test namespace")
	err = utilsHelpers.CreateNamespace(testNS)
	Expect(err).ToNot(HaveOccurred())

	nodeList = &corev1.NodeList{}
	err = cs.List(context.TODO(), nodeList)
	Expect(err).ToNot(HaveOccurred())
})

var _ = AfterSuite(func() {
	By("Deleting Namespace")
	err := utilsHelpers.DeleteNamespace(testNS)
	Expect(err).ToNot(HaveOccurred())
})

func TestE2e(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "E2e Suite")
}
