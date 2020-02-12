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
// restores the backup file.
type Restorer struct {
	db Database
}

// CheckDatabaseState determines whether this database is appropriate
// for restoring into.
func (r *Restorer) CheckDatabaseState() error {
	replicaSet, err := r.db.ReplicaSet()
	if err != nil {
		return errors.Annotate(err, "getting database replica set")
	}

	logger.Debugf("replicaset status: %s", pretty.Sprint(replicaSet))

	var primary *ReplicaSetMember
	var unhealthyMembers []ReplicaSetMember
	for _, member := range replicaSet.Members {
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
