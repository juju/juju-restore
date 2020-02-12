// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

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
		replicaSetF: func() (core.ReplicaSet, error) {
			return core.ReplicaSet{
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
			}, nil
		},
	})

	err := r.CheckDatabaseState()
	c.Assert(err, jc.Satisfies, core.IsUnhealthyMembersError)
	c.Assert(err, gc.ErrorMatches, `unhealthy replica set members: 1 "kaira-ba", 3 "bibi"`)
}

func (s *restorerSuite) TestCheckDatabaseStateNoPrimary(c *gc.C) {
	r := core.NewRestorer(&fakeDatabase{
		replicaSetF: func() (core.ReplicaSet, error) {
			return core.ReplicaSet{
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
			}, nil
		},
	})
	err := r.CheckDatabaseState()
	c.Assert(err, gc.ErrorMatches, "no primary found in replica set")
}

func (s *restorerSuite) TestCheckDatabaseStateNotPrimary(c *gc.C) {
	r := core.NewRestorer(&fakeDatabase{
		replicaSetF: func() (core.ReplicaSet, error) {
			return core.ReplicaSet{
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
			}, nil
		},
	})
	err := r.CheckDatabaseState()
	c.Assert(err, gc.ErrorMatches, `not running on primary replica set member, primary is 2 "djula"`)
}

func (s *restorerSuite) TestCheckDatabaseStateAllGood(c *gc.C) {
	r := core.NewRestorer(&fakeDatabase{
		replicaSetF: func() (core.ReplicaSet, error) {
			return core.ReplicaSet{
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
			}, nil
		},
	})
	err := r.CheckDatabaseState()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *restorerSuite) TestCheckDatabaseStateOneMember(c *gc.C) {
	r := core.NewRestorer(&fakeDatabase{
		replicaSetF: func() (core.ReplicaSet, error) {
			return core.ReplicaSet{
				Members: []core.ReplicaSetMember{{
					Healthy: true,
					ID:      2,
					Name:    "djula",
					State:   "PRIMARY",
					Self:    true,
				}},
			}, nil
		},
	})
	err := r.CheckDatabaseState()
	c.Assert(err, jc.ErrorIsNil)
}

type fakeDatabase struct {
	testing.Stub
	replicaSetF func() (core.ReplicaSet, error)
	closeF      func() error
}

func (db *fakeDatabase) ReplicaSet() (core.ReplicaSet, error) {
	db.Stub.MethodCall(db, "ReplicaSet")
	return db.replicaSetF()
}

func (db *fakeDatabase) Close() error {
	db.Stub.MethodCall(db, "Close")
	return db.closeF()
}
