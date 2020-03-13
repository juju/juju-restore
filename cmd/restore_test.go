// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd_test

import (
	corecmd "github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju-restore/cmd"
	"github.com/juju/juju-restore/core"
	"github.com/juju/juju-restore/db"
	"github.com/juju/juju-restore/machine"
)

type restoreSuite struct {
	testing.IsolationSuite

	database  *testDatabase
	backup    *fakeBackup
	connectF  func(db.DialInfo) (core.Database, error)
	openF     func(string, string) (core.BackupFile, error)
	converter func(member core.ReplicaSetMember) core.ControllerNode
	readFunc  func(*corecmd.Context) (string, error)
}

var _ = gc.Suite(&restoreSuite{})

func (s *restoreSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.database = &testDatabase{
		Stub: &testing.Stub{},
		replicaSetF: func() (core.ReplicaSet, error) {
			return core.ReplicaSet{
				Members: []core.ReplicaSetMember{
					{
						Healthy:       true,
						ID:            1,
						Name:          "one-node",
						State:         "PRIMARY",
						Self:          true,
						JujuMachineID: "2",
					},
				},
			}, nil
		},
	}
	s.backup = &fakeBackup{}
	s.connectF = func(db.DialInfo) (core.Database, error) { return s.database, nil }
	s.openF = func(string, string) (core.BackupFile, error) { return s.backup, nil }
	s.converter = machine.ControllerNodeForReplicaSetMember
	s.readFunc = func(*corecmd.Context) (string, error) { return "", nil }
}

type restoreCommandTestData struct {
	title    string
	args     []string
	errMatch string
}

var commandArgsTests = []restoreCommandTestData{
	{
		title:    "no args",
		args:     []string{},
		errMatch: "missing backup file",
	},
	{
		title: "just file",
		args:  []string{"backup.file"},
	},
	{
		title:    "verbose and logging-config conflict",
		args:     []string{"backup.file", "--logging-config", "<root>=TRACE", "--verbose"},
		errMatch: "verbose and logging-config conflict - use one or the other",
	},
}

func (s *restoreSuite) TestArgParsing(c *gc.C) {
	command := cmd.NewRestoreCommand(s.connectF, s.openF, s.converter, s.readFunc)
	for i, test := range commandArgsTests {
		c.Logf("%d: %s", i, test.title)
		err := cmdtesting.InitCommand(command, test.args)
		if test.errMatch == "" {
			c.Assert(err, jc.ErrorIsNil)
		} else {
			c.Assert(err, gc.ErrorMatches, test.errMatch)
		}
	}
}

func (s *restoreSuite) TestRestoreAborted(c *gc.C) {
	ctx, err := s.runCmd(c, "\n", "backup.file")
	c.Assert(err, gc.ErrorMatches, "restore operation: aborted")

	s.database.CheckCallNames(c, "ReplicaSet", "Close")
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, `
Connecting to database...
Checking database and replica set health...

Replica set is healthy     ✓
Running on primary HA node ✓

You are about to restore a controller from a backup file taken on 0001-01-01 00:00:00 +0000 UTC. 
It contains a controller  at Juju version 0.0.0 with 0 models.

All restore pre-checks are completed.

Restore cannot be cleanly aborted from here on.

Are you sure you want to proceed? (y/N): `[1:])
}

func (s *restoreSuite) TestRestoreProceed(c *gc.C) {
	s.converter = func(member core.ReplicaSetMember) core.ControllerNode {
		node := &fakeControllerNode{Stub: &testing.Stub{}, ip: member.Name}
		return node
	}
	ctx, err := s.runCmd(c, "y", "backup.file")
	c.Assert(err, jc.ErrorIsNil)

	s.database.CheckCallNames(c, "ReplicaSet", "Close")
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, `
Connecting to database...
Checking database and replica set health...

Replica set is healthy     ✓
Running on primary HA node ✓

You are about to restore a controller from a backup file taken on 0001-01-01 00:00:00 +0000 UTC. 
It contains a controller  at Juju version 0.0.0 with 0 models.

All restore pre-checks are completed.

Restore cannot be cleanly aborted from here on.

Are you sure you want to proceed? (y/N): 
Stopping Juju agents...
 
    one-node ✓ 
`[1:])
}

func (s *restoreSuite) setupHA() {
	s.database.replicaSetF = func() (core.ReplicaSet, error) {
		return core.ReplicaSet{
			Members: []core.ReplicaSetMember{
				{
					Healthy:       true,
					ID:            1,
					Name:          "one:node",
					State:         "PRIMARY",
					Self:          true,
					JujuMachineID: "2",
				},
				{
					Healthy:       true,
					ID:            2,
					Name:          "two:node",
					State:         "SECONDARY",
					JujuMachineID: "1",
				},
			},
		}, nil
	}
}

