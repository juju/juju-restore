// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"fmt"
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
	runner := NewLocalRunner()
	if !member.Self {
		runner = NewRemoteRunner(ip)
	}
	return New(ip, member.JujuMachineID, runner)
}

type Machine struct {
	ip string

	jujuID  string
	command CommandRunner
}

// New returns a machine that satisfies core.ControllerNode.
func New(ip string, jujuID string, runner CommandRunner) *Machine {
	return &Machine{ip, jujuID, runner}
}

// IP implements ControllerNode.IP.
func (r *Machine) IP() string {
	return r.ip
}

// Ping implements ControllerNode.Ping()
// by ssh'ing into the machine and executing an 'echo' command.
func (r *Machine) Ping() error {
	command := fmt.Sprintf("hello from %v", r.IP())
	out, err := r.command.Run("echo", command)
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
	return r.ctrlAgent("stop")
}

// StartAgent implements ControllerNode.StartAgent.
func (r *Machine) StartAgent() error {
	return r.ctrlAgent("start")
}

func (r *Machine) ctrlAgent(op string) error {
	command := []string{"sudo", "systemctl", op, fmt.Sprintf("jujud-machine-%v", r.jujuID)}
	out, err := r.command.Run(command...)
	if err != nil {
		return errors.Trace(err)
	}
	if out != "" {
		return errors.Errorf("start agent command should not have returned any output, but got %v", out)
	}
	return nil
}
