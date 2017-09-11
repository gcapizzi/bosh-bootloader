package azure_test

import (
	"errors"
	"io/ioutil"
	"path/filepath"
	"strings"

	"github.com/cloudfoundry/bosh-bootloader/cloudconfig/azure"
	"github.com/cloudfoundry/bosh-bootloader/fakes"
	"github.com/cloudfoundry/bosh-bootloader/storage"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pivotal-cf-experimental/gomegamatchers"
)

var _ = Describe("AzureOpsGenerator", func() {
	Describe("Generate", func() {
		var (
			terraformManager *fakes.TerraformManager
			opsGenerator     azure.OpsGenerator

			incomingState   storage.State
			expectedOpsFile []byte
		)

		BeforeEach(func() {
			terraformManager = &fakes.TerraformManager{}

			incomingState = storage.State{
				IAAS: "azure",
			}

			terraformManager.GetOutputsCall.Returns.Outputs = map[string]interface{}{
				"bosh_network_name":           "some-virtual-network-name",
				"bosh_subnet_name":            "some-subnet-name",
				"bosh_default_security_group": "some-security-group",
			}

			var err error
			expectedOpsFile, err = ioutil.ReadFile(filepath.Join("fixtures", "azure-ops.yml"))
			Expect(err).NotTo(HaveOccurred())

			opsGenerator = azure.NewOpsGenerator(terraformManager)
		})

		It("returns an ops file to transform the base cloud config into azure specific cloud config", func() {
			opsYAML, err := opsGenerator.Generate(incomingState)
			Expect(err).NotTo(HaveOccurred())

			Expect(terraformManager.GetOutputsCall.Receives.BBLState).To(Equal(incomingState))

			Expect(opsYAML).To(gomegamatchers.MatchYAML(expectedOpsFile))
		})

		Context("when a cf load balancer exists", func() {
			It("returns an ops file with additional vm extensions", func() {
				terraformManager.GetOutputsCall.Returns.Outputs = map[string]interface{}{
					"bosh_network_name":           "some-virtual-network-name",
					"bosh_subnet_name":            "some-subnet-name",
					"bosh_default_security_group": "some-security-group",
					"web_lb_name":                 "some-web-lb-name",
					"tcp_lb_name":                 "some-tcp-lb-name",
				}

				incomingState.LB.Type = "cf"

				expectedLBOpsFile, err := ioutil.ReadFile(filepath.Join("fixtures", "azure-cf-lb-ops.yml"))
				Expect(err).NotTo(HaveOccurred())
				expectedOps := strings.Join([]string{string(expectedOpsFile), string(expectedLBOpsFile)}, "\n")

				opsYAML, err := opsGenerator.Generate(incomingState)
				Expect(err).NotTo(HaveOccurred())

				Expect(terraformManager.GetOutputsCall.Receives.BBLState).To(Equal(incomingState))

				Expect(opsYAML).To(gomegamatchers.MatchYAML(expectedOps))
			})
		})

		Context("failure cases", func() {
			It("returns an error when terraform output provider fails to retrieve", func() {
				terraformManager.GetOutputsCall.Returns.Error = errors.New("failed to output")
				_, err := opsGenerator.Generate(storage.State{})
				Expect(err).To(MatchError("failed to output"))
			})

			It("returns an error when ops fail to marshal", func() {
				azure.SetMarshal(func(interface{}) ([]byte, error) {
					return []byte{}, errors.New("failed to marshal")
				})
				_, err := opsGenerator.Generate(storage.State{})
				Expect(err).To(MatchError("failed to marshal"))
				azure.ResetMarshal()
			})
		})
	})
})
