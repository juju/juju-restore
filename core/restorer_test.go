// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package core_test

import (
	"regexp"
	"time"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju-restore/core"
	"github.com/juju/juju-restore/machine"
)

type restorerSuite struct {
	testing.IsolationSuite
	converter func(member core.ReplicaSetMember) core.ControllerNode
}

var _ = gc.Suite(&restorerSuite{})

func (s *restorerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.converter = machine.ControllerNodeForReplicaSetMember
}

func (s *restorerSuite) TestCheckDatabaseStateUnhealthyMembers(c *gc.C) {
	r, err := core.NewRestorer(&fakeDatabase{
		replicaSetF: func() (core.ReplicaSet, error) {
			return core.ReplicaSet{
				Members: []core.ReplicaSetMember{{
					Healthy:       false,
					ID:            1,
					Name:          "kaira-ba",
					State:         "SECONDARY",
					JujuMachineID: "0",
				}, {
					Healthy:       true,
					ID:            2,
					Name:          "djula",
					State:         "PRIMARY",
					JujuMachineID: "1",
				}, {
					Healthy:       true,
					ID:            3,
					Name:          "bibi",
					State:         "OUCHY",
					JujuMachineID: "2",
				}},
			}, nil
		},
	}, &fakeBackup{}, s.converter)
	c.Assert(err, jc.ErrorIsNil)
	err = r.CheckDatabaseState()
	c.Assert(err, jc.Satisfies, core.IsUnhealthyMembersError)
	c.Assert(err, gc.ErrorMatches, regexp.QuoteMeta(`unhealthy replica set members: 1 "kaira-ba" (juju machine 0), 3 "bibi" (juju machine 2)`))
}

func (s *restorerSuite) TestCheckDatabaseStateNoPrimary(c *gc.C) {
	r, err := core.NewRestorer(&fakeDatabase{
		replicaSetF: func() (core.ReplicaSet, error) {
			return core.ReplicaSet{
				Members: []core.ReplicaSetMember{{
					Healthy:       true,
					ID:            1,
					Name:          "kaira-ba",
					State:         "SECONDARY",
					JujuMachineID: "2",
				}, {
					Healthy:       true,
					ID:            2,
					Name:          "djula",
					State:         "SECONDARY",
					JujuMachineID: "1",
				}, {
					Healthy:       true,
					ID:            3,
					Name:          "bibi",
					State:         "SECONDARY",
					JujuMachineID: "0",
				}},
			}, nil
		},
	}, &fakeBackup{}, s.converter)
	c.Assert(err, jc.ErrorIsNil)
	err = r.CheckDatabaseState()
	c.Assert(err, gc.ErrorMatches, "no primary found in replica set")
}

func (s *restorerSuite) TestCheckDatabaseStateNotPrimary(c *gc.C) {
	r, err := core.NewRestorer(&fakeDatabase{
		replicaSetF: func() (core.ReplicaSet, error) {
			return core.ReplicaSet{
				Members: []core.ReplicaSetMember{{
					Healthy:       true,
					ID:            1,
					Name:          "kaira-ba",
					State:         "SECONDARY",
					Self:          true,
					JujuMachineID: "1",
				}, {
					Healthy:       true,
					ID:            2,
					Name:          "djula",
					State:         "PRIMARY",
					JujuMachineID: "2",
				}, {
					Healthy:       true,
					ID:            3,
					Name:          "bibi",
					State:         "SECONDARY",
					JujuMachineID: "0",
				}},
			}, nil
		},
	}, &fakeBackup{}, s.converter)
	c.Assert(err, jc.ErrorIsNil)
	err = r.CheckDatabaseState()
	c.Assert(err, gc.ErrorMatches, regexp.QuoteMeta(`not running on primary replica set member, primary is 2 "djula" (juju machine 2)`))
}

