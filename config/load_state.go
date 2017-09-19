package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/cloudfoundry/bosh-bootloader/application"
	"github.com/cloudfoundry/bosh-bootloader/storage"
	flags "github.com/jessevdk/go-flags"
)

type globalFlags struct {
	Help     bool   `short:"h" long:"help"`
	Debug    bool   `short:"d" long:"debug"         env:"BBL_DEBUG"`
	Version  bool   `short:"v" long:"version"`
	StateDir string `short:"s" long:"state-dir"`
	IAAS     string `long:"iaas"                    env:"BBL_IAAS"`

	AWSAccessKeyID     string `long:"aws-access-key-id"       env:"BBL_AWS_ACCESS_KEY_ID"`
	AWSSecretAccessKey string `long:"aws-secret-access-key"   env:"BBL_AWS_SECRET_ACCESS_KEY"`
	AWSRegion          string `long:"aws-region"              env:"BBL_AWS_REGION"`

	AzureClientID       string `long:"azure-client-id"        env:"BBL_AZURE_CLIENT_ID"`
	AzureClientSecret   string `long:"azure-client-secret"    env:"BBL_AZURE_CLIENT_SECRET"`
	AzureLocation       string `long:"azure-location"    env:"BBL_AZURE_LOCATION"`
	AzureSubscriptionID string `long:"azure-subscription-id"  env:"BBL_AZURE_SUBSCRIPTION_ID"`
	AzureTenantID       string `long:"azure-tenant-id"        env:"BBL_AZURE_TENANT_ID"`

	GCPServiceAccountKey string `long:"gcp-service-account-key" env:"BBL_GCP_SERVICE_ACCOUNT_KEY"`
	GCPProjectID         string `long:"gcp-project-id"          env:"BBL_GCP_PROJECT_ID"`
	GCPZone              string `long:"gcp-zone"                env:"BBL_GCP_ZONE"`
	GCPRegion            string `long:"gcp-region"              env:"BBL_GCP_REGION"`
}

func NewConfig(getState func(string) (storage.State, error)) Config {
	return Config{
		getState: getState,
	}
}

type Config struct {
	getState func(string) (storage.State, error)
}

func (c Config) Bootstrap(args []string) (application.Configuration, error) {
	if len(args) == 1 {
		return application.Configuration{
			Command: "help",
		}, nil
	}

	var globalFlags globalFlags
	parser := flags.NewParser(&globalFlags, flags.IgnoreUnknown)

	remainingArgs, err := parser.ParseArgs(args[1:])
	if err != nil {
		return application.Configuration{}, err
	}

	if globalFlags.Version || (len(remainingArgs) > 0 && remainingArgs[0] == "version") {
		return application.Configuration{
			ShowCommandHelp: globalFlags.Help,
			Command:         "version",
		}, nil
	}

	if len(remainingArgs) == 0 || (len(remainingArgs) == 1 && remainingArgs[0] == "help") {
		return application.Configuration{
			ShowCommandHelp: globalFlags.Help,
			Command:         "help",
		}, nil
	}

	if remainingArgs[0] == "help" {
		globalFlags.Help = true
		remainingArgs = remainingArgs[1:]
	}

	if globalFlags.StateDir == "" {
		globalFlags.StateDir, err = os.Getwd()
		if err != nil {
			// not tested
			return application.Configuration{}, err
		}
	}

	state, err := c.getState(globalFlags.StateDir)
	if err != nil {
		return application.Configuration{}, err
	}

	state, err = updateIAASState(globalFlags, state)
	if err != nil {
		return application.Configuration{}, err
	}

	return application.Configuration{
		Global: application.GlobalConfiguration{
			Debug:    globalFlags.Debug,
			StateDir: globalFlags.StateDir,
		},
		State:           state,
		Command:         remainingArgs[0],
		SubcommandFlags: remainingArgs[1:],
		ShowCommandHelp: globalFlags.Help,
	}, nil
}

func updateIAASState(globalFlags globalFlags, state storage.State) (storage.State, error) {
	if globalFlags.IAAS != "" {
		if state.IAAS != "" && globalFlags.IAAS != state.IAAS {
			iaasMismatch := fmt.Sprintf("The iaas type cannot be changed for an existing environment. The current iaas type is %s.", state.IAAS)
			return storage.State{}, errors.New(iaasMismatch)
		}
		state.IAAS = globalFlags.IAAS
	}

	switch state.IAAS {
	case "aws":
		state, err := updateAWSState(globalFlags, state)
		return state, err
	case "gcp":
		state, err := updateGCPState(globalFlags, state)
		return state, err
	case "azure":
		state, err := updateAzureState(globalFlags, state)
		return state, err
	}

	return state, nil
}

func updateAWSState(globalFlags globalFlags, state storage.State) (storage.State, error) {
	if globalFlags.AWSAccessKeyID != "" {
		state.AWS.AccessKeyID = globalFlags.AWSAccessKeyID
	}
	if globalFlags.AWSSecretAccessKey != "" {
		state.AWS.SecretAccessKey = globalFlags.AWSSecretAccessKey
	}
	if globalFlags.AWSRegion != "" {
		if state.AWS.Region != "" && globalFlags.AWSRegion != state.AWS.Region {
			regionMismatch := fmt.Sprintf("The region cannot be changed for an existing environment. The current region is %s.", state.AWS.Region)
			return storage.State{}, errors.New(regionMismatch)
		}
		state.AWS.Region = globalFlags.AWSRegion
	}

	return state, nil
}

