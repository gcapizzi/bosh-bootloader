package terraform

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/cloudfoundry/bosh-bootloader/storage"
)

var writeFile func(file string, data []byte, perm os.FileMode) error = ioutil.WriteFile
var readFile func(filename string) ([]byte, error) = ioutil.ReadFile

type Executor struct {
	cmd          terraformCmd
	debug        bool
	terraformDir string
}

type ImportInput struct {
	TerraformAddr string
	AWSResourceID string
	TFState       string
	Creds         storage.AWS
}

type tfOutput struct {
	Sensitive bool
	Type      string
	Value     interface{}
}

type terraformCmd interface {
	Run(stdout io.Writer, workingDirectory string, args []string, debug bool) error
}

func NewExecutor(cmd terraformCmd, debug bool, terraformDir string) Executor {
	return Executor{cmd: cmd, debug: debug, terraformDir: terraformDir}
}

func (e Executor) Apply(input map[string]string, template, prevTFState string) (string, error) {
	err := writeFile(filepath.Join(e.terraformDir, "template.tf"), []byte(template), os.ModePerm)
	if err != nil {
		return "", err
	}

	if prevTFState != "" {
		err = writeFile(filepath.Join(e.terraformDir, "terraform.tfstate"), []byte(prevTFState), os.ModePerm)
		if err != nil {
			return "", err
		}
	}

	err = e.cmd.Run(os.Stdout, e.terraformDir, []string{"init"}, e.debug)
	if err != nil {
		return "", err
	}

	err = writeFile(filepath.Join(e.terraformDir, "terraform.tfvars"), []byte(makeTFVars(input)), os.ModePerm)
	if err != nil {
		return "", err
	}

	args := []string{"apply"}
	err = e.cmd.Run(os.Stdout, e.terraformDir, args, e.debug)
	if err != nil {
		return "", NewExecutorError(filepath.Join(e.terraformDir, "terraform.tfstate"), err, e.debug)
	}

	tfState, err := readFile(filepath.Join(e.terraformDir, "terraform.tfstate"))
	if err != nil {
		return "", err
	}

	return string(tfState), nil
}

func (e Executor) Destroy(input map[string]string, template, prevTFState string) (string, error) {
	err := writeFile(filepath.Join(e.terraformDir, "template.tf"), []byte(template), os.ModePerm)
	if err != nil {
		return "", err
	}

	if prevTFState != "" {
		err = writeFile(filepath.Join(e.terraformDir, "terraform.tfstate"), []byte(prevTFState), os.ModePerm)
		if err != nil {
			return "", err
		}
	}

	err = e.cmd.Run(os.Stdout, e.terraformDir, []string{"init"}, e.debug)
	if err != nil {
		return "", err
	}

	args := []string{"destroy", "-force"}
	for k, v := range input {
		args = append(args, makeVar(k, v)...)
	}
	err = e.cmd.Run(os.Stdout, e.terraformDir, args, e.debug)
	if err != nil {
		return "", NewExecutorError(filepath.Join(e.terraformDir, "terraform.tfstate"), err, e.debug)
	}

	tfState, err := readFile(filepath.Join(e.terraformDir, "terraform.tfstate"))
	if err != nil {
		return "", err
	}

	return string(tfState), nil
}

func (e Executor) Import(input ImportInput) (string, error) {
	resourceType := strings.Split(input.TerraformAddr, ".")[0]
	resourceName := strings.Split(input.TerraformAddr, ".")[1]
	resourceName = strings.Split(resourceName, "[")[0]

	template := fmt.Sprintf(`
provider "aws" {
	region     = %q
	access_key = %q
	secret_key = %q
}

resource %q %q {
}`, input.Creds.Region, input.Creds.AccessKeyID, input.Creds.SecretAccessKey, resourceType, resourceName)

	err := writeFile(filepath.Join(e.terraformDir, "template.tf"), []byte(template), os.ModePerm)
	if err != nil {
		return "", err
	}

	err = writeFile(filepath.Join(e.terraformDir, "terraform.tfstate"), []byte(input.TFState), os.ModePerm)
	if err != nil {
		return "", err
	}

	err = e.cmd.Run(os.Stdout, e.terraformDir, []string{"init"}, e.debug)
	if err != nil {
		return "", err
	}

	err = e.cmd.Run(os.Stdout, e.terraformDir, []string{"import", input.TerraformAddr, input.AWSResourceID}, e.debug)
	if err != nil {
		return "", fmt.Errorf("failed to import: %s", err)
	}

	tfStateContents, err := readFile(filepath.Join(e.terraformDir, "terraform.tfstate"))
	if err != nil {
		return "", err
	}

	return string(tfStateContents), nil
}

func (e Executor) Version() (string, error) {
	buffer := bytes.NewBuffer([]byte{})
	err := e.cmd.Run(buffer, "/tmp", []string{"version"}, true)
	if err != nil {
		return "", err
	}
	versionOutput := buffer.String()
	regex := regexp.MustCompile(`\d+.\d+.\d+`)

	version := regex.FindString(versionOutput)
	if version == "" {
		return "", errors.New("Terraform version could not be parsed")
	}

	return version, nil
}

func (e Executor) Output(tfState, outputName string) (string, error) {
	err := writeFile(filepath.Join(e.terraformDir, "terraform.tfstate"), []byte(tfState), os.ModePerm)
	if err != nil {
		return "", err
	}

	err = e.cmd.Run(os.Stdout, e.terraformDir, []string{"init"}, e.debug)
	if err != nil {
		return "", err
	}

	args := []string{"output", outputName}
	buffer := bytes.NewBuffer([]byte{})
	err = e.cmd.Run(buffer, e.terraformDir, args, true)
	if err != nil {
		return "", err
	}

	return strings.TrimSuffix(buffer.String(), "\n"), nil
}

func (e Executor) Outputs(tfState string) (map[string]interface{}, error) {
	err := writeFile(filepath.Join(e.terraformDir, "terraform.tfstate"), []byte(tfState), os.ModePerm)
	if err != nil {
		return map[string]interface{}{}, err
	}

	err = e.cmd.Run(os.Stdout, e.terraformDir, []string{"init"}, false)
	if err != nil {
		return map[string]interface{}{}, err
	}

	args := []string{"output", "--json"}
	buffer := bytes.NewBuffer([]byte{})
	err = e.cmd.Run(buffer, e.terraformDir, args, true)
	if err != nil {
		return map[string]interface{}{}, err
	}

	var tfOutputs map[string]tfOutput
	err = json.Unmarshal(buffer.Bytes(), &tfOutputs)
	if err != nil {
		return map[string]interface{}{}, err
	}

	outputs := map[string]interface{}{}

	for tfKey, tfValue := range tfOutputs {
		outputs[tfKey] = tfValue.Value
	}

	return outputs, nil
}

func makeVar(name string, value string) []string {
	return []string{"-var", fmt.Sprintf("%s=%s", name, value)}
}

func makeTFVars(input map[string]string) string {
	var tfVars []string

	for k, v := range input {
		tfVars = append(tfVars, fmt.Sprintf(`%s = "%s"`, k, v))
	}

	return strings.Join(tfVars, "\n")
}