func (s *restoreSuite) TestRestoreHAConnectionFail(c *gc.C) {
	s.setupHA()
	s.converter = func(member core.ReplicaSetMember) core.ControllerNode {
		node := &fakeControllerNode{Stub: &testing.Stub{}, ip: member.Name}
		node.SetErrors(errors.New("kaboom"))
		return node
	}
	ctx, err := s.runCmd(c, "y\r\n", "backup.file")
	c.Assert(err, gc.ErrorMatches, `'juju-restore' could not connect to all controller machines: controllers' agents cannot be managed`)

	s.database.CheckCallNames(c, "ReplicaSet", "Close")
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, `
Connecting to database...
Checking database and replica set health...

Replica set is healthy     ✓
Running on primary HA node ✓

You are about to restore a controller from a backup file taken on 0001-01-01 00:00:00 +0000 UTC. 
It contains a controller  at Juju version 0.0.0 with 0 models.

This controller is in HA and to restore into it successfully, 
'juju-restore' needs to manage Juju and Mongo agents on  
secondary controller nodes.
However, on the bigger systems, the operator might want to manage 
these agents manually.

Do you want 'juju-restore' to manage these agents automatically? (y/N): 

Checking connectivity to secondary controller machines...
 
    two:node ✗ error: kaboom
`[1:])
}

func (s *restoreSuite) TestRestoreHAConnectionOk(c *gc.C) {
	s.setupHA()
	s.converter = func(member core.ReplicaSetMember) core.ControllerNode {
		return &fakeControllerNode{Stub: &testing.Stub{}, ip: member.Name}
	}
	ctx, err := s.runCmd(c, "y\r\n\n", "backup.file")
	c.Assert(err, gc.ErrorMatches, "restore operation: aborted")

	s.database.CheckCallNames(c, "ReplicaSet", "Close")
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, `
Connecting to database...
Checking database and replica set health...

Replica set is healthy     ✓
Running on primary HA node ✓

You are about to restore a controller from a backup file taken on 0001-01-01 00:00:00 +0000 UTC. 
It contains a controller  at Juju version 0.0.0 with 0 models.

This controller is in HA and to restore into it successfully, 
'juju-restore' needs to manage Juju and Mongo agents on  
secondary controller nodes.
However, on the bigger systems, the operator might want to manage 
these agents manually.

Do you want 'juju-restore' to manage these agents automatically? (y/N): 

Checking connectivity to secondary controller machines...
 
    two:node ✓ 

All restore pre-checks are completed.

Restore cannot be cleanly aborted from here on.

Are you sure you want to proceed? (y/N): `[1:])
}

func (s *restoreSuite) TestRestoreHAChoseManual(c *gc.C) {
	s.setupHA()
	ctx, err := s.runCmd(c, "\n\n", "backup.file")
	c.Assert(err, gc.ErrorMatches, "restore operation: aborted")

	s.database.CheckCallNames(c, "ReplicaSet", "Close")
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, `
Connecting to database...
Checking database and replica set health...

Replica set is healthy     ✓
Running on primary HA node ✓

You are about to restore a controller from a backup file taken on 0001-01-01 00:00:00 +0000 UTC. 
It contains a controller  at Juju version 0.0.0 with 0 models.

This controller is in HA and to restore into it successfully, 
'juju-restore' needs to manage Juju and Mongo agents on  
secondary controller nodes.
However, on the bigger systems, the operator might want to manage 
these agents manually.

Do you want 'juju-restore' to manage these agents automatically? (y/N): 
All restore pre-checks are completed.

Restore cannot be cleanly aborted from here on.

Are you sure you want to proceed? (y/N): `[1:])
}

func (s *restoreSuite) TestRestoreHAManualControlOption(c *gc.C) {
	s.setupHA()
	s.converter = func(member core.ReplicaSetMember) core.ControllerNode {
		node := &fakeControllerNode{Stub: &testing.Stub{}, ip: member.Name}
		return node
	}
	ctx, err := s.runCmd(c, "y\r\ny\r\n", "backup.file", "--manual-agent-control")
	c.Assert(err, jc.ErrorIsNil)
	s.database.CheckCallNames(c, "ReplicaSet", "Close")
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, `
Connecting to database...
Checking database and replica set health...

Replica set is healthy     ✓
Running on primary HA node ✓

You are about to restore a controller from a backup file taken on 0001-01-01 00:00:00 +0000 UTC. 
It contains a controller  at Juju version 0.0.0 with 0 models.

Juju agents on secondary controller machines must be stopped by this point.
To stop the agents, login into each secondary controller and run:
    $ systemctl stop jujud-machine-*

All restore pre-checks are completed.

Restore cannot be cleanly aborted from here on.

Are you sure you want to proceed? (y/N): 
Stopping Juju agents...
 
    one:node ✓ 
`[1:])
}

