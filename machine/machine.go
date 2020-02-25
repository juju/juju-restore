// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju-restore/core"
)

var logger = loggo.GetLogger("juju-restore.machine")

// ControllerNodeForReplicaSetMember returns ControllerNode for ReplicaSetMember.
func ControllerNodeForReplicaSetMember(member core.ReplicaSetMember) core.ControllerNode {
	//	Replica set member name is in the form <machine IP>:<Mongo port>.
	ip := member.Name[:strings.Index(member.Name, ":")]
	return New(ip, member.JujuMachineID, member.Self)
}

type Machine struct {
	ip string

	jujuID string
	self   bool
}

// New returns a machine that satisfies core.ControllerNode.
func New(ip string, jujuID string, self bool) *Machine {
	return &Machine{ip, jujuID, self}
}

// IP implements ControllerNode.IP.
func (r *Machine) IP() string {
	return r.ip
}

// Ping implements ControllerNode.Ping()
// by ssh'ing into the machine and executing an 'echo' command.
func (r *Machine) Ping() error {
	command := fmt.Sprintf("hello from %v", r.IP())
	if r.self {
		logger.Debugf("%v, self", command)
		return nil
	}
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

// StopAgent implements ControllerNode.StopAgent.
func (r *Machine) StopAgent() error {
	command := []string{"sudo", "systemctl", "stop", fmt.Sprintf("jujud-machine-%v", r.jujuID)}
	var out string
	var err error
	if r.self {
		out, err = runCommand(command...)
	} else {
		out, err = runRemoteCommand(r, command...)
	}
	if err != nil {
		return errors.Trace(err)
	}
	if out != "" {
		return errors.Errorf("stop agent command should not have returned any output, but got %v", out)
	}
	return nil
}

// StartAgent implements ControllerNode.StartAgent.
func (r *Machine) StartAgent() error {
	command := []string{"sudo", "systemctl", "start", fmt.Sprintf("jujud-machine-%v", r.jujuID)}
	var out string
	var err error
	if r.self {
		out, err = runCommand(command...)
	} else {
		out, err = runRemoteCommand(r, command...)
	}
	if err != nil {
		return errors.Trace(err)
	}
	if out != "" {
		return errors.Errorf("start agent command should not have returned any output, but got %v", out)
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
		"sudo",
		"ssh",
		"-o", "StrictHostKeyChecking no",
		"-t", "-t", // twice to force tty allocation, even if ssh jas no local tty
		"-i", "/var/lib/juju/system-identity", // only root can read /var/lib/juju/system-identity
		fmt.Sprintf("ubuntu@%v", r.IP()),
	}
	args = append(args, commands...)
	return runCommand(args...)
}

// runCommand runs a command.
// All strings within the command must be individually passed-in.
var runCommand = func(commands ...string) (string, error) {
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
