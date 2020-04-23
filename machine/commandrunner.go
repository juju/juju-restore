// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/juju/errors"
)

// CommandRunner defines what is needed to run a command on a machine.
type CommandRunner interface {
	// All strings within the command must be individually passed-in.
	// For example,
	//     to run 'echo hi:D', pass in "echo", "hi:D",
	//     to stop juju-db, pass in "systemctl", "stop", "juju-db".
	Run(commands ...string) (string, error)
	RunScript(script string, args ...string) (string, error)
}

type localRunner struct{}

// NewLocalRunner constructs a command runner that runs commands locally.
func NewLocalRunner() CommandRunner {
	return &localRunner{}
}

// Run implements CommandRunner.Run.
func (r *localRunner) Run(commands ...string) (string, error) {
	customSSH := exec.Command(commands[0], commands[1:]...)
	var out, cmdErr bytes.Buffer
	customSSH.Stdout = &out
	customSSH.Stderr = &cmdErr
	if err := customSSH.Run(); err != nil {
		if cmdErr.Len() > 0 {
			// Remove trailing newlines from the output.
			return "", errors.New(strings.TrimSpace(cmdErr.String()))
		}
		return "", err
	}
	return out.String(), nil
}

// RunScript for a local machine can still just run the string
// directly.
func (r *localRunner) RunScript(script string, args ...string) (string, error) {
	fullArgs := []string{"sudo", "bash", "-c", script, "local-script"}
	fullArgs = append(fullArgs, args...)
	return r.Run(fullArgs...)
}

type remoteRunner struct {
	*localRunner
	ip string
}

// NewRemoteRunner constructs a command runner that runs commands remotely using ssh.
func NewRemoteRunner(ip string) CommandRunner {
	return &remoteRunner{&localRunner{}, ip}
}

// Run implements CommandRunner.Run.
func (r *remoteRunner) Run(commands ...string) (string, error) {
	// Since we are logged in as a 'ubuntu' user,
	// we need to run in sudo to read the identity file.
	args := []string{
		"sudo",
		"ssh",
		"-o", "StrictHostKeyChecking no",
		"-i", "/var/lib/juju/system-identity",
		fmt.Sprintf("ubuntu@%v", r.ip),
		strings.Join(commands, " "), // The commands should be sent to the target as one string.
	}
	return r.localRunner.Run(args...)
}

// RunScript on a remote machine needs to scp the script over and then
// run it.
func (r *remoteRunner) RunScript(script string, args ...string) (string, error) {
	scriptFile, err := ioutil.TempFile("/tmp", "juju-restore-script")
	if err != nil {
		return "", errors.Annotate(err, "creating tempfile")
	}
	defer func() {
		_ = scriptFile.Close()
		_ = os.Remove(scriptFile.Name())
	}()
	_, err = io.WriteString(scriptFile, script)
	if err != nil {
		return "", errors.Trace(err)
	}
	err = scriptFile.Close()
	if err != nil {
		return "", errors.Trace(err)
	}
	err = r.scpTempScript(filepath.Base(scriptFile.Name()))
	if err != nil {
		return "", errors.Annotatef(err, "scping script to %s", r.ip)
	}
	fullArgs := []string{"sudo", "bash", scriptFile.Name()}
	fullArgs = append(fullArgs, args...)
	return r.Run(fullArgs...)
}

// copyTempScript copies a script file from /tmp locally to /tmp on
// the target.
func (r *remoteRunner) scpTempScript(name string) error {
	path := filepath.Join("/tmp", name)
	args := []string{
		"sudo",
		"scp",
		"-o", "StrictHostKeyChecking no",
		"-i", "/var/lib/juju/system-identity",
		path,
		fmt.Sprintf("ubuntu@%s:%s", r.ip, path),
	}
	_, err := r.localRunner.Run(args...)
	return errors.Trace(err)
}
