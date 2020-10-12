// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd_test

import (
	"io/ioutil"
	"path/filepath"
	"strings"
	"time"

	corecmd "github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version"
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
	loadCreds func() (string, string, error)
	devMode   bool
}

var _ = gc.Suite(&restoreSuite{})

func (s *restoreSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.database = &testDatabase{
		Stub: &testing.Stub{},
		replicaSetF: func() (core.ReplicaSet, error) {
			return core.ReplicaSet{
				Members: []core.ReplicaSetMember{{
					Healthy:       true,
					ID:            1,
					Name:          "one-node",
					State:         "PRIMARY",
					Self:          true,
					JujuMachineID: "2",
				}},
			}, nil
		},
		controllerInfoF: func() (core.ControllerInfo, error) {
			return core.ControllerInfo{
				ControllerModelUUID: "how-bizarre",
				JujuVersion:         version.MustParse("2.7.5.2"),
				Series:              "disco",
				HANodes:             1,
			}, nil
		},
	}
	created, err := time.Parse(time.RFC3339, "2020-03-17T16:28:24Z")
	c.Assert(err, jc.ErrorIsNil)
	s.backup = &fakeBackup{
		metadataF: func() (core.BackupMetadata, error) {
			return core.BackupMetadata{
				FormatVersion:       1,
				ControllerModelUUID: "how-bizarre",
				JujuVersion:         version.MustParse("2.7.5"),
				Series:              "disco",
				BackupCreated:       created,
				Hostname:            "juju-123456-0",
				ContainsLogs:        true,
				ModelCount:          3,
				HANodes:             1,
			}, nil
		},
		dumpDirF: func() string {
			return "dump-directory"
		},
	}
	s.connectF = func(db.DialInfo) (core.Database, error) { return s.database, nil }
	s.openF = func(string, string) (core.BackupFile, error) { return s.backup, nil }
	s.converter = machine.ControllerNodeForReplicaSetMember
	s.loadCreds = func() (string, string, error) {
		return "", "", errors.Errorf("loading those creds")
	}

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
	command := cmd.NewRestoreCommand(
		s.connectF,
		s.openF,
		s.converter,
		s.loadCreds,
		s.devMode,
	)
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

	assertLastCallIsClose(c, s.database.Calls())
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, `
Connecting to database...
Checking database and replica set health...

Replica set is healthy     ✓
Running on primary HA node ✓

You are about to restore this backup:
    Created at:   2020-03-17 16:28:24 +0000 UTC
    Controller:   how-bizarre
    Juju version: 2.7.5
    Models:       3

All restore pre-checks are completed.

Restore cannot be cleanly aborted from here on.

Are you sure you want to proceed? (y/N): `[1:])
}

func (s *restoreSuite) TestPrecheckFailed(c *gc.C) {
	s.database.controllerInfoF = func() (core.ControllerInfo, error) {
		return core.ControllerInfo{
			ControllerModelUUID: "how-bizarre",
			JujuVersion:         version.MustParse("2.7.5"),
			HANodes:             1,
			Series:              "focal",
		}, nil
	}
	ctx, err := s.runCmd(c, "\n", "backup.file")
	c.Assert(err, gc.ErrorMatches, `precheck: controller series don't match - backup: "disco", controller: "focal"`)

	assertLastCallIsClose(c, s.database.Calls())
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, `
Connecting to database...
Checking database and replica set health...

Replica set is healthy     ✓
Running on primary HA node ✓
`[1:])
}

