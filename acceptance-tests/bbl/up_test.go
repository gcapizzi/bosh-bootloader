package acceptance_test

import (
	"fmt"
	"net/url"
	"time"

	"golang.org/x/crypto/ssh"

	acceptance "github.com/cloudfoundry/bosh-bootloader/acceptance-tests"
	"github.com/cloudfoundry/bosh-bootloader/acceptance-tests/actors"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

var _ = Describe("up", func() {
	var (
		bbl             actors.BBL
		boshcli         actors.BOSHCLI
		configuration   acceptance.Config
		directorAddress string
		caCertPath      string
	)

	BeforeEach(func() {
		var err error
		configuration, err = acceptance.LoadConfig()
		Expect(err).NotTo(HaveOccurred())

		bbl = actors.NewBBL(configuration.StateFileDir, pathToBBL, configuration, "up-env")
		boshcli = actors.NewBOSHCLI()
	})

	AfterEach(func() {
		session := bbl.Down()
		Eventually(session, 10*time.Minute).Should(gexec.Exit())
	})

	FIt("bbl's up a new bosh director", func() {
		acceptance.SkipUnless("bbl-up")
		session := bbl.Up("--name", bbl.PredefinedEnvID())
		Eventually(session, 40*time.Minute).Should(gexec.Exit(0))

		By("checking if the bosh director exists", func() {
			directorAddress = bbl.DirectorAddress()
			caCertPath = bbl.SaveDirectorCA()

			exists, err := boshcli.DirectorExists(directorAddress, caCertPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(exists).To(BeTrue())
		})

		By("checking that the cloud config exists", func() {
			directorUsername := bbl.DirectorUsername()
			directorPassword := bbl.DirectorPassword()

			cloudConfig, err := boshcli.CloudConfig(directorAddress, caCertPath, directorUsername, directorPassword)
			Expect(err).NotTo(HaveOccurred())
			Expect(cloudConfig).NotTo(BeEmpty())
		})

		By("checking if ssh'ing works", func() {
			privateKey, err := ssh.ParsePrivateKey([]byte(bbl.SSHKey()))
			Expect(err).NotTo(HaveOccurred())

			directorAddressURL, err := url.Parse(bbl.DirectorAddress())
			Expect(err).NotTo(HaveOccurred())

			address := fmt.Sprintf("%s:22", directorAddressURL.Hostname())
			_, err = ssh.Dial("tcp", address, &ssh.ClientConfig{
				User: "jumpbox",
				Auth: []ssh.AuthMethod{
					ssh.PublicKeys(privateKey),
				},
				HostKeyCallback: ssh.InsecureIgnoreHostKey(),
			})
			Expect(err).NotTo(HaveOccurred())
		})

		By("checking if bbl print-env prints the bosh environment variables", func() {
			stdout := bbl.PrintEnv()

			Expect(stdout).To(ContainSubstring("export BOSH_ENVIRONMENT="))
			Expect(stdout).To(ContainSubstring("export BOSH_CLIENT="))
			Expect(stdout).To(ContainSubstring("export BOSH_CLIENT_SECRET="))
			Expect(stdout).To(ContainSubstring("export BOSH_CA_CERT="))
		})

		By("checking bbl up with director is idempotent", func() {
			session := bbl.Up()
			Eventually(session, 40*time.Minute).Should(gexec.Exit(0))
		})

		By("destroying the director", func() {
			session := bbl.Down()
			Eventually(session, 10*time.Minute).Should(gexec.Exit(0))
		})
	})
})
