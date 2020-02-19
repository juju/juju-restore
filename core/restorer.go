// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package core

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/kr/pretty"
)

var logger = loggo.GetLogger("juju-restore.core")

// NewRestorer returns a new restorer for a specific database and
// backup.
func NewRestorer(db Database, convert func(member ReplicaSetMember) ControllerNode) (*Restorer, error) {
	replicaSet, err := db.ReplicaSet()
	if err != nil {
		return nil, errors.Annotate(err, "getting database replica set")
	}
	return &Restorer{db, replicaSet, convert}, nil
}

// Restorer checks the database health and backup file state and
// restores the backup file.
type Restorer struct {
	db                      Database
	replicaSet              ReplicaSet
	convertToControllerNode func(member ReplicaSetMember) ControllerNode
}

// CheckDatabaseState determines whether this database is appropriate
// for restoring into.
func (r *Restorer) CheckDatabaseState() error {
	logger.Debugf("replicaset status: %s", pretty.Sprint(r.replicaSet))
	var primary *ReplicaSetMember
	var unhealthyMembers []ReplicaSetMember
	for _, member := range r.replicaSet.Members {
		if member.State == statePrimary {
			// We need to put member into a new variable, otherwise
			// the value pointed at by primary will be overwritten the
			// next time around the loop.
			saved := member
			primary = &saved
		}
		validState := member.State == statePrimary || member.State == stateSecondary
		if !validState || !member.Healthy {
			unhealthyMembers = append(unhealthyMembers, member)
		}
	}

	if len(unhealthyMembers) != 0 {
		return errors.Trace(NewUnhealthyMembersError(unhealthyMembers))
	}
	if primary == nil {
		return errors.Errorf("no primary found in replica set")
	}
	if !primary.Self {
		return errors.Errorf("not running on primary replica set member, primary is %s", primary)
	}
	return nil
}

// IsHA returns true of there is more than one member in replica set.
func (r *Restorer) IsHA() bool {
	return len(r.replicaSet.Members) > 1
}

// CheckSecondaryControllerNodes determines whether secondary controller nodes can be reached.
func (r *Restorer) CheckSecondaryControllerNodes() map[string]error {
	reachable := map[string]error{}
	for _, member := range r.replicaSet.Members {
		if member.Self {
			// We are already on this machine, so no need to check connectivity.
			continue
		}
		memberMachine := r.convertToControllerNode(member)
		reachable[memberMachine.IP()] = memberMachine.Ping()
	}
	return reachable
}
