// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/version"

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

// Machine represents a juju controller machine and holds a runner for
// running commands on that machine (whether it's the current machine
// or a different one).
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
func (m *Machine) IP() string {
	return m.ip
}

// String reports "machine n (ip)".
func (m *Machine) String() string {
	return fmt.Sprintf("machine %s (%s)", m.jujuID, m.ip)
}

// Ping implements ControllerNode.Ping()
// by ssh'ing into the machine and executing an 'echo' command.
func (m *Machine) Ping() error {
	message := fmt.Sprintf("hello from %v", m.IP())
	out, err := m.command.Run("echo", message)
	if err != nil {
		return err
	}
	// echo will add a carriage return, \n
	expectedOut := fmt.Sprintf("%v\n", message)
	if out != expectedOut {
		return errors.Errorf("ping controller machine %v failed: expected %q, got %q", m.IP(), expectedOut, out)
	}
	return nil
}

// StopAgent implements ControllerNode.StopAgent.
func (m *Machine) StopAgent() error {
	return m.ctrlAgent("stop")
}

// StartAgent implements ControllerNode.StartAgent.
func (m *Machine) StartAgent() error {
	return m.ctrlAgent("start")
}

func (m *Machine) ctrlAgent(op string) error {
	command := []string{"sudo", "systemctl", op, fmt.Sprintf("jujud-machine-%v", m.jujuID)}
	out, err := m.command.Run(command...)
	if err != nil {
		return errors.Trace(err)
	}
	if out != "" {
		return errors.Errorf("start agent command should not have returned any output, but got %v", out)
	}
	return nil
}

// UpdateAgentVersion edits the agent.conf and updates the symlink to
// point to the tools for the specified version.
func (m *Machine) UpdateAgentVersion(targetVersion version.Number) error {
	out, err := m.command.RunScript(updateAgentVersionScript, m.jujuID, targetVersion.String())
	if err != nil {
		return errors.Trace(err)
	}
	if out != "" {
		return errors.Errorf("update agent script shouldn't have returned any output but got %v", out)
	}
	return nil
}

const updateAgentVersionScript = `
set -e
cd /var/lib/juju/tools
target_tools_dir=$(ls -1d $2-*-* | head -n 1)
if [ ! -d "$target_tools_dir" ]; then
    echo "no tools directory for version $2, can't downgrade"
    exit 1
fi
ln -s --no-dereference --force "$target_tools_dir" "machine-$1"
cd "/var/lib/juju/agents/machine-$1"
sed --in-place=.bkup "s/^upgradedToVersion:.*$/upgradedToVersion: $2/1" agent.conf
`
