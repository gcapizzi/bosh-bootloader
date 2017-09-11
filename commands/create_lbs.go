package commands

import (
	"errors"
	"fmt"

	"github.com/cloudfoundry/bosh-bootloader/flags"
	"github.com/cloudfoundry/bosh-bootloader/storage"
)

type CreateLBs struct {
	awsCreateLBs         awsCreateLBs
<<<<<<< Updated upstream
	boshManager          boshManager
	certificateValidator certificateValidator
=======
	azureCreateLBs       azureCreateLBs
>>>>>>> Stashed changes
	gcpCreateLBs         gcpCreateLBs
	logger               logger
	stateValidator       stateValidator
}

type lbConfig struct {
	lbType       string
	certPath     string
	keyPath      string
	chainPath    string
	domain       string
	skipIfExists bool
}

type awsCreateLBs interface {
	Execute(AWSCreateLBsConfig, storage.State) error
}

type azureCreateLBs interface {
	Execute(AzureCreateLBsConfig, storage.State) error
}

type gcpCreateLBs interface {
	Execute(GCPCreateLBsConfig, storage.State) error
}

type certificateValidator interface {
	Validate(command, certPath, keyPath, chainPath string) error
}

<<<<<<< Updated upstream
func NewCreateLBs(awsCreateLBs awsCreateLBs, gcpCreateLBs gcpCreateLBs, logger logger, stateValidator stateValidator, certificateValidator certificateValidator, boshManager boshManager) CreateLBs {
=======
func NewCreateLBs(awsCreateLBs awsCreateLBs, azureCreateLBs azureCreateLBs, gcpCreateLBs gcpCreateLBs, stateValidator stateValidator, certificateValidator certificateValidator, boshManager boshManager) CreateLBs {
>>>>>>> Stashed changes
	return CreateLBs{
		boshManager:          boshManager,
		logger:               logger,
		awsCreateLBs:         awsCreateLBs,
		azureCreateLBs:       azureCreateLBs,
		gcpCreateLBs:         gcpCreateLBs,
		stateValidator:       stateValidator,
		certificateValidator: certificateValidator,
	}
}

func (c CreateLBs) CheckFastFails(subcommandFlags []string, state storage.State) error {
	config, err := parseFlags(subcommandFlags)
	if err != nil {
		return err
	}

	if err := c.stateValidator.Validate(); err != nil {
		return fmt.Errorf("Validate state: %s", err)
	}

	if config.skipIfExists && config.lbType == state.LB.Type {
		c.logger.Step(fmt.Sprintf("lb type %q exists, skipping...", state.LB.Type))
		return nil
	}

	if !lbExists(config.lbType) {
		return errors.New("--type is required")
	}

	if !(state.IAAS == "gcp" && config.lbType == "concourse") {
		err = c.certificateValidator.Validate("create-lbs", config.certPath, config.keyPath, config.chainPath)
		if err != nil {
			return fmt.Errorf("Validate certificate: %s", err)
		}
	}

	if config.lbType == "concourse" && config.domain != "" {
		return errors.New("--domain is not implemented for concourse load balancers. Remove the --domain flag and try again.")
	}

	if !state.NoDirector {
		err := fastFailBOSHVersion(c.boshManager)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c CreateLBs) Execute(args []string, state storage.State) error {
	config, err := parseFlags(args)
	if err != nil {
		return err
	}

	switch state.IAAS {
	case "aws":
		if err := c.awsCreateLBs.Execute(AWSCreateLBsConfig{
			LBType:       config.lbType,
			CertPath:     config.certPath,
			KeyPath:      config.keyPath,
			ChainPath:    config.chainPath,
			Domain:       config.domain,
			SkipIfExists: config.skipIfExists,
		}, state); err != nil {
			return err
		}
	case "azure":
		if err := c.azureCreateLBs.Execute(AzureCreateLBsConfig{
			LBType:       config.lbType,
			CertPath:     config.certPath,
			KeyPath:      config.keyPath,
			ChainPath:    config.chainPath,
			Domain:       config.domain,
			SkipIfExists: config.skipIfExists,
		}, state); err != nil {
			return err
		}
	case "gcp":
		if err := c.gcpCreateLBs.Execute(GCPCreateLBsConfig{
			LBType:       config.lbType,
			CertPath:     config.certPath,
			KeyPath:      config.keyPath,
			Domain:       config.domain,
			SkipIfExists: config.skipIfExists,
		}, state); err != nil {
			return err
		}
	}

	return nil
}

func parseFlags(subcommandFlags []string) (lbConfig, error) {
	lbFlags := flags.New("create-lbs")

	config := lbConfig{}
	lbFlags.String(&config.lbType, "type", "")
	lbFlags.String(&config.certPath, "cert", "")
	lbFlags.String(&config.keyPath, "key", "")
	lbFlags.String(&config.chainPath, "chain", "")
	lbFlags.String(&config.domain, "domain", "")
	lbFlags.Bool(&config.skipIfExists, "skip-if-exists", "", false)

	if err := lbFlags.Parse(subcommandFlags); err != nil {
		return config, err
	}

	return config, nil
}
