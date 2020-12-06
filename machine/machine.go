// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/version"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju-restore/core"
)

var logger = loggo.GetLogger("juju-restore.machine")

const (
	dbPath = "/var/lib/juju"
)

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

// Status implements ControllerNode.Status() by sshing to it to run a
// few commands.
func (m *Machine) Status() (core.NodeStatus, error) {
	out, err := m.command.RunScript(statusScript)
	if err != nil {
		return core.NodeStatus{}, err
	}
	var outDoc struct {
		FreeSpace          int64  `yaml:"free-space"`
		DatabaseSize       int64  `yaml:"db-size"`
		DatabaseStatus     string `yaml:"db-status"`
		MachineAgentStatus string `yaml:"machine-agent-status"`
	}

	err = yaml.Unmarshal([]byte(out), &outDoc)
	if err != nil {
		return core.NodeStatus{}, errors.Annotatef(err, "getting status from %s", m)
	}
	return core.NodeStatus{
		FreeSpace:           outDoc.FreeSpace,
		DatabaseSize:        outDoc.DatabaseSize,
		MachineAgentRunning: outDoc.MachineAgentStatus == "active",
		DatabaseRunning:     outDoc.DatabaseStatus == "active",
	}, nil
}

const statusScript = `
set -e
echo free-space: $(df -B1 --output=avail /var/lib/juju/db | tail -1)
echo db-size: $(du -sB1 /var/lib/juju/db | cut -f 1)
echo db-status: $(systemctl is-active juju-db.service)
echo machine-agent-status: $(systemctl is-active jujud-machine-*.service)
`

// StopService implements ControllerNode.StopService.
func (m *Machine) StopService(stype core.ServiceType) error {
	return m.ctrlService("stop", stype)
}

// StartService implements ControllerNode.StartService.
func (m *Machine) StartService(stype core.ServiceType) error {
	return m.ctrlService("start", stype)
}

func (m *Machine) serviceName(stype core.ServiceType) string {
	switch stype {
	case core.MachineAgentService:
		return fmt.Sprintf("jujud-machine-%v", m.jujuID)
	case core.DatabaseService:
		return "juju-db"
	default:
		logger.Errorf("unknown service type %q", stype)
		return "unknown-service"
	}
}

func (m *Machine) ctrlService(op string, stype core.ServiceType) error {
	command := []string{"sudo", "systemctl", op, m.serviceName(stype)}
	out, err := m.command.Run(command...)
	if err != nil {
		return errors.Trace(err)
	}
	if out != "" {
		return errors.Errorf("start agent command should not have returned any output, but got %v", out)
	}
	return nil
}

const (
	snapshotTemplate = "db-snapshot-"
	snapshotScript   = `
set -e
templateBase="$1"
destDir=$(mktemp -d --tmpdir=/var/lib/juju "${templateBase}XXX")
cp -r /var/lib/juju/db/* $destDir
echo $destDir
`
)

// SnapshotDatabase is part of core.ControllerNode.
func (m *Machine) SnapshotDatabase() (string, error) {
	output, err := m.command.RunScript(snapshotScript, snapshotTemplate)
	if err != nil {
		return "", errors.Annotatef(err, "snapshotting database on %s", m)
	}
	// Chop off the path and db-snapshot part of the new directory
	// name - we don't want people to be able to "restore" or discard
	// arbitrary directories.
	name := filepath.Base(strings.TrimSpace(output))[len(snapshotTemplate):]
	return name, nil
}

// DiscardSnapshot is part of core.ControllerNode.
func (m *Machine) DiscardSnapshot(name string) error {
	fullSnapshotPath := fmt.Sprintf("/var/lib/juju/%s%s", snapshotTemplate, name)
	_, err := m.command.Run("sudo", "rm", "-r", fullSnapshotPath)
	if err != nil {
		return errors.Annotatef(err, "discarding snapshot %q on %s", name, m)
	}
	return nil
}

const restoreScript = `
set -e
snapshotPath="/var/lib/juju/db-snapshot-$1"
if [ ! -d "$snapshotPath" ]; then
    echo "snapshot $snapshotPath not found"
    exit 1
fi
rm -r /var/lib/juju/db
mv "$snapshotPath" /var/lib/juju/db
`

// RestoreSnapshot is part of core.ControllerNode.
func (m *Machine) RestoreSnapshot(name string) error {
	_, err := m.command.RunScript(restoreScript, name)
	if err != nil {
		return errors.Annotatef(err, "restoring snapshot %q on %s", name, m)
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
