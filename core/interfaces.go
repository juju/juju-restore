// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package core

import "fmt"

// Database represents a connection to MongoDB and abstracts the
// operations the core needs to apply as part of restoring a backup..
type Database interface {
	ReplicaSet() (ReplicaSet, error)
	Close() error
}

// ReplicaSet holds information about the members of a replica set and
// its status.
type ReplicaSet struct {
	Name    string
	Members []ReplicaSetMember
}

// ReplicaSetMember holds status informatian about a database replica
// set member.
type ReplicaSetMember struct {
	ID      int
	Name    string
	Self    bool
	Healthy bool
	State   string
}

// String is part of Stringer.
func (m ReplicaSetMember) String() string {
	return fmt.Sprintf("%d %q (self=%v healthy=%v state=%q)",
		m.ID, m.Name, m.Self, m.Healthy, m.State,
	)
}

const (
	statePrimary   = "PRIMARY"
	stateSecondary = "SECONDARY"
)
