// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"bytes"
	"fmt"
	"os/exec"

	"github.com/juju/errors"
)

// CommandRunner defines what is needed to run a command on a machine.
type CommandRunner interface {
	// All strings within the command must be individually passed-in.
	// For example,
	//     to run 'echo hi:D', pass in "echo", "hi:D",
	//     to stop juju-db, pass in "systemctl", "stop", "juju-db".
	Run(commands ...string) (string, error)
}

type localRunner struct{}

// NewLocalRunner constructs a command runner that runs commands locally.
func NewLocalRunner() CommandRunner {
	return &localRunner{}
}

// Run implements CommandRunner.Run.
func (r *localRunner) Run(commands ...string) (string, error) {
	// Since we are logged in as a 'ubuntu' user,
	// we need to run in sudo to switch to 'root' user to elevate privileges.
	customSSH := exec.Command(commands[0], commands[1:]...)
	var out bytes.Buffer
	customSSH.Stdout = &out
	cmdErr := bytes.Buffer{}
	customSSH.Stderr = &cmdErr
	if err := customSSH.Run(); err != nil {
		if cmdErr.Len() > 0 {
			return "", errors.New(cmdErr.String())
		}
		return "", err
	}
	return out.String(), nil
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
	args := []string{
		"sudo",
		"ssh",
		"-o", "StrictHostKeyChecking no",
		"-t", "-t", // twice to force tty allocation, even if ssh jas no local tty
		"-i", "/var/lib/juju/system-identity", // only root can read /var/lib/juju/system-identity
		fmt.Sprintf("ubuntu@%v", r.ip),
	}
	args = append(args, commands...)
	return r.localRunner.Run(args...)
}