func updateGCPState(globalFlags globalFlags, state storage.State) (storage.State, error) {
	if globalFlags.GCPServiceAccountKey != "" {
		serviceAccountKey, err := parseServiceAccountKey(globalFlags.GCPServiceAccountKey)
		if err != nil {
			return storage.State{}, err
		}
		state.GCP.ServiceAccountKey = serviceAccountKey
	}
	if globalFlags.GCPProjectID != "" {
		state.GCP.ProjectID = globalFlags.GCPProjectID
	}
	if globalFlags.GCPZone != "" {
		if state.GCP.Zone != "" && globalFlags.GCPZone != state.GCP.Zone {
			zoneMismatch := fmt.Sprintf("The zone cannot be changed for an existing environment. The current zone is %s.", state.GCP.Zone)
			return storage.State{}, errors.New(zoneMismatch)
		}
		state.GCP.Zone = globalFlags.GCPZone
	}
	if globalFlags.GCPRegion != "" {
		if state.GCP.Region != "" && globalFlags.GCPRegion != state.GCP.Region {
			regionMismatch := fmt.Sprintf("The region cannot be changed for an existing environment. The current region is %s.", state.GCP.Region)
			return storage.State{}, errors.New(regionMismatch)
		}
		state.GCP.Region = globalFlags.GCPRegion
	}

	return state, nil
}

func updateAzureState(globalFlags globalFlags, state storage.State) (storage.State, error) {
	if globalFlags.AzureClientID != "" {
		state.Azure.ClientID = globalFlags.AzureClientID
	}
	if globalFlags.AzureClientSecret != "" {
		state.Azure.ClientSecret = globalFlags.AzureClientSecret
	}
	if globalFlags.AzureLocation != "" {
		state.Azure.Location = globalFlags.AzureLocation
	}
	if globalFlags.AzureSubscriptionID != "" {
		state.Azure.SubscriptionID = globalFlags.AzureSubscriptionID
	}
	if globalFlags.AzureTenantID != "" {
		state.Azure.TenantID = globalFlags.AzureTenantID
	}

	return state, nil
}

func ValidateIAAS(state storage.State, command string) error {
	if state.IAAS == "" || (state.IAAS != "gcp" && state.IAAS != "aws" && state.IAAS != "azure") {
		return errors.New("--iaas [gcp, aws, azure] must be provided or BBL_IAAS must be set")
	}
	if state.IAAS == "aws" {
		err := validateAWS(state.AWS)
		if err != nil {
			return err
		}
	}
	if state.IAAS == "gcp" {
		err := validateGCP(state.GCP)
		if err != nil {
			return err
		}
	}
	if state.IAAS == "azure" {
		err := validateAzure(state.Azure)
		if err != nil {
			return err
		}
	}
	return nil
}

func NeedsIAASConfig(command string) bool {
	_, ok := map[string]struct{}{
		"up":         struct{}{},
		"down":       struct{}{},
		"destroy":    struct{}{},
		"create-lbs": struct{}{},
		"delete-lbs": struct{}{},
		"update-lbs": struct{}{},
		"rotate":     struct{}{},
	}[command]
	return ok
}

func validateAWS(aws storage.AWS) error {
	if aws.AccessKeyID == "" {
		return errors.New("AWS access key ID must be provided")
	}
	if aws.SecretAccessKey == "" {
		return errors.New("AWS secret access key must be provided")
	}
	if aws.Region == "" {
		return errors.New("AWS region must be provided")
	}
	return nil
}

func validateGCP(gcp storage.GCP) error {
	if gcp.ServiceAccountKey == "" {
		return errors.New("GCP service account key must be provided")
	}
	if gcp.ProjectID == "" {
		return errors.New("GCP project ID must be provided")
	}
	if gcp.Zone == "" {
		return errors.New("GCP zone must be provided")
	}
	if gcp.Region == "" {
		return errors.New("GCP region must be provided")
	}
	return nil
}

func validateAzure(azure storage.Azure) error {
	if azure.ClientID == "" {
		return errors.New("Azure client id must be provided")
	}
	if azure.ClientSecret == "" {
		return errors.New("Azure client secret must be provided")
	}
	if azure.Location == "" {
		return errors.New("Azure location must be provided")
	}
	if azure.SubscriptionID == "" {
		return errors.New("Azure subscription id must be provided")
	}
	if azure.TenantID == "" {
		return errors.New("Azure tenant id must be provided")
	}
	return nil
}

func parseServiceAccountKey(serviceAccountKey string) (string, error) {
	var key string

	if _, err := os.Stat(serviceAccountKey); err != nil {
		key = serviceAccountKey
	} else {
		rawServiceAccountKey, err := ioutil.ReadFile(serviceAccountKey)
		if err != nil {
			return "", fmt.Errorf("error reading service account key from file: %v", err)
		}

		key = string(rawServiceAccountKey)
	}

	var tmp interface{}
	err := json.Unmarshal([]byte(key), &tmp)
	if err != nil {
		return "", fmt.Errorf("error unmarshalling service account key (must be valid json): %v", err)
	}

	return key, err
}
