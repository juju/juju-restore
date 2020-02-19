// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package core_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju-restore/core"
)

type restorerSuite struct {
	testing.IsolationSuite
	converter func(member core.ReplicaSetMember) core.ControllerNode
}

var _ = gc.Suite(&restorerSuite{})

func (s *restorerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.converter = core.ControllerNodeForReplicaSetMember
}

func (s *restorerSuite) TestCheckDatabaseStateUnhealthyMembers(c *gc.C) {
	r, err := core.NewRestorer(&fakeDatabase{
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
	}, s.converter)
	c.Assert(err, jc.ErrorIsNil)
	err = r.CheckDatabaseState()
	c.Assert(err, jc.Satisfies, core.IsUnhealthyMembersError)
	c.Assert(err, gc.ErrorMatches, `unhealthy replica set members: 1 "kaira-ba", 3 "bibi"`)
}

func (s *restorerSuite) TestCheckDatabaseStateNoPrimary(c *gc.C) {
	r, err := core.NewRestorer(&fakeDatabase{
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
	}, s.converter)
	c.Assert(err, jc.ErrorIsNil)
	err = r.CheckDatabaseState()
	c.Assert(err, gc.ErrorMatches, "no primary found in replica set")
}

func (s *restorerSuite) TestCheckDatabaseStateNotPrimary(c *gc.C) {
	r, err := core.NewRestorer(&fakeDatabase{
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
	}, s.converter)
	c.Assert(err, jc.ErrorIsNil)
	err = r.CheckDatabaseState()
	c.Assert(err, gc.ErrorMatches, `not running on primary replica set member, primary is 2 "djula"`)
}

func (s *restorerSuite) TestCheckDatabaseStateAllGood(c *gc.C) {
	r, err := core.NewRestorer(&fakeDatabase{
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
	}, s.converter)
	c.Assert(err, jc.ErrorIsNil)
	err = r.CheckDatabaseState()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(r.IsHA(), jc.IsTrue)
}

func (s *restorerSuite) TestCheckDatabaseStateOneMember(c *gc.C) {
	r, err := core.NewRestorer(&fakeDatabase{
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
	}, s.converter)
	c.Assert(err, jc.ErrorIsNil)
	err = r.CheckDatabaseState()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(r.IsHA(), jc.IsFalse)
}

func (s *restorerSuite) TestCheckSecondaryControllerNodesSkipsSelf(c *gc.C) {
	r, err := core.NewRestorer(&fakeDatabase{
		replicaSetF: func() (core.ReplicaSet, error) {
			return core.ReplicaSet{
				Members: []core.ReplicaSetMember{
					{
						Healthy: true,
						ID:      2,
						Name:    "djula:wot",
						State:   "PRIMARY",
						Self:    true,
					},
				},
			}, nil
		},
	}, s.converter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(r.CheckSecondaryControllerNodes(), gc.DeepEquals, map[string]error{})
}

func (s *restorerSuite) checkSecondaryControllerNodes(c *gc.C, expected map[string]error) {
	r, err := core.NewRestorer(&fakeDatabase{
		replicaSetF: func() (core.ReplicaSet, error) {
			return core.ReplicaSet{
				Members: []core.ReplicaSetMember{
					{
						Healthy: true,
						ID:      2,
						Name:    "djula",
						State:   "PRIMARY",
						Self:    true,
					},
					{
						Healthy: true,
						ID:      1,
						Name:    "wot",
						State:   "SECONDARY",
					},
				},
			}, nil
		},
	}, s.converter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(r.CheckSecondaryControllerNodes(), gc.DeepEquals, expected)
}

func (s *restorerSuite) TestCheckSecondaryControllerNodesOk(c *gc.C) {
	s.converter = func(member core.ReplicaSetMember) core.ControllerNode {
		return &fakeControllerNode{Stub: &testing.Stub{}, ip: member.Name}
	}
	s.checkSecondaryControllerNodes(c, map[string]error{"wot": nil})
}

func (s *restorerSuite) TestCheckSecondaryControllerNodesFail(c *gc.C) {
	err := errors.New("boom")
	s.converter = func(member core.ReplicaSetMember) core.ControllerNode {
		node := &fakeControllerNode{Stub: &testing.Stub{}, ip: member.Name}
		node.SetErrors(err)
		return node
	}
	s.checkSecondaryControllerNodes(c, map[string]error{"wot": err})
}

type fakeDatabase struct {
	testing.Stub
	replicaSetF func() (core.ReplicaSet, error)
}

func (db *fakeDatabase) ReplicaSet() (core.ReplicaSet, error) {
	db.Stub.MethodCall(db, "ReplicaSet")
	return db.replicaSetF()
}

func (db *fakeDatabase) Close() {
	db.Stub.MethodCall(db, "Close")
}

type fakeControllerNode struct {
	*testing.Stub
	ip string
}

func (f *fakeControllerNode) IP() string {
	f.Stub.MethodCall(f, "IP")
	return f.ip
}

func (f *fakeControllerNode) Ping() error {
	f.Stub.MethodCall(f, "Ping")
	return f.NextErr()
}
