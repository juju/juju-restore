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
func NewRestorer(db Database) *Restorer {
	return &Restorer{db: db}
}

// Restorer checks the database health and backup file state and
// restores the backup file into the database.
type Restorer struct {
	db Database
}

// CheckDatabaseState determines whether this database is appropriate
// for restoring into.
func (r *Restorer) CheckDatabaseState() error {
	replicaSet, err := r.db.ReplicaSet()
	if err != nil {
		return errors.Trace(err)
	}

	logger.Tracef("replicaset: %s", pretty.Sprint(replicaSet))

	var primary *ReplicaSetMember
	var unhealthyMembers []ReplicaSetMember
	for _, member := range replicaSet.Members {
		if member.State == statePrimary {
			// Keep a copy so we don't overwrite in the loop.
			saved := member
			primary = &saved
		}
		validState := member.State == statePrimary || member.State == stateSecondary
		if !member.Healthy || !validState {
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
		return errors.Errorf("not running on primary replica set member %s", primary)
	}

	return nil
}
