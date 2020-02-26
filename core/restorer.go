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
		if !validState || !member.Healthy || member.JujuMachineID == "" {
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

// StopAgents stops controller agents, jujud-machine-*.
// If stopSecondaries is true, these agents on other controller nodes will be stopped
// as well.
// The agents on the primary node are always stopped last.
func (r *Restorer) StopAgents(stopSecondaries bool) map[string]error {
	// When stopping agents we want to stop primary last in an attempt to
	// avoid re-election now - we are stopping anyway.
	return r.manageAgents(stopSecondaries, func(n ControllerNode) error { return n.StopAgent() }, false)
}

// StartAgents starts controller agents, jujud-machine-*.
// If stopSecondaries is true, these agents on other controller nodes will be started
// as well.
// The agents on the primary node are always started first.
func (r *Restorer) StartAgents(startSecondaries bool) map[string]error {
	// Check replicaset is healthy before restarting agents.
	r.replicaSetStabilised()
	// When starting agents we want to start primary first in an attempt to
	// preserve it being a primary.
	return r.manageAgents(startSecondaries, func(n ControllerNode) error { return n.StartAgent() }, true)
}

// TODO Need to figure out how to wait for stability....
// Is this enough?..
func (r *Restorer) replicaSetStabilised() {
	// We want to refresh replicaset as we go...
	attempt := 0
	// keep a copy of replicaset, just in case
	pre := r.replicaSet
	for {
		attempt++
		logger.Debugf("checking db health, attempt %d", attempt)
		replicaSet, err := r.db.ReplicaSet()
		if err != nil {
			logger.Errorf("getting database replica set: %v", err)
			continue
		}
		r.replicaSet = replicaSet
		err = r.CheckDatabaseState()
		if err == nil {
			logger.Debugf("replicaset is healthy again")
			break
		} else {
			logger.Debugf(" replicaset is sick: %v", err)
		}
		if attempt == 20 {
			r.replicaSet = pre
			logger.Debugf("Could not finish waiting for healthy replicaset")
			break
		}
	}
}

func (r *Restorer) manageAgents(all bool, operation func(n ControllerNode) error, primaryFirst bool) map[string]error {
	var primary ControllerNode
	result := map[string]error{}
	secondaries := []ControllerNode{}
	for _, member := range r.replicaSet.Members {
		memberMachine := r.convertToControllerNode(member)
		if member.Self {
			primary = memberMachine
			continue
		}
		if all {
			secondaries = append(secondaries, memberMachine)
		}
	}
	if primaryFirst {
		result[primary.IP()] = operation(primary)
	}
	for _, n := range secondaries {
		result[n.IP()] = operation(n)
	}
	if !primaryFirst {
		result[primary.IP()] = operation(primary)
	}
	return result
}
