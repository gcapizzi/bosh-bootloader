package gcp

import (
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/cloudfoundry/bosh-bootloader/storage"
)

var writeFile func(file string, data []byte, perm os.FileMode) error = ioutil.WriteFile

type InputGenerator struct {
	stateDir string
}

func NewInputGenerator(stateDir string) InputGenerator {
	return InputGenerator{
		stateDir: stateDir,
	}
}

func (i InputGenerator) Generate(state storage.State) (map[string]string, error) {
	credentialsPath := filepath.Join(i.stateDir, "credentials.json")
	err := writeFile(credentialsPath, []byte(state.GCP.ServiceAccountKey), os.ModePerm)
	if err != nil {
		return map[string]string{}, err
	}

	input := map[string]string{
		"env_id":        state.EnvID,
		"project_id":    state.GCP.ProjectID,
		"region":        state.GCP.Region,
		"zone":          state.GCP.Zone,
		"credentials":   credentialsPath,
		"system_domain": state.LB.Domain,
	}

	if state.LB.Cert != "" && state.LB.Key != "" {
		certPath := filepath.Join(i.stateDir, "cert")
		err = writeFile(certPath, []byte(state.LB.Cert), os.ModePerm)
		if err != nil {
			return map[string]string{}, err
		}
		input["ssl_certificate"] = certPath

		keyPath := filepath.Join(i.stateDir, "key")
		err = writeFile(keyPath, []byte(state.LB.Key), os.ModePerm)
		if err != nil {
			return map[string]string{}, err
		}
		input["ssl_certificate_private_key"] = keyPath
	}

	return input, nil
}
