package e2e

import (
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/decisiveai/mdai-operator/test/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	// Optional Environment Variables:
	// - CERT_MANAGER_INSTALL_SKIP=true: Skips CertManager installation during test setup.
	// These variables are useful if CertManager is already installed, avoiding
	// re-installation and conflicts.
	skipCertManagerInstall = os.Getenv("CERT_MANAGER_INSTALL_SKIP") == "true"
	// isCertManagerAlreadyInstalled will be set true when CertManager CRDs be found on the cluster
	isCertManagerAlreadyInstalled = false

	// projectImage is the name of the image which will be build and loaded
	// with the code source changes to be tested.
	projectImage = "mdai-operator:v0.0.1"
)

// TestE2E runs the end-to-end (e2e) test suite for the project. These tests execute in an isolated,
// temporary environment to validate project changes with the purposed to be used in CI jobs.
// The default setup requires Kind, builds/loads the Manager Docker image locally, and installs
// CertManager and Prometheus.
func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	GinkgoWriter.Printf("Starting mdai-operator integration test suite\n")
	RunSpecs(t, "e2e suite")
}

var _ = BeforeSuite(func() {
	By("Ensure that Prometheus is enabled")
	_ = utils.UncommentCode("config/default/kustomization.yaml", "#- ../prometheus", "#")

	By("building the manager(Operator) image")
	cmd := exec.Command("make", "docker-build", "IMG="+projectImage)
	_, err := utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to build the manager(Operator) image")

	By("loading the manager(Operator) image on Kind")
	err = utils.LoadImageToKindClusterWithName(projectImage)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to load the manager(Operator) image into Kind")

	// The tests-e2e are intended to run on a temporary cluster that is created and destroyed for testing.
	// To prevent errors when tests run in environments with CertManager already installed,
	// we check for their presence before execution.
	// Setup CertManager before the suite if not skipped and if not already installed
	if !skipCertManagerInstall {
		By("checking if cert manager is installed already")
		isCertManagerAlreadyInstalled = utils.IsCertManagerCRDsInstalled()
		if !isCertManagerAlreadyInstalled {
			GinkgoWriter.Printf("Installing CertManager...\n")
			Expect(utils.InstallCertManager()).To(Succeed(), "Failed to install CertManager")
		} else {
			GinkgoWriter.Printf("WARNING: CertManager is already installed. Skipping installation...\n")
		}
	}

	GinkgoWriter.Printf("Installing Valkey...\n")
	Expect(utils.InstallValkey()).To(Succeed(), "Failed to install Valkey")

	GinkgoWriter.Printf("Waiting for certificates...\n")
	time.Sleep(15 * time.Second)

	// Install Prometheus operator CRD
	Expect(utils.InstallPrometheusOperatorCRD()).To(Succeed(), "Failed to install Prometheus Operator CRD")

	GinkgoWriter.Printf("Installing OTEL...\n")
	Expect(utils.InstallOtelOperator()).To(Succeed(), "Failed to install OpenTelemetry Operator")
	Expect(utils.WaitForValkeyReady()).To(Succeed(), "Failed to wait for Valkey to be ready")
})

var _ = AfterSuite(func() {
	// Teardown CertManager after the suite if not skipped and if they were not already installed
	if !skipCertManagerInstall && !isCertManagerAlreadyInstalled {
		GinkgoWriter.Printf("Uninstalling CertManager...\n")
		utils.UninstallCertManager()
	}

	utils.UninstallOtelOperator()
	utils.UninstallValkey()
})
