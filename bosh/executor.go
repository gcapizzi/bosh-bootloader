package bosh

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"

	"github.com/cloudfoundry/bosh-bootloader/helpers"
)

const gcpBoshDirectorEphemeralIPOps = `
- type: replace
  path: /networks/name=default/subnets/0/cloud_properties/ephemeral_external_ip?
  value: true
`

const awsBoshDirectorEphemeralIPOps = `
- type: replace
  path: /resource_pools/name=vms/cloud_properties/auto_assign_public_ip?
  value: true
`

const awsEncryptDiskOps = `---
- type: replace
  path: /disk_pools/name=disks/cloud_properties?
  value:
    type: gp2
    encrypted: true
    kms_key_arn: ((kms_key_arn))
`

const azureSSHStaticIP = `
- type: replace
  path: /cloud_provider/ssh_tunnel/host
  value: ((external_ip))
`

type Executor struct {
	command       command
	readFile      func(string) ([]byte, error)
	unmarshalJSON func([]byte, interface{}) error
	marshalJSON   func(interface{}) ([]byte, error)
	writeFile     func(string, []byte, os.FileMode) error
	boshDir       string
}

type InterpolateInput struct {
	IAAS                   string
	DirectorDeploymentVars string
	JumpboxDeploymentVars  string
	BOSHState              map[string]interface{}
	Variables              string
	OpsFile                string
}

type InterpolateOutput struct {
	Variables string
	Manifest  string
}

type JumpboxInterpolateOutput struct {
	Variables string
	Manifest  string
}

type CreateEnvInput struct {
	Manifest  string
	Variables string
	State     map[string]interface{}
	Dir       string
}

type CreateEnvOutput struct {
	State map[string]interface{}
}

type DeleteEnvInput struct {
	Manifest  string
	Variables string
	State     map[string]interface{}
	Dir       string
}

type command interface {
	Run(stdout io.Writer, workingDirectory string, args []string) error
}

const VERSION_DEV_BUILD = "[DEV BUILD]"

func NewExecutor(cmd command, readFile func(string) ([]byte, error),
	unmarshalJSON func([]byte, interface{}) error,
	marshalJSON func(interface{}) ([]byte, error), writeFile func(string, []byte, os.FileMode) error, boshDir string) Executor {
	return Executor{
		command:       cmd,
		readFile:      readFile,
		unmarshalJSON: unmarshalJSON,
		marshalJSON:   marshalJSON,
		writeFile:     writeFile,
		boshDir:       boshDir,
	}
}

func (e Executor) JumpboxInterpolate(interpolateInput InterpolateInput) (JumpboxInterpolateOutput, error) {
	jumpboxDir := filepath.Join(e.boshDir, "jumpbox")
	var jumpboxSetupFiles = map[string][]byte{
		"jumpbox-deployment-vars.yml": []byte(interpolateInput.JumpboxDeploymentVars),
		"jumpbox.yml":                 MustAsset("vendor/github.com/cppforlife/jumpbox-deployment/jumpbox.yml"),
		"cpi.yml":                     MustAsset(fmt.Sprintf("vendor/github.com/cppforlife/jumpbox-deployment/%s/cpi.yml", interpolateInput.IAAS)),
	}

	if interpolateInput.Variables != "" {
		jumpboxSetupFiles["variables.yml"] = []byte(interpolateInput.Variables)
	}

	for path, contents := range jumpboxSetupFiles {
		err := e.writeFile(filepath.Join(jumpboxDir, path), contents, os.ModePerm)
		if err != nil {
			//not tested
			return JumpboxInterpolateOutput{}, fmt.Errorf("write file: %s", err)
		}
	}

	args := []string{
		"interpolate", filepath.Join(jumpboxDir, "jumpbox.yml"),
		"--var-errs",
		"--vars-store", filepath.Join(jumpboxDir, "variables.yml"),
		"--vars-file", filepath.Join(jumpboxDir, "jumpbox-deployment-vars.yml"),
		"-o", filepath.Join(jumpboxDir, "cpi.yml"),
	}

	buffer := bytes.NewBuffer([]byte{})
	err := e.command.Run(buffer, jumpboxDir, args)
	if err != nil {
		return JumpboxInterpolateOutput{}, fmt.Errorf("bosh interpolate: %s: %s", err, buffer)
	}

	varsStore, err := e.readFile(filepath.Join(jumpboxDir, "variables.yml"))
	if err != nil {
		return JumpboxInterpolateOutput{}, fmt.Errorf("read file: %s", err)
	}

	return JumpboxInterpolateOutput{
		Variables: string(varsStore),
		Manifest:  buffer.String(),
	}, nil
}

