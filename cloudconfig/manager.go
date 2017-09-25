package cloudconfig

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"golang.org/x/net/proxy"

	"github.com/cloudfoundry/bosh-bootloader/bosh"
	"github.com/cloudfoundry/bosh-bootloader/storage"
)

var (
	writeFile   func(string, []byte, os.FileMode) error                               = ioutil.WriteFile
	proxySOCKS5 func(string, string, *proxy.Auth, proxy.Dialer) (proxy.Dialer, error) = proxy.SOCKS5
)

type Manager struct {
	logger             logger
	command            command
	opsGenerator       OpsGenerator
	boshClientProvider boshClientProvider
	socks5Proxy        socks5Proxy
	terraformManager   terraformManager
	sshKeyGetter       sshKeyGetter
	cloudConfigDir     string
}

type logger interface {
	Step(string, ...interface{})
}

type command interface {
	Run(stdout io.Writer, workingDirectory string, args []string) error
}

type OpsGenerator interface {
	Generate(state storage.State) (string, error)
}

type boshClientProvider interface {
	Client(jumpbox storage.Jumpbox, directorAddress, directorUsername, directorPassword, caCert string) (bosh.Client, error)
}

type socks5Proxy interface {
	Start(string, string) error
	Addr() string
}

type terraformManager interface {
	GetOutputs(storage.State) (map[string]interface{}, error)
}

type sshKeyGetter interface {
	Get(storage.State) (string, error)
}

func NewManager(logger logger, cmd command, opsGenerator OpsGenerator, boshClientProvider boshClientProvider,
	socks5Proxy socks5Proxy, terraformManager terraformManager, sshKeyGetter sshKeyGetter, cloudConfigDir string) Manager {
	return Manager{
		logger:             logger,
		command:            cmd,
		opsGenerator:       opsGenerator,
		boshClientProvider: boshClientProvider,
		socks5Proxy:        socks5Proxy,
		terraformManager:   terraformManager,
		sshKeyGetter:       sshKeyGetter,
		cloudConfigDir:     cloudConfigDir,
	}
}

func (m Manager) Generate(state storage.State) (string, error) {
	buf := bytes.NewBuffer([]byte{})
	err := writeFile(filepath.Join(m.cloudConfigDir, "cloud-config.yml"), []byte(BaseCloudConfig), os.ModePerm)
	if err != nil {
		return "", err
	}

	ops, err := m.opsGenerator.Generate(state)
	if err != nil {
		return "", err
	}

	err = writeFile(filepath.Join(m.cloudConfigDir, "ops.yml"), []byte(ops), os.ModePerm)
	if err != nil {
		return "", err
	}

	args := []string{
		"interpolate", fmt.Sprintf("%s/cloud-config.yml", m.cloudConfigDir),
		"-o", fmt.Sprintf("%s/ops.yml", m.cloudConfigDir),
	}

	err = m.command.Run(buf, m.cloudConfigDir, args)
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}

func (m Manager) Update(state storage.State) error {
	boshClient, err := m.boshClientProvider.Client(state.Jumpbox, state.BOSH.DirectorAddress, state.BOSH.DirectorUsername, state.BOSH.DirectorPassword, state.BOSH.DirectorSSLCA)
	if err != nil {
		return err // not tested
	}

	m.logger.Step("generating cloud config")
	cloudConfig, err := m.Generate(state)
	if err != nil {
		return err
	}

	m.logger.Step("applying cloud config")
	err = boshClient.UpdateCloudConfig([]byte(cloudConfig))
	if err != nil {
		return err
	}

	return nil
}
