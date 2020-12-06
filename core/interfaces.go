// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package core

import (
	"fmt"
	"time"

	"github.com/juju/version"
)

// Database represents a connection to MongoDB and abstracts the
// operations the core needs to apply as part of restoring a backup.
type Database interface {
	// ReplicaSet gets the status of the replica set and all members.
	ReplicaSet() (ReplicaSet, error)

	// ControllerInfo gets information about the controller that we
	// can compare to the backup file.
	ControllerInfo() (ControllerInfo, error)

	// RestoreFromDump restores the database dump in the directory
	// passed in to the database and writes progress logging to the
	// specified path.
	RestoreFromDump(dumpDir string, logFile string, includeStatusHistory bool) error

	// Reconnect reconnects to the database if the database agents
	// have been restarted.
	Reconnect() error

	// Close terminates the database connection.
	Close()
}

// ReplicaSet holds information about the members of a replica set and
// its status.
type ReplicaSet struct {
	// Name of the replica set - this will be "juju" for replica sets
	// that juju has created.
	Name string

	// Members lists the nodes that make up the set.
	Members []ReplicaSetMember
}

// ControllerInfo holds identifying information about a Juju controller.
type ControllerInfo struct {
	// ControllerModelUUID is the controller model UUID for this controller.
	ControllerModelUUID string

	// JujuVersion is the version of Juju running on this controller.
	JujuVersion version.Number

	// Series is the OS series the controller is deployed on. Ths
	// determines what version of mongo is installed and whether we
	// can restore a given backup.
	Series string

	// HANodes is the count of controller machines.
	HANodes int
}

// ReplicaSetMember holds status information about a database replica
// set member.
type ReplicaSetMember struct {
	// ID is unique across the nodes.
	ID int

	// Name will contain the ip-address:port in the case of
	// Juju-created replica sets.
	Name string

	// Self will be true for the member we're currently connected to,
	// false for the others.
	Self bool

	// Healthy indicates whether there's some problem with the node.
	Healthy bool

	// State should be PRIMARY or SECONDARY, but if there's a problem
	// with the replica set it could be any one of the values listed
	// at https://docs.mongodb.com/manual/reference/replica-states/
	State string

	// JujuMachineID has Juju machine ID for this controller node.
	// This information is needed when trying to manage Juju agents,
	// their config or any other artifacts created by Juju.
	JujuMachineID string
}

// String is part of Stringer.
func (m ReplicaSetMember) String() string {
	return fmt.Sprintf("%d %q (juju machine %v)", m.ID, m.Name, m.JujuMachineID)
}

// ServiceType is used to pick which service to stop/start on a
// controller node.
type ServiceType string

const (
	// MachineAgentService is the jujud-machine-n service on the
	// machine.
	MachineAgentService ServiceType = "machine-agent"

	// DatabaseService is the juju-db service on the machine.
	DatabaseService ServiceType = "database"
)

// ControllerNode defines behavior for a controller node machine.
type ControllerNode interface {
	// IP returns IP address of the machine.
	IP() string

	// Status checks connection to the controller machine and returns
	// some information about it.
	Status() (NodeStatus, error)

	// StopAgent stops the jujud-machine-* or juju-db service on the
	// controller node.
	StopService(ServiceType) error

	// StartAgent starts the jujud-machine-* or juju-db service on the
	// controller node.
	StartService(ServiceType) error

	// SnapshotDatabase copies the Mongo data directory to a temporary
	// location (which is returned). Requires that juju-db isn't
	// running.
	SnapshotDatabase() (string, error)

	// RestoreSnapshot replaces the Mongo data directory with the
	// specified snapshot. Requires that juju-db isn't running.
	RestoreSnapshot(string) error

	// DiscardSnapshot deletes an unused snapshot from the machine.
	DiscardSnapshot(string) error

	// UpdateAgentVersion changes the tools symlink and agent.conf for
	// this machine to match the specified version.
	UpdateAgentVersion(version.Number) error
}

// NodeStatus holds information about a controller node.
type NodeStatus struct {
	// FreeSpace is the amount of free space (in bytes?) on the
	// filesystem containing /var/lib/juju
	FreeSpace int64

	// DatabaseSize is the size (in bytes?) of the mongo db on disk.
	DatabaseSize int64

	// MachineAgentRunning is whether the machine agent jujud is
	// running on the machine.
	MachineAgentRunning bool

	// DatabaseRunning is whether the juju-db service is running on
	// the machine.
	DatabaseRunning bool
}

// PrecheckResult contains the results of a pre-check run.
type PrecheckResult struct {
	// BackupDate is the date the backup was finished.
	BackupDate time.Time

	// ControllerModelUUID is the controller model UUID from which
	// backup was taken.
	ControllerModelUUID string

	// BackupJujuVersion is the Juju version of the controller from which backup was taken.
	BackupJujuVersion version.Number

	// ControllerJujuVersion is the Juju version of the controller
	// we're restoring into. If it's greater than BackupJujuVersion
	// (disregarding build number) then restoring this version is also
	// a downgrade.
	ControllerJujuVersion version.Number

	// ModelCount is the count of models that this backup contains.
	ModelCount int
}

const (
	statePrimary   = "PRIMARY"
	stateSecondary = "SECONDARY"
)

// BackupFile represents a specific backup file and provides methods
// for getting information from it.
type BackupFile interface {
	// Metadata retrieves identifying information from the backup file
	// and returns it.
	Metadata() (BackupMetadata, error)

	// DumpDirectory returns the path of the database dump to be
	// restored.
	DumpDirectory() string

	// Close indicates the backup file is not needed anymore so any
	// temp space used can be freed.
	Close() error
}

// BackupMetadata holds interesting information about a backup file.
type BackupMetadata struct {
	// FormatVersion tells us which version of the backup structure
	// this file uses. If one wasn't specified in the file, it's
	// version 0.
	FormatVersion int64

	// ControllerModelUUID is the model UUID of the backed up
	// controller model.
	ControllerModelUUID string

	// JujuVersion is the Juju version of the controller from which
	// the backup was taken.
	JujuVersion version.Number

	// Series is the OS series the backup was taken on. This will
	// determine the version of mongo that's installed and will need
	// to match the restore target.
	Series string

	// BackupCreated stores when this backup was created.
	BackupCreated time.Time

	// Hostname stores the name of the machine that created the
	// backup.
	Hostname string

	// ContainsLogs will be true if this backup includes log
	// collections.
	ContainsLogs bool

	// ModelCount reports how many models are contained in the backup.
	ModelCount int

	// HANodes is the number of machines in the controller that was
	// backed up.
	HANodes int
}