func (e Executor) DirectorInterpolate(interpolateInput InterpolateInput) (InterpolateOutput, error) {
	directorDir := filepath.Join(e.boshDir, "director")
	var directorSetupFiles = map[string][]byte{
		"deployment-vars.yml":                    []byte(interpolateInput.DirectorDeploymentVars),
		"user-ops-file.yml":                      []byte(interpolateInput.OpsFile),
		"bosh.yml":                               MustAsset("vendor/github.com/cloudfoundry/bosh-deployment/bosh.yml"),
		"cpi.yml":                                MustAsset(fmt.Sprintf("vendor/github.com/cloudfoundry/bosh-deployment/%s/cpi.yml", interpolateInput.IAAS)),
		"iam-instance-profile.yml":               MustAsset("vendor/github.com/cloudfoundry/bosh-deployment/aws/iam-instance-profile.yml"),
		"gcp-bosh-director-ephemeral-ip-ops.yml": []byte(gcpBoshDirectorEphemeralIPOps),
		"aws-bosh-director-ephemeral-ip-ops.yml": []byte(awsBoshDirectorEphemeralIPOps),
		"aws-bosh-director-encrypt-disk-ops.yml": []byte(awsEncryptDiskOps),
		"azure-ssh-static-ip.yml":                []byte(azureSSHStaticIP),
		"jumpbox-user.yml":                       MustAsset("vendor/github.com/cloudfoundry/bosh-deployment/jumpbox-user.yml"),
		"gcp-external-ip-not-recommended.yml":    MustAsset("vendor/github.com/cloudfoundry/bosh-deployment/external-ip-not-recommended.yml"),
		"azure-external-ip-not-recommended.yml":  MustAsset("vendor/github.com/cloudfoundry/bosh-deployment/external-ip-not-recommended.yml"),
		"aws-external-ip-not-recommended.yml":    MustAsset("vendor/github.com/cloudfoundry/bosh-deployment/external-ip-with-registry-not-recommended.yml"),
		"uaa.yml":     MustAsset("vendor/github.com/cloudfoundry/bosh-deployment/uaa.yml"),
		"credhub.yml": MustAsset("vendor/github.com/cloudfoundry/bosh-deployment/credhub.yml"),
	}

	if interpolateInput.Variables != "" {
		directorSetupFiles["variables.yml"] = []byte(interpolateInput.Variables)
	}

	for path, contents := range directorSetupFiles {
		err := e.writeFile(filepath.Join(directorDir, path), contents, os.ModePerm)
		if err != nil {
			//not tested
			return InterpolateOutput{}, err
		}
	}

	var args = []string{
		"interpolate", filepath.Join(directorDir, "bosh.yml"),
		"--var-errs",
		// "--var-errs-unused",
		"--vars-store", filepath.Join(directorDir, "variables.yml"),
		"--vars-file", filepath.Join(directorDir, "deployment-vars.yml"),
		"-o", filepath.Join(directorDir, "cpi.yml"),
	}

	switch interpolateInput.IAAS {
	case "gcp":
		args = append(args,
			"-o", filepath.Join(directorDir, "jumpbox-user.yml"),
			"-o", filepath.Join(directorDir, "uaa.yml"),
			"-o", filepath.Join(directorDir, "credhub.yml"),
			"-o", filepath.Join(directorDir, "gcp-bosh-director-ephemeral-ip-ops.yml"),
		)
	case "aws":
		args = append(args,
			"-o", filepath.Join(directorDir, "jumpbox-user.yml"),
			"-o", filepath.Join(directorDir, "uaa.yml"),
			"-o", filepath.Join(directorDir, "credhub.yml"),
			"-o", filepath.Join(directorDir, "aws-bosh-director-ephemeral-ip-ops.yml"),
			"-o", filepath.Join(directorDir, "iam-instance-profile.yml"),
			"-o", filepath.Join(directorDir, "aws-bosh-director-encrypt-disk-ops.yml"),
		)
	case "azure":
		// NOTE: azure does not yet support jumpbox
		args = append(args,
			"-o", filepath.Join(directorDir, "jumpbox-user.yml"),
			"-o", filepath.Join(directorDir, "azure-external-ip-not-recommended.yml"),
			"-o", filepath.Join(directorDir, "azure-ssh-static-ip.yml"),
		)
	}

	buffer := bytes.NewBuffer([]byte{})
	err := e.command.Run(buffer, directorDir, args)
	if err != nil {
		return InterpolateOutput{}, err
	}

	if interpolateInput.OpsFile != "" {
		err = e.writeFile(filepath.Join(directorDir, "bosh.yml"), buffer.Bytes(), os.ModePerm)
		if err != nil {
			//not tested
			return InterpolateOutput{}, err
		}

		args = []string{
			"interpolate", filepath.Join(directorDir, "bosh.yml"),
			"--var-errs",
			"--vars-store", filepath.Join(directorDir, "variables.yml"),
			"--vars-file", filepath.Join(directorDir, "deployment-vars.yml"),
			"-o", filepath.Join(directorDir, "user-ops-file.yml"),
		}

		buffer = bytes.NewBuffer([]byte{})
		err = e.command.Run(buffer, directorDir, args)
		if err != nil {
			return InterpolateOutput{}, err
		}
	}

	varsStore, err := e.readFile(filepath.Join(directorDir, "variables.yml"))
	if err != nil {
		return InterpolateOutput{}, err
	}

	return InterpolateOutput{
		Variables: string(varsStore),
		Manifest:  buffer.String(),
	}, nil
}

