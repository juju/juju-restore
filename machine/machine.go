// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"bytes"
	"fmt"
	"os/exec"

	"github.com/juju/errors"
)

type Machine struct {
	ip string
}

// NewMachine returns a machine that satisfies core.ControllerNode.
func NewMachine(ip string) *Machine {
	return &Machine{ip}
}

// IP implements ControllerNode.IP.
func (r *Machine) IP() string {
	return r.ip
}

// Ping implements ControllerNode.Ping()
// by ssh'ing into the machine and executing an 'echo' command.
func (r *Machine) Ping() error {
	command := fmt.Sprintf("hello from %v", r.IP())
	out, err := runRemoteCommand(r, "echo", command)
	if err != nil {
		return err
	}
	// echo will add a carriage return, \r\n
	expectedOut := fmt.Sprintf("%v\r\n", command)
	if out != expectedOut {
		return errors.Errorf("ping controller machine %v failed: expected %q, got %q", r.IP(), expectedOut, out)
	}
	return nil
}

// runRemoteCommand takes in a command and runs it remotely using ssh.
// All strings within the command must be individually passed-in.
// For example,
//     to run 'echo hi:D', pass in "echo", "hi:D",
//     to stop juju-db, pass in "systemctl", "stop", "juju-db".
var runRemoteCommand = func(r *Machine, commands ...string) (string, error) {
	args := []string{
		"ssh",
		"-o", "StrictHostKeyChecking no",
		"-t", "-t", // twice to force tty allocation, even if ssh jas no local tty
		"-i", "/var/lib/juju/system-identity",
		fmt.Sprintf("ubuntu@%v", r.IP()),
	}
	args = append(args, commands...)
	// Since we are on the primary controller node, logged in as a 'ubuntu' user,
	// we need to run in sudo to switch to 'root' user to elevate privileges
	// since only root can read /var/lib/juju/system-identity.
	customSSH := exec.Command("sudo", args...)
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