func (s *restoreSuite) TestRestoreAgentStopFail(c *gc.C) {
	s.setupHA()
	s.converter = func(member core.ReplicaSetMember) core.ControllerNode {
		node := &fakeControllerNode{Stub: &testing.Stub{}, ip: member.Name}
		node.SetErrors(errors.New("kaboom"))
		return node
	}
	ctx, err := s.runCmd(c, "y\r\ny\r\n", "backup.file", "--manual-agent-control")
	c.Assert(err, gc.ErrorMatches, "'juju-restore' could not manipulate all necessary agents: controllers' agents cannot be managed")
	s.database.CheckCallNames(c, "ReplicaSet", "Close")
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, `
Connecting to database...
Checking database and replica set health...

Replica set is healthy     ✓
Running on primary HA node ✓

You are about to restore a controller from a backup file taken on 0001-01-01 00:00:00 +0000 UTC. 
It contains a controller  at Juju version 0.0.0 with 0 models.

Juju agents on secondary controller machines must be stopped by this point.
To stop the agents, login into each secondary controller and run:
    $ systemctl stop jujud-machine-*

All restore pre-checks are completed.

Restore cannot be cleanly aborted from here on.

Are you sure you want to proceed? (y/N): 
Stopping Juju agents...
 
    one:node ✗ error: kaboom
`[1:])
}

func (s *restoreSuite) TestRestoreStartAgents(c *gc.C) {
	s.converter = func(member core.ReplicaSetMember) core.ControllerNode {
		node := &fakeControllerNode{Stub: &testing.Stub{}, ip: member.Name}
		return node
	}
	ctx, err := s.runCmd(c, "y", "backup.file", "--rs")
	c.Assert(err, jc.ErrorIsNil)

	s.database.CheckCallNames(c, "ReplicaSet", "ReplicaSet", "Close")
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, `
Connecting to database...
Checking database and replica set health...

Replica set is healthy     ✓
Running on primary HA node ✓

You are about to restore a controller from a backup file taken on 0001-01-01 00:00:00 +0000 UTC. 
It contains a controller  at Juju version 0.0.0 with 0 models.

All restore pre-checks are completed.

Restore cannot be cleanly aborted from here on.

Are you sure you want to proceed? (y/N): 
Stopping Juju agents...
 
    one-node ✓ 

Starting Juju agents...
 
    one-node ✓ 
`[1:])
}

func (s *restoreSuite) TestRestoreStartAgentsInHA(c *gc.C) {
	s.setupHA()
	s.converter = func(member core.ReplicaSetMember) core.ControllerNode {
		node := &fakeControllerNode{Stub: &testing.Stub{}, ip: member.Name}
		return node
	}
	ctx, err := s.runCmd(c, "yy", "backup.file", "--rs")
	c.Assert(err, jc.ErrorIsNil)

	s.database.CheckCallNames(c, "ReplicaSet", "ReplicaSet", "Close")
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, `
Connecting to database...
Checking database and replica set health...

Replica set is healthy     ✓
Running on primary HA node ✓

You are about to restore a controller from a backup file taken on 0001-01-01 00:00:00 +0000 UTC. 
It contains a controller  at Juju version 0.0.0 with 0 models.

This controller is in HA and to restore into it successfully, 
'juju-restore' needs to manage Juju and Mongo agents on  
secondary controller nodes.
However, on the bigger systems, the operator might want to manage 
these agents manually.

Do you want 'juju-restore' to manage these agents automatically? (y/N): 

Checking connectivity to secondary controller machines...
 
    two:node ✓ 

All restore pre-checks are completed.

Restore cannot be cleanly aborted from here on.

Are you sure you want to proceed? (y/N): 
Stopping Juju agents...
 
    one:node ✓  
    two:node ✓ 

Starting Juju agents...
 
    one:node ✓  
    two:node ✓ 
Primary node may have shifted.
`[1:])
}

func (s *restoreSuite) runCmd(c *gc.C, input string, args ...string) (*corecmd.Context, error) {
	count := -1
	s.readFunc = func(*corecmd.Context) (string, error) {
		count++
		return string(input[count]), nil
	}
	command := cmd.NewRestoreCommand(s.connectF, s.openF, s.converter, s.readFunc)
	err := cmdtesting.InitCommand(command, args)
	if err != nil {
		return nil, err
	}
	ctx := cmdtesting.Context(c)
	return ctx, command.Run(ctx)
}

type testDatabase struct {
	*testing.Stub
	replicaSetF     func() (core.ReplicaSet, error)
	controllerInfoF func() (core.ControllerInfo, error)
}

func (d *testDatabase) ReplicaSet() (core.ReplicaSet, error) {
	d.AddCall("ReplicaSet")
	return d.replicaSetF()
}

func (d *testDatabase) ControllerInfo() (core.ControllerInfo, error) {
	d.AddCall("ControllerInfo")
	return d.controllerInfoF()
}

func (d *testDatabase) Close() {
	d.AddCall("Close")
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

func (f *fakeControllerNode) StopAgent() error {
	f.Stub.MethodCall(f, "StopAgent")
	return f.NextErr()
}

func (f *fakeControllerNode) StartAgent() error {
	f.Stub.MethodCall(f, "StartAgent")
	return f.NextErr()
}

type fakeBackup struct {
	testing.Stub
	metadataF func() (core.BackupMetadata, error)
}

func (b *fakeBackup) Metadata() (core.BackupMetadata, error) {
	b.Stub.MethodCall(b, "Metadata")
	return b.metadataF()
}

func (b *fakeBackup) Close() error {
	b.Stub.MethodCall(b, "Close")
	return b.Stub.NextErr()
}