func (s *restorerSuite) TestCheckDatabaseStateAllGood(c *gc.C) {
	r, err := core.NewRestorer(&fakeDatabase{
		replicaSetF: func() (core.ReplicaSet, error) {
			return core.ReplicaSet{
				Members: []core.ReplicaSetMember{{
					Healthy:       true,
					ID:            1,
					Name:          "kaira-ba",
					State:         "SECONDARY",
					JujuMachineID: "0",
				}, {
					Healthy:       true,
					ID:            2,
					Name:          "djula",
					State:         "PRIMARY",
					Self:          true,
					JujuMachineID: "1",
				}, {
					Healthy:       true,
					ID:            3,
					Name:          "bibi",
					State:         "SECONDARY",
					JujuMachineID: "2",
				}},
			}, nil
		},
	}, &fakeBackup{}, s.converter)
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
					Healthy:       true,
					ID:            2,
					Name:          "djula",
					State:         "PRIMARY",
					Self:          true,
					JujuMachineID: "2",
				}},
			}, nil
		},
	}, &fakeBackup{}, s.converter)
	c.Assert(err, jc.ErrorIsNil)
	err = r.CheckDatabaseState()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(r.IsHA(), jc.IsFalse)
}

func (s *restorerSuite) TestCheckDatabaseStateMissingJujuID(c *gc.C) {
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
	}, &fakeBackup{}, s.converter)
	c.Assert(err, jc.ErrorIsNil)
	err = r.CheckDatabaseState()
	c.Assert(err, gc.ErrorMatches, regexp.QuoteMeta(`unhealthy replica set members: 2 "djula" (juju machine )`))
}

