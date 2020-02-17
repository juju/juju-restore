// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd_test

import (
	"strings"

	corecmd "github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju-restore/cmd"
	"github.com/juju/juju-restore/core"
)

type restoreSuite struct {
	testing.IsolationSuite

	command  corecmd.Command
	database *testDatabase
	connectF func() (core.Database, func(), error)
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
						Healthy: true,
						ID:      1,
						Name:    "one-node",
						State:   "PRIMARY",
						Self:    true,
					},
				},
			}, nil
		},
	}
	s.connectF = func() (core.Database, func(), error) { return s.database, func() {}, nil }

	s.command = cmd.NewRestoreCommandForTest(s.connectF)
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
	for i, test := range commandArgsTests {
		c.Logf("%d: %s", i, test.title)
		err := cmdtesting.InitCommand(s.command, test.args)
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

	s.database.CheckCallNames(c, "ReplicaSet")
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, `

Replica set is healthy     ✓
Running on primary HA node ✓

All restore pre-checks are completed.

You are about to restore a controller from a backup file taken on 0001-01-01 00:00:00 +0000 UTC. 
It contains a controller  at Juju version 0.0.0 with 0 models.

Restore cannot be cleanly aborted from here on.

Are you sure you want to proceed? (y/N): `[1:])
}

func (s *restoreSuite) TestRestoreProceed(c *gc.C) {
	ctx, err := s.runCmd(c, "y", "backup.file")
	c.Assert(err, jc.ErrorIsNil)

	s.database.CheckCallNames(c, "ReplicaSet")
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, `

Replica set is healthy     ✓
Running on primary HA node ✓

All restore pre-checks are completed.

You are about to restore a controller from a backup file taken on 0001-01-01 00:00:00 +0000 UTC. 
It contains a controller  at Juju version 0.0.0 with 0 models.

Restore cannot be cleanly aborted from here on.

Are you sure you want to proceed? (y/N): `[1:])
}

func (s *restoreSuite) runCmd(c *gc.C, input string, args ...string) (*corecmd.Context, error) {
	err := cmdtesting.InitCommand(s.command, args)
	if err != nil {
		return nil, err
	}
	ctx := cmdtesting.Context(c)
	stdin := strings.NewReader(input)
	ctx.Stdin = stdin
	return ctx, s.command.Run(ctx)
}

type testDatabase struct {
	*testing.Stub
	replicaSetF func() (core.ReplicaSet, error)
}

func (d *testDatabase) ReplicaSet() (core.ReplicaSet, error) {
	d.AddCall("ReplicaSet")
	return d.replicaSetF()
}

func (d *testDatabase) Close() error {
	d.AddCall("Close")
	return d.NextErr()
}
