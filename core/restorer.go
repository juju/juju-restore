// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package core

import (
	"sort"
	"strings"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/version"
	"github.com/kr/pretty"
	"gopkg.in/retry.v1"
)

var logger = loggo.GetLogger("juju-restore.core")

// ControllerNodeFactory gets a controller node machine from a
// replicaset member.
type ControllerNodeFactory func(member ReplicaSetMember) ControllerNode

// NewRestorer returns a new restorer for a specific database and
// backup.
func NewRestorer(db Database, backup BackupFile, convert ControllerNodeFactory) (*Restorer, error) {
	replicaSet, err := db.ReplicaSet()
	if err != nil {
		return nil, errors.Annotate(err, "getting database replica set")
	}
	return &Restorer{
		db:                      db,
		backup:                  backup,
		replicaSet:              replicaSet,
		convertToControllerNode: convert,
	}, nil
}

// Restorer checks the database health and backup file state and
// restores the backup file.
type Restorer struct {
	db                      Database
	backup                  BackupFile
	replicaSet              ReplicaSet
	convertToControllerNode ControllerNodeFactory
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
	return r.manageAgents(stopSecondaries, false, func(n ControllerNode) error {
		return n.StopAgent()
	})
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
	return r.manageAgents(startSecondaries, true, func(n ControllerNode) error {
		return n.StartAgent()
	})
}

func (r *Restorer) replicaSetStabilised() {
	// keep a copy of replicaset, in case all exponential attempts fail.
	pre := r.replicaSet

	checkReplicaset := func() error {
		replicaSet, err := r.db.ReplicaSet()
		if err != nil {
			return errors.Annotate(err, "getting database replica set")
		}
		// We want to refresh replicaset as we go...
		r.replicaSet = replicaSet
		err = r.CheckDatabaseState()
		if err != nil {
			return errors.Annotate(err, "replicaset is sick")
		}
		return nil
	}

	attempt := retry.Start(
		retry.LimitCount(20, retry.Exponential{
			Initial: 5 * time.Second,
			Factor:  1.6,
		}),
		clock.WallClock,
	)

	var err error
	for attempt.Next() {
		err = checkReplicaset()
		if err == nil {
			logger.Debugf("replicaset is healthy")
			break
		}
		if attempt.More() {
			logger.Debugf("replicaset is sick (retrying, attempt %v): %v", attempt.Count(), err)
		}
	}
	if err != nil {
		r.replicaSet = pre
		logger.Errorf("Could not finish waiting for healthy replicaset")
	}
}

func (r *Restorer) manageAgents(all bool, primaryFirst bool, operation func(n ControllerNode) error) map[string]error {
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

// CheckRestorable checks whether the backup file can be restored into
// the target database.
func (r *Restorer) CheckRestorable(allowDowngrade bool) (*PrecheckResult, error) {
	backup, err := r.backup.Metadata()
	if err != nil {
		return nil, errors.Annotate(err, "getting backup metadata")
	}
	controller, err := r.db.ControllerInfo()
	if err != nil {
		return nil, errors.Annotate(err, "getting controller info")
	}

	// Disregard differences in build numbers - we don't want to
	// prevent restores when fixing code bugs.
	controllerVersion := controller.JujuVersion
	controllerVersion.Build = 0
	backupVersion := backup.JujuVersion
	backupVersion.Build = 0

	if allowDowngrade {
		if backupVersion.Compare(controllerVersion) == 1 {
			return nil, errors.Errorf("backup juju version %q is greater than controller version %q",
				backup.JujuVersion,
				controller.JujuVersion,
			)

		}
	} else if backupVersion.Compare(controllerVersion) == -1 {
		return nil, errors.Errorf("restoring backup would downgrade from juju %q to %q - pass --allow-downgrade if this is intended", controllerVersion, backupVersion)
	} else if controllerVersion != backupVersion {
		return nil, errors.Errorf("juju versions don't match - backup: %q, controller: %q",
			backup.JujuVersion,
			controller.JujuVersion,
		)
	}

	if backup.ControllerModelUUID != controller.ControllerModelUUID {
		return nil, errors.Errorf("controller model uuids don't match - backup: %q, controller: %q",
			backup.ControllerModelUUID,
			controller.ControllerModelUUID,
		)
	}

	if backup.HANodes != controller.HANodes {
		return nil, errors.Errorf("controller HA node counts don't match - backup: %d, controller: %d",
			backup.HANodes,
			controller.HANodes,
		)
	}

	if backup.Series != controller.Series {
		return nil, errors.Errorf("controller series don't match - backup: %q, controller: %q",
			backup.Series,
			controller.Series,
		)
	}

	return &PrecheckResult{
		BackupDate:            backup.BackupCreated,
		ControllerModelUUID:   backup.ControllerModelUUID,
		BackupJujuVersion:     backup.JujuVersion,
		ControllerJujuVersion: controller.JujuVersion,
		ModelCount:            backup.ModelCount,
	}, nil
}

// Restore replaces the database's contents with the data from the
// backup's database dump.
func (r *Restorer) Restore(logPath string, includeStatusHistory bool) error {
	controller, err := r.db.ControllerInfo()
	if err != nil {
		return errors.Annotate(err, "getting controller info")
	}
	metadata, err := r.backup.Metadata()
	if err != nil {
		return errors.Annotatef(err, "getting backup metadata")
	}
	logger.Debugf("restoring dump")
	err = r.db.RestoreFromDump(r.backup.DumpDirectory(), logPath, includeStatusHistory)
	if err != nil {
		return errors.Annotatef(err, "restoring dump from %q", r.backup.DumpDirectory())
	}
	if controller.JujuVersion != metadata.JujuVersion {
		logger.Debugf("updating controller agent versions to %s", metadata.JujuVersion)
		results := r.manageAgents(true, true, func(n ControllerNode) error {
			logger.Debugf("    %s", n)
			err := n.UpdateAgentVersion(metadata.JujuVersion)
			return errors.Annotatef(err, "updating %s", n)
		})
		if err := collectMachineErrors(results); err != nil {
			return errors.Annotatef(err, "problems updating controllers to version %q", metadata.JujuVersion)
		}
	}
	return nil
}

func collectMachineErrors(results map[string]error) error {
	var messages []string
	for _, err := range results {
		if err == nil {
			continue
		}
		messages = append(messages, err.Error())
	}
	if len(messages) == 0 {
		return nil
	}
	// Ensure they're reported in a consistent order.
	sort.Strings(messages)
	return errors.Errorf(strings.Join(messages, "\n"))
}

func versionsMatchExcludingBuild(a, b version.Number) bool {
	a.Build = 0
	b.Build = 0
	return a == b
}