func (s *restorerSuite) TestCheckSecondaryControllerNodesSkipsSelf(c *gc.C) {
	r, err := core.NewRestorer(&fakeDatabase{
		replicaSetF: func() (core.ReplicaSet, error) {
			return core.ReplicaSet{
				Members: []core.ReplicaSetMember{
					{
						Healthy:       true,
						ID:            2,
						Name:          "djula:wot",
						State:         "PRIMARY",
						Self:          true,
						JujuMachineID: "2",
					},
				},
			}, nil
		},
	}, &fakeBackup{}, s.converter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(r.CheckSecondaryControllerNodes(), gc.DeepEquals, map[string]error{})
}

func (s *restorerSuite) checkSecondaryControllerNodes(c *gc.C, expected map[string]error) {
	r, err := core.NewRestorer(&fakeDatabase{
		replicaSetF: func() (core.ReplicaSet, error) {
			return core.ReplicaSet{
				Members: []core.ReplicaSetMember{
					{
						Healthy:       true,
						ID:            2,
						Name:          "djula",
						State:         "PRIMARY",
						Self:          true,
						JujuMachineID: "2",
					},
					{
						Healthy:       true,
						ID:            1,
						Name:          "wot",
						State:         "SECONDARY",
						JujuMachineID: "1",
					},
				},
			}, nil
		},
	}, &fakeBackup{}, s.converter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(r.CheckSecondaryControllerNodes(), gc.DeepEquals, expected)
}

func (s *restorerSuite) TestCheckSecondaryControllerNodesOk(c *gc.C) {
	s.converter = func(member core.ReplicaSetMember) core.ControllerNode {
		return &fakeControllerNode{ip: member.Name}
	}
	s.checkSecondaryControllerNodes(c, map[string]error{"wot": nil})
}

func (s *restorerSuite) TestCheckSecondaryControllerNodesFail(c *gc.C) {
	err := errors.New("boom")
	s.converter = func(member core.ReplicaSetMember) core.ControllerNode {
		node := &fakeControllerNode{ip: member.Name}
		node.SetErrors(err)
		return node
	}
	s.checkSecondaryControllerNodes(c, map[string]error{"wot": err})
}

type agentMgmtTest struct {
	mgmtFunc    func(*core.Restorer, bool) map[string]error
	secondaries bool
	result      map[string]error
	nodeErrs    map[string]string
}

func (s *restorerSuite) checkManagedAgents(c *gc.C, t agentMgmtTest) []*fakeControllerNode {
	nodes := []*fakeControllerNode{}
	s.converter = func(member core.ReplicaSetMember) core.ControllerNode {
		node := &fakeControllerNode{ip: member.Name}
		nodes = append(nodes, node)
		if e := t.nodeErrs[member.Name]; e != "" {
			node.SetErrors(errors.New(e))
		}
		return node
	}

	r, err := core.NewRestorer(&fakeDatabase{
		replicaSetF: func() (core.ReplicaSet, error) {
			return core.ReplicaSet{
				Members: []core.ReplicaSetMember{
					{
						Healthy:       true,
						ID:            2,
						Name:          "djula",
						State:         "PRIMARY",
						Self:          true,
						JujuMachineID: "2",
					},
					{
						Healthy:       true,
						ID:            1,
						Name:          "wot",
						State:         "SECONDARY",
						JujuMachineID: "1",
					},
				},
			}, nil
		},
	}, &fakeBackup{}, s.converter)
	c.Assert(err, jc.ErrorIsNil)

	result := t.mgmtFunc(r, t.secondaries)
	c.Assert(len(result), gc.Equals, len(t.result))
	for k, v := range result {
		if v != nil {
			c.Assert(v, gc.ErrorMatches, t.result[k].Error())
		} else {
			c.Assert(v, jc.ErrorIsNil)
		}
	}
	return nodes
}

func (s *restorerSuite) TestStopAgentsWithSecondaries(c *gc.C) {
	nodes := s.checkManagedAgents(c, agentMgmtTest{
		func(r *core.Restorer, s bool) map[string]error { return r.StopAgents(s) },
		true,
		map[string]error{
			"wot":   nil,
			"djula": nil,
		},
		map[string]string{},
	})
	c.Assert(nodes, gc.HasLen, 2)
	for _, n := range nodes {
		n.CheckCallNames(c, "IP", "StopAgent")
	}
}

func (s *restorerSuite) TestStopAgentsNoSecondaries(c *gc.C) {
	nodes := s.checkManagedAgents(c, agentMgmtTest{
		func(r *core.Restorer, s bool) map[string]error { return r.StopAgents(s) },
		false,
		map[string]error{
			"djula": nil,
		},
		map[string]string{},
	})
	c.Assert(nodes, gc.HasLen, 2)
	for _, n := range nodes {
		// When no secondaries are requested, only primary node will be run
		if n.IP() == "djula" {
			n.CheckCallNames(c, "IP", "StopAgent", "IP")
		} else {
			n.CheckCallNames(c, "IP")
		}
	}
}

func (s *restorerSuite) TestStopAgentFail(c *gc.C) {
	s.checkManagedAgents(c, agentMgmtTest{
		func(r *core.Restorer, s bool) map[string]error { return r.StopAgents(s) },
		true,
		map[string]error{
			"djula": errors.New("kaboom"),
			"wot":   nil,
		},
		map[string]string{"djula": "kaboom"},
	})
}

func (s *restorerSuite) TestStartAgentsWithSecondaries(c *gc.C) {
	nodes := s.checkManagedAgents(c, agentMgmtTest{
		func(r *core.Restorer, s bool) map[string]error { return r.StartAgents(s) },
		true,
		map[string]error{
			"wot":   nil,
			"djula": nil,
		},
		map[string]string{},
	})
	c.Assert(nodes, gc.HasLen, 2)
	for _, n := range nodes {
		n.CheckCallNames(c, "IP", "StartAgent")
	}
}

func (s *restorerSuite) TestStartAgentsNoSecondaries(c *gc.C) {
	nodes := s.checkManagedAgents(c, agentMgmtTest{
		func(r *core.Restorer, s bool) map[string]error { return r.StartAgents(s) },
		false,
		map[string]error{
			"djula": nil,
		},
		map[string]string{},
	})
	c.Assert(nodes, gc.HasLen, 2)
	for _, n := range nodes {
		// When no secondaries are requested, only primary node will be run
		if n.IP() == "djula" {
			n.CheckCallNames(c, "IP", "StartAgent", "IP")
		} else {
			n.CheckCallNames(c, "IP")
		}
	}
}

func (s *restorerSuite) TestStartAgentFail(c *gc.C) {
	s.checkManagedAgents(c, agentMgmtTest{
		func(r *core.Restorer, s bool) map[string]error { return r.StartAgents(s) },
		true,
		map[string]error{
			"wot":   errors.New("kaboom"),
			"djula": nil,
		},
		map[string]string{"wot": "kaboom"},
	})
}

func (s *restorerSuite) TestCheckRestorable(c *gc.C) {
	created, err := time.Parse(time.RFC3339, "2020-03-17T12:24:30Z")
	c.Assert(err, jc.ErrorIsNil)
	r, err := core.NewRestorer(&fakeDatabase{
		replicaSetF: func() (core.ReplicaSet, error) {
			return core.ReplicaSet{}, nil
		},
		controllerInfoF: func() (core.ControllerInfo, error) {
			return core.ControllerInfo{
				ControllerModelUUID: "alex the astronaut",
				JujuVersion:         version.MustParse("2.8-beta5.6"),
				HANodes:             5,
				Series:              "eoan",
			}, nil
		},
	}, &fakeBackup{
		metadataF: func() (core.BackupMetadata, error) {
			return core.BackupMetadata{
				ControllerModelUUID: "alex the astronaut",
				JujuVersion:         version.MustParse("2.8-beta5.3"),
				Series:              "eoan",
				BackupCreated:       created,
				ModelCount:          3,
				HANodes:             5,
			}, nil
		},
	}, nil)
	c.Assert(err, jc.ErrorIsNil)

	result, err := r.CheckRestorable(false)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(result, gc.DeepEquals, &core.PrecheckResult{
		BackupDate:            created,
		ControllerModelUUID:   "alex the astronaut",
		BackupJujuVersion:     version.MustParse("2.8-beta5.3"),
		ControllerJujuVersion: version.MustParse("2.8-beta5.6"),
		ModelCount:            3,
	})
}

func (s *restorerSuite) TestCheckRestorableAllowDowngrade(c *gc.C) {
	created, err := time.Parse(time.RFC3339, "2020-03-17T12:24:30Z")
	c.Assert(err, jc.ErrorIsNil)
	r, err := core.NewRestorer(&fakeDatabase{
		replicaSetF: func() (core.ReplicaSet, error) {
			return core.ReplicaSet{}, nil
		},
		controllerInfoF: func() (core.ControllerInfo, error) {
			return core.ControllerInfo{
				ControllerModelUUID: "alex the astronaut",
				JujuVersion:         version.MustParse("2.8-beta5.6"),
				HANodes:             5,
				Series:              "eoan",
			}, nil
		},
	}, &fakeBackup{
		metadataF: func() (core.BackupMetadata, error) {
			return core.BackupMetadata{
				ControllerModelUUID: "alex the astronaut",
				JujuVersion:         version.MustParse("2.7.6.3"),
				Series:              "eoan",
				BackupCreated:       created,
				ModelCount:          3,
				HANodes:             5,
			}, nil
		},
	}, nil)
	c.Assert(err, jc.ErrorIsNil)

	result, err := r.CheckRestorable(true)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(result, gc.DeepEquals, &core.PrecheckResult{
		BackupDate:            created,
		ControllerModelUUID:   "alex the astronaut",
		BackupJujuVersion:     version.MustParse("2.7.6.3"),
		ControllerJujuVersion: version.MustParse("2.8-beta5.6"),
		ModelCount:            3,
	})
}

func (s *restorerSuite) TestCheckRestorableWithAllowDowngradeButUpgrading(c *gc.C) {
	created, err := time.Parse(time.RFC3339, "2020-03-17T12:24:30Z")
	c.Assert(err, jc.ErrorIsNil)

	r, err := core.NewRestorer(&fakeDatabase{
		replicaSetF: func() (core.ReplicaSet, error) {
			return core.ReplicaSet{}, nil
		},
		controllerInfoF: func() (core.ControllerInfo, error) {
			return core.ControllerInfo{
				ControllerModelUUID: "porridge radio",
				JujuVersion:         version.MustParse("2.7.6"),
				HANodes:             5,
				Series:              "eoan",
			}, nil
		},
	}, &fakeBackup{
		metadataF: func() (core.BackupMetadata, error) {
			return core.BackupMetadata{
				ControllerModelUUID: "porridge radio",
				JujuVersion:         version.MustParse("2.8-beta5.3"),
				Series:              "eoan",
				BackupCreated:       created,
				ModelCount:          3,
				HANodes:             5,
			}, nil
		},
	}, nil)
	c.Assert(err, jc.ErrorIsNil)

	result, err := r.CheckRestorable(true)
	c.Assert(err, gc.ErrorMatches, `backup juju version "2.8-beta5.3" is greater than controller version "2.7.6"`)
	c.Assert(result, gc.IsNil)
}

func (s *restorerSuite) checkRestorableMismatch(c *gc.C, expectErr string, tweak func(*core.ControllerInfo)) {
	created, err := time.Parse(time.RFC3339, "2020-03-17T12:24:30Z")
	c.Assert(err, jc.ErrorIsNil)

	controllerInfo := core.ControllerInfo{
		ControllerModelUUID: "porridge radio",
		JujuVersion:         version.MustParse("2.8-beta5.6"),
		HANodes:             5,
		Series:              "eoan",
	}
	tweak(&controllerInfo)

	r, err := core.NewRestorer(&fakeDatabase{
		replicaSetF: func() (core.ReplicaSet, error) {
			return core.ReplicaSet{}, nil
		},
		controllerInfoF: func() (core.ControllerInfo, error) {
			return controllerInfo, nil
		},
	}, &fakeBackup{
		metadataF: func() (core.BackupMetadata, error) {
			return core.BackupMetadata{
				ControllerModelUUID: "porridge radio",
				JujuVersion:         version.MustParse("2.8-beta5.3"),
				Series:              "eoan",
				BackupCreated:       created,
				ModelCount:          3,
				HANodes:             5,
			}, nil
		},
	}, nil)
	c.Assert(err, jc.ErrorIsNil)

	result, err := r.CheckRestorable(false)
	c.Assert(err, gc.ErrorMatches, expectErr)
	c.Assert(result, gc.IsNil)
}

func (s *restorerSuite) TestCheckRestorableMismatchController(c *gc.C) {
	s.checkRestorableMismatch(c, `controller model uuids don't match - backup: "porridge radio", controller: "alex the astronaut"`,
		func(i *core.ControllerInfo) {
			i.ControllerModelUUID = "alex the astronaut"
		},
	)
}

func (s *restorerSuite) TestCheckRestorableMismatchJujuVersion(c *gc.C) {
	s.checkRestorableMismatch(c, `juju versions don't match - backup: "2.8-beta5.3", controller: "2.7.5"`,
		func(i *core.ControllerInfo) {
			i.JujuVersion = version.MustParse("2.7.5")
		},
	)
}

func (s *restorerSuite) TestCheckRestorableMismatchHANodes(c *gc.C) {
	s.checkRestorableMismatch(c, `controller HA node counts don't match - backup: 5, controller: 3`,
		func(i *core.ControllerInfo) {
			i.HANodes = 3
		},
	)
}

func (s *restorerSuite) TestCheckRestorableMismatchSeries(c *gc.C) {
	s.checkRestorableMismatch(c, `controller series don't match - backup: "eoan", controller: "zesty"`,
		func(i *core.ControllerInfo) {
			i.Series = "zesty"
		},
	)
}

func (s *restorerSuite) TestRestoreSameVersion(c *gc.C) {
	db := fakeDatabase{
		replicaSetF: func() (core.ReplicaSet, error) {
			return core.ReplicaSet{
				Members: []core.ReplicaSetMember{
					{
						Healthy:       true,
						ID:            2,
						Name:          "djula",
						State:         "PRIMARY",
						Self:          true,
						JujuMachineID: "2",
					},
				},
			}, nil
		},
		controllerInfoF: func() (core.ControllerInfo, error) {
			return core.ControllerInfo{
				JujuVersion: version.MustParse("2.7.6"),
			}, nil
		},
	}
	r, err := core.NewRestorer(
		&db,
		&fakeBackup{
			dumpDirF: func() string {
				return "the dump dir!"
			},
			metadataF: func() (core.BackupMetadata, error) {
				return core.BackupMetadata{
					JujuVersion: version.MustParse("2.7.6"),
				}, nil
			},
		},
		s.converter,
	)
	c.Assert(err, jc.ErrorIsNil)
	db.SetErrors(errors.Errorf("bad!"))
	err = r.Restore("log path", true)
	c.Assert(err, gc.ErrorMatches, `restoring dump from "the dump dir!": bad!`)

	c.Assert(db.Calls(), gc.HasLen, 3)
	db.CheckCall(c, 2, "RestoreFromDump", "the dump dir!", "log path", true)
}

func (s *restorerSuite) TestRestoreDowngrade(c *gc.C) {
	machines := []fakeControllerNode{
		{ip: "1.1.1.1"},
		{ip: "1.1.1.2"},
	}
	convertToMachine := func(member core.ReplicaSetMember) core.ControllerNode {
		return &machines[member.ID]
	}
	db := fakeDatabase{
		replicaSetF: func() (core.ReplicaSet, error) {
			return core.ReplicaSet{
				Members: []core.ReplicaSetMember{{
					Healthy:       true,
					ID:            0,
					Name:          "djula",
					State:         "PRIMARY",
					Self:          true,
					JujuMachineID: "2",
				}, {
					Healthy:       true,
					ID:            1,
					Name:          "cosmonauts",
					State:         "SECONDARY",
					Self:          false,
					JujuMachineID: "3",
				}},
			}, nil
		},
		controllerInfoF: func() (core.ControllerInfo, error) {
			return core.ControllerInfo{
				JujuVersion: version.MustParse("2.8-beta1"),
			}, nil
		},
	}
	r, err := core.NewRestorer(
		&db,
		&fakeBackup{
			dumpDirF: func() string {
				return "the dump dir!"
			},
			metadataF: func() (core.BackupMetadata, error) {
				return core.BackupMetadata{
					JujuVersion: version.MustParse("2.7.6"),
				}, nil
			},
		},
		convertToMachine,
	)
	c.Assert(err, jc.ErrorIsNil)
	err = r.Restore("log path", true)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(db.Calls(), gc.HasLen, 3)
	db.CheckCall(c, 2, "RestoreFromDump", "the dump dir!", "log path", true)

	for i, machine := range machines {
		c.Logf("machine %d", i)
		machine.CheckCallNames(c, "IP", "UpdateAgentVersion")
		machine.CheckCall(c, 1, "UpdateAgentVersion", version.MustParse("2.7.6"))
	}
}

func (s *restorerSuite) TestRestoreDowngradeError(c *gc.C) {
	machines := []fakeControllerNode{
		{ip: "1.1.1.1"},
		{ip: "1.1.1.2"},
	}
	convertToMachine := func(member core.ReplicaSetMember) core.ControllerNode {
		return &machines[member.ID]
	}
	db := fakeDatabase{
		replicaSetF: func() (core.ReplicaSet, error) {
			return core.ReplicaSet{
				Members: []core.ReplicaSetMember{{
					Healthy:       true,
					ID:            0,
					Name:          "djula",
					State:         "PRIMARY",
					Self:          true,
					JujuMachineID: "2",
				}, {
					Healthy:       true,
					ID:            1,
					Name:          "cosmonauts",
					State:         "SECONDARY",
					Self:          false,
					JujuMachineID: "3",
				}},
			}, nil
		},
		controllerInfoF: func() (core.ControllerInfo, error) {
			return core.ControllerInfo{
				JujuVersion: version.MustParse("2.8-beta1"),
			}, nil
		},
	}
	r, err := core.NewRestorer(
		&db,
		&fakeBackup{
			dumpDirF: func() string {
				return "the dump dir!"
			},
			metadataF: func() (core.BackupMetadata, error) {
				return core.BackupMetadata{
					JujuVersion: version.MustParse("2.7.6"),
				}, nil
			},
		},
		convertToMachine,
	)
	c.Assert(err, jc.ErrorIsNil)

	machines[0].SetErrors(errors.New("stuff went bad"))
	machines[1].SetErrors(errors.New("oopsy daisy"))

	err = r.Restore("log path", true)
	c.Assert(err, gc.ErrorMatches, `
problems updating controllers to version "2.7.6": updating node 1.1.1.1: stuff went bad
updating node 1.1.1.2: oopsy daisy`[1:])
}

type fakeDatabase struct {
	testing.Stub
	replicaSetF     func() (core.ReplicaSet, error)
	controllerInfoF func() (core.ControllerInfo, error)
}

func (db *fakeDatabase) ReplicaSet() (core.ReplicaSet, error) {
	db.Stub.MethodCall(db, "ReplicaSet")
	return db.replicaSetF()
}

func (db *fakeDatabase) ControllerInfo() (core.ControllerInfo, error) {
	db.Stub.MethodCall(db, "ControllerInfo")
	return db.controllerInfoF()
}

func (db *fakeDatabase) RestoreFromDump(dumpDir, logFile string, includeStatusHistory bool) error {
	db.Stub.MethodCall(db, "RestoreFromDump", dumpDir, logFile, includeStatusHistory)
	return db.Stub.NextErr()
}

func (db *fakeDatabase) Close() {
	db.Stub.MethodCall(db, "Close")
}

type fakeControllerNode struct {
	testing.Stub
	ip string
}

func (f *fakeControllerNode) String() string {
	return "node " + f.ip
}

func (f *fakeControllerNode) IP() string {
	f.Stub.MethodCall(f, "IP")
	return f.ip
}

func (f *fakeControllerNode) Ping() error {
	f.Stub.MethodCall(f, "Ping")
	return f.NextErr()
}

func (f *fakeControllerNode) StopAgent() error {
	f.Stub.MethodCall(f, "StopAgent")
	return f.NextErr()
}

func (f *fakeControllerNode) StartAgent() error {
	f.Stub.MethodCall(f, "StartAgent")
	return f.NextErr()
}

func (f *fakeControllerNode) UpdateAgentVersion(target version.Number) error {
	f.Stub.MethodCall(f, "UpdateAgentVersion", target)
	return f.NextErr()
}

type fakeBackup struct {
	testing.Stub
	metadataF func() (core.BackupMetadata, error)
	dumpDirF  func() string
}

func (b *fakeBackup) Metadata() (core.BackupMetadata, error) {
	b.Stub.MethodCall(b, "Metadata")
	return b.metadataF()
}

func (b *fakeBackup) DumpDirectory() string {
	b.Stub.MethodCall(b, "DumpDirectory")
	return b.dumpDirF()
}

func (b *fakeBackup) Close() error {
	b.Stub.MethodCall(b, "Close")
	return b.Stub.NextErr()
}