func (e Executor) CreateEnv(createEnvInput CreateEnvInput) (CreateEnvOutput, error) {
	workingDir := filepath.Join(e.boshDir, createEnvInput.Dir)

	err := e.writePreviousFiles(createEnvInput.State, createEnvInput.Variables, createEnvInput.Manifest, workingDir)
	if err != nil {
		return CreateEnvOutput{}, err
	}

	statePath := fmt.Sprintf("%s/state.json", workingDir)
	variablesPath := fmt.Sprintf("%s/variables.yml", workingDir)
	manifestPath := filepath.Join(workingDir, "manifest.yml")

	args := []string{
		"create-env", manifestPath,
		"--vars-store", variablesPath,
		"--state", statePath,
	}

	err = e.command.Run(os.Stdout, workingDir, args)
	if err != nil {
		state, readErr := e.readBOSHState(statePath)
		if readErr != nil {
			errorList := helpers.Errors{}
			errorList.Add(err)
			errorList.Add(readErr)
			return CreateEnvOutput{}, errorList
		}

		return CreateEnvOutput{}, NewCreateEnvError(state, err)
	}

	state, err := e.readBOSHState(statePath)
	if err != nil {
		return CreateEnvOutput{}, err
	}

	return CreateEnvOutput{
		State: state,
	}, nil
}

func (e Executor) readBOSHState(statePath string) (map[string]interface{}, error) {
	stateContents, err := e.readFile(statePath)
	if err != nil {
		return map[string]interface{}{}, err
	}

	var state map[string]interface{}
	err = e.unmarshalJSON(stateContents, &state)
	if err != nil {
		return map[string]interface{}{}, err
	}

	return state, nil
}

func (e Executor) DeleteEnv(deleteEnvInput DeleteEnvInput) error {
	workingDir := filepath.Join(e.boshDir, deleteEnvInput.Dir)
	err := e.writePreviousFiles(deleteEnvInput.State, deleteEnvInput.Variables, deleteEnvInput.Manifest, workingDir)
	if err != nil {
		return err
	}

	statePath := fmt.Sprintf("%s/state.json", workingDir)
	variablesPath := fmt.Sprintf("%s/variables.yml", workingDir)
	boshManifestPath := filepath.Join(workingDir, "manifest.yml")

	args := []string{
		"delete-env", boshManifestPath,
		"--vars-store", variablesPath,
		"--state", statePath,
	}

	err = e.command.Run(os.Stdout, workingDir, args)
	if err != nil {
		state, readErr := e.readBOSHState(statePath)
		if readErr != nil {
			errorList := helpers.Errors{}
			errorList.Add(err)
			errorList.Add(readErr)
			return errorList
		}
		return NewDeleteEnvError(state, err)
	}

	return nil
}

func (e Executor) Version() (string, error) {
	args := []string{"-v"}

	buffer := bytes.NewBuffer([]byte{})
	err := e.command.Run(buffer, e.boshDir, args)
	if err != nil {
		return "", err
	}

	versionOutput := buffer.String()
	regex := regexp.MustCompile(`\d+.\d+.\d+`)

	version := regex.FindString(versionOutput)
	if version == "" {
		return "", NewBOSHVersionError(errors.New("BOSH version could not be parsed"))
	}

	return version, nil
}

func (e Executor) writePreviousFiles(state map[string]interface{}, variables, manifest, workingDir string) error {
	statePath := fmt.Sprintf("%s/state.json", workingDir)
	variablesPath := fmt.Sprintf("%s/variables.yml", workingDir)
	boshManifestPath := filepath.Join(workingDir, "manifest.yml")

	if state != nil {
		boshStateContents, err := e.marshalJSON(state)
		if err != nil {
			return err
		}
		err = e.writeFile(statePath, boshStateContents, os.ModePerm)
		if err != nil {
			return err
		}
	}

	err := e.writeFile(variablesPath, []byte(variables), os.ModePerm)
	if err != nil {
		// not tested
		return err
	}

	err = e.writeFile(boshManifestPath, []byte(manifest), os.ModePerm)
	if err != nil {
		// not tested
		return err
	}

	return nil
}