func (s *restoreSuite) TestRestoreProceed(c *gc.C) {
	s.converter = func(member core.ReplicaSetMember) core.ControllerNode {
		node := &fakeControllerNode{Stub: &testing.Stub{}, ip: member.Name}
		return node
	}
	ctx, err := s.runCmd(c, "y\n", "backup.file")
	c.Assert(err, jc.ErrorIsNil)

	assertLastCallIsClose(c, s.database.Calls())
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, `
Connecting to database...
Checking database and replica set health...

Replica set is healthy     ✓
Running on primary HA node ✓

You are about to restore this backup:
    Created at:   2020-03-17 16:28:24 +0000 UTC
    Controller:   how-bizarre
    Juju version: 2.7.5
    Models:       3

All restore pre-checks are completed.

Restore cannot be cleanly aborted from here on.

Are you sure you want to proceed? (y/N): 
Stopping Juju agents...
 
    one-node ✓ 

Running restore...
Detailed mongorestore output in restore.log.

Database restore complete.
Starting Juju agents...
 
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
	ctx, err := s.runCmd(c, "y\n", "backup.file")
	c.Assert(err, gc.ErrorMatches, `'juju-restore' could not connect to all controller machines: controllers' agents cannot be managed`)

	assertLastCallIsClose(c, s.database.Calls())
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, `
Connecting to database...
Checking database and replica set health...

Replica set is healthy     ✓
Running on primary HA node ✓

You are about to restore this backup:
    Created at:   2020-03-17 16:28:24 +0000 UTC
    Controller:   how-bizarre
    Juju version: 2.7.5
    Models:       3

This controller is in HA and to restore into it successfully, 'juju-restore' 
needs to manage Juju and Mongo agents on secondary controller nodes.
However on bigger systems the user might want to manage these agents manually.

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
	ctx, err := s.runCmd(c, "y\n\n", "backup.file")
	c.Assert(err, gc.ErrorMatches, "restore operation: aborted")

	assertLastCallIsClose(c, s.database.Calls())
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, `
Connecting to database...
Checking database and replica set health...

Replica set is healthy     ✓
Running on primary HA node ✓

You are about to restore this backup:
    Created at:   2020-03-17 16:28:24 +0000 UTC
    Controller:   how-bizarre
    Juju version: 2.7.5
    Models:       3

This controller is in HA and to restore into it successfully, 'juju-restore' 
needs to manage Juju and Mongo agents on secondary controller nodes.
However on bigger systems the user might want to manage these agents manually.

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

	assertLastCallIsClose(c, s.database.Calls())
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, `
Connecting to database...
Checking database and replica set health...

Replica set is healthy     ✓
Running on primary HA node ✓

You are about to restore this backup:
    Created at:   2020-03-17 16:28:24 +0000 UTC
    Controller:   how-bizarre
    Juju version: 2.7.5
    Models:       3

This controller is in HA and to restore into it successfully, 'juju-restore' 
needs to manage Juju and Mongo agents on secondary controller nodes.
However on bigger systems the user might want to manage these agents manually.

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
	ctx, err := s.runCmd(c, "y\ny\n", "backup.file", "--manual-agent-control")
	c.Assert(err, jc.ErrorIsNil)
	assertLastCallIsClose(c, s.database.Calls())
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, `
Connecting to database...
Checking database and replica set health...

Replica set is healthy     ✓
Running on primary HA node ✓

You are about to restore this backup:
    Created at:   2020-03-17 16:28:24 +0000 UTC
    Controller:   how-bizarre
    Juju version: 2.7.5
    Models:       3

Juju agents on secondary controller machines must be stopped by this point.
To stop the agents, login into each secondary controller and run:
    $ sudo systemctl stop jujud-machine-*

All restore pre-checks are completed.

Restore cannot be cleanly aborted from here on.

Are you sure you want to proceed? (y/N): 
Stopping Juju agents...
 
    one:node ✓ 

Running restore...
Detailed mongorestore output in restore.log.

Database restore complete.
Starting Juju agents...
 
    one:node ✓ 
Primary node may have shifted.
`[1:])
}

func (s *restoreSuite) TestRestoreAgentStopFail(c *gc.C) {
	s.setupHA()
	s.converter = func(member core.ReplicaSetMember) core.ControllerNode {
		node := &fakeControllerNode{Stub: &testing.Stub{}, ip: member.Name}
		node.SetErrors(errors.New("kaboom"))
		return node
	}
	ctx, err := s.runCmd(c, "y\ny\n", "backup.file", "--manual-agent-control")
	c.Assert(err, gc.ErrorMatches, "'juju-restore' could not manipulate all necessary agents: controllers' agents cannot be managed")
	assertLastCallIsClose(c, s.database.Calls())
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, `
Connecting to database...
Checking database and replica set health...

Replica set is healthy     ✓
Running on primary HA node ✓

You are about to restore this backup:
    Created at:   2020-03-17 16:28:24 +0000 UTC
    Controller:   how-bizarre
    Juju version: 2.7.5
    Models:       3

Juju agents on secondary controller machines must be stopped by this point.
To stop the agents, login into each secondary controller and run:
    $ sudo systemctl stop jujud-machine-*

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
	s.devMode = true
	ctx, err := s.runCmd(c, "y\n", "backup.file", "--rs")
	c.Assert(err, jc.ErrorIsNil)

	assertLastCallIsClose(c, s.database.Calls())
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, `
Connecting to database...

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
	s.devMode = true
	ctx, err := s.runCmd(c, "y\ny\n", "backup.file", "--rs")
	c.Assert(err, jc.ErrorIsNil)

	assertLastCallIsClose(c, s.database.Calls())
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, `
Connecting to database...

Starting Juju agents...
 
    one:node ✓  
    two:node ✓ 
Primary node may have shifted.
`[1:])
}

func (s *restoreSuite) TestLoadsCredsIfNoUsername(c *gc.C) {
	_, err := s.runCmdNoUser(c, "", "backup.file")
	c.Assert(err, gc.ErrorMatches, "loading credentials: loading those creds")
}

type readerFunc func(string) ([]byte, error)

func makeFakeReader(c *gc.C, expectedPath string, contents []byte) readerFunc {
	return func(path string) ([]byte, error) {
		c.Assert(path, gc.Equals, expectedPath)
		return contents, nil
	}
}

func (s *restoreSuite) TestReadCredsFromPattern(c *gc.C) {
	dir := c.MkDir()
	confPath := filepath.Join(dir, "agent.conf")
	err := ioutil.WriteFile(confPath, nil, 0777)
	c.Assert(err, jc.ErrorIsNil)

	username, password, err := cmd.ReadCredsFromPattern(
		filepath.Join(dir, "*.conf"),
		makeFakeReader(c, confPath, []byte(agentConfContents)),
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(username, gc.Equals, "porridge-radio")
	c.Assert(password, gc.Equals, "lilac")
}

func (s *restoreSuite) TestReadCredsMissingUsername(c *gc.C) {
	dir := c.MkDir()
	confPath := filepath.Join(dir, "agent.conf")
	err := ioutil.WriteFile(confPath, nil, 0777)
	c.Assert(err, jc.ErrorIsNil)

	_, _, err = cmd.ReadCredsFromPattern(
		filepath.Join(dir, "*.conf"),
		makeFakeReader(c, confPath, []byte(missingTagConf)),
	)
	c.Assert(err, gc.ErrorMatches, `no username found in ".*/agent\.conf" - tag field is missing or blank`)
}

func (s *restoreSuite) TestReadCredsMissingPassword(c *gc.C) {
	dir := c.MkDir()
	confPath := filepath.Join(dir, "agent.conf")
	err := ioutil.WriteFile(confPath, nil, 0777)
	c.Assert(err, jc.ErrorIsNil)

	_, _, err = cmd.ReadCredsFromPattern(
		filepath.Join(dir, "*.conf"),
		makeFakeReader(c, confPath, []byte(missingPasswordConf)),
	)
	c.Assert(err, gc.ErrorMatches, `no password found in ".*/agent\.conf" - statepassword field is missing or blank`)
}

var (
	agentConfContents = `
# format: 2.0
some-field:
  something: else
tag: porridge-radio
other: value
statepassword: lilac
`[1:]

	missingTagConf = `
# format: 2.0
some-field:
  something: else
other: value
statepassword: lilac
`[1:]

	missingPasswordConf = `
# format: 2.0
some-field:
  something: else
tag: porridge-radio
other: value
`[1:]
)

func (s *restoreSuite) runCmd(c *gc.C, input string, args ...string) (*corecmd.Context, error) {
	args = append([]string{"--username=admin"}, args...)
	return s.runCmdNoUser(c, input, args...)
}

func (s *restoreSuite) runCmdNoUser(c *gc.C, input string, args ...string) (*corecmd.Context, error) {
	command := cmd.NewRestoreCommand(s.connectF, s.openF, s.converter, s.loadCreds, s.devMode)
	err := cmdtesting.InitCommand(command, args)
	if err != nil {
		return nil, err
	}
	ctx := cmdtesting.Context(c)
	ctx.Stdin = strings.NewReader(input)
	return ctx, command.Run(ctx)
}

func assertLastCallIsClose(c *gc.C, calls []testing.StubCall) {
	if len(calls) == 0 {
		c.Fatalf("not closed because there were no calls")
	}
	c.Assert(calls[len(calls)-1].FuncName, gc.Equals, "Close")
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

func (d *testDatabase) RestoreFromDump(dumpDir, logFile string, includeStatusHistory bool) error {
	d.Stub.MethodCall(d, "RestoreFromDump", dumpDir, logFile, includeStatusHistory)
	return d.Stub.NextErr()
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
