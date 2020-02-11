// Copyright 2020 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package core_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju-restore/core"
)

type restorerSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&restorerSuite{})

func (s *restorerSuite) TestCheckDatabaseStateUnhealthyMembers(c *gc.C) {
	r := core.NewRestorer(&fakeDatabase{
		replicaSet: core.ReplicaSet{
			Members: []core.ReplicaSetMember{{
				Healthy: false,
				ID:      1,
				Name:    "kaira-ba",
				State:   "SECONDARY",
			}, {
				Healthy: true,
				ID:      2,
				Name:    "djula",
				State:   "PRIMARY",
			}, {
				Healthy: true,
				ID:      3,
				Name:    "bibi",
				State:   "OUCHY",
			}},
		},
	})

	err := r.CheckDatabaseState()
	c.Assert(err, jc.Satisfies, core.IsUnhealthyMembersError)
	c.Assert(err, gc.ErrorMatches, "unhealthy replica set members: "+
		`1 "kaira-ba" \(self=false healthy=false state="SECONDARY"\), `+
		`3 "bibi" \(self=false healthy=true state="OUCHY"\)`)
}

func (s *restorerSuite) TestCheckDatabaseStateNoPrimary(c *gc.C) {
	r := core.NewRestorer(&fakeDatabase{
		replicaSet: core.ReplicaSet{
			Members: []core.ReplicaSetMember{{
				Healthy: true,
				ID:      1,
				Name:    "kaira-ba",
				State:   "SECONDARY",
			}, {
				Healthy: true,
				ID:      2,
				Name:    "djula",
				State:   "SECONDARY",
			}, {
				Healthy: true,
				ID:      3,
				Name:    "bibi",
				State:   "SECONDARY",
			}},
		},
	})
	err := r.CheckDatabaseState()
	c.Assert(err, gc.ErrorMatches, "no primary found in replica set")
}

func (s *restorerSuite) TestCheckDatabaseStateNotPrimary(c *gc.C) {
	r := core.NewRestorer(&fakeDatabase{
		replicaSet: core.ReplicaSet{
			Members: []core.ReplicaSetMember{{
				Healthy: true,
				ID:      1,
				Name:    "kaira-ba",
				State:   "SECONDARY",
				Self:    true,
			}, {
				Healthy: true,
				ID:      2,
				Name:    "djula",
				State:   "PRIMARY",
			}, {
				Healthy: true,
				ID:      3,
				Name:    "bibi",
				State:   "SECONDARY",
			}},
		},
	})
	err := r.CheckDatabaseState()
	c.Assert(err, gc.ErrorMatches, "not running on primary replica set member "+
		`2 "djula" \(self=false healthy=true state="PRIMARY"\)`,
	)
}

func (s *restorerSuite) TestCheckDatabaseStateAllGood(c *gc.C) {
	r := core.NewRestorer(&fakeDatabase{
		replicaSet: core.ReplicaSet{
			Members: []core.ReplicaSetMember{{
				Healthy: true,
				ID:      1,
				Name:    "kaira-ba",
				State:   "SECONDARY",
			}, {
				Healthy: true,
				ID:      2,
				Name:    "djula",
				State:   "PRIMARY",
				Self:    true,
			}, {
				Healthy: true,
				ID:      3,
				Name:    "bibi",
				State:   "SECONDARY",
			}},
		},
	})
	err := r.CheckDatabaseState()
	c.Assert(err, jc.ErrorIsNil)
}

type fakeDatabase struct {
	testing.Stub
	replicaSet core.ReplicaSet
}

func (db *fakeDatabase) ReplicaSet() (core.ReplicaSet, error) {
	db.Stub.MethodCall(db, "ReplicaSet")
	if err := db.Stub.NextErr(); err != nil {
		return core.ReplicaSet{}, err
	}
	return db.replicaSet, nil
}

func (db *fakeDatabase) Close() error {
	db.Stub.MethodCall(db, "Close")
	return db.NextErr()
}
