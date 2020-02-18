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

// ReplicaSetMember holds status informatian about a database replica
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
}

// PrecheckResult contains the results of a pre-check run.
type PrecheckResult struct {
	// BackupDate is the date the backup was finished.
	BackupDate time.Time

	// ControllerUUID is the controller UUID from which backup was taken.
	ControllerUUID string

	// JujuVersion is the Juju version of the controller from which backup was taken.
	JujuVersion version.Number

	// ModelCount is the count of models that this backup contains.
	ModelCount int64
}

// String is part of Stringer.
func (m ReplicaSetMember) String() string {
	return fmt.Sprintf("%d %q", m.ID, m.Name)
}

const (
	statePrimary   = "PRIMARY"
	stateSecondary = "SECONDARY"
)
