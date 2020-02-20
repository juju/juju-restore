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
)

type InteractionsSuite struct {
	testing.IsolationSuite

	ctx      *corecmd.Context
	readFunc func(ctx *corecmd.Context) (string, error)
}

var _ = gc.Suite(&InteractionsSuite{})

func (s *InteractionsSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.ctx = cmdtesting.Context(c)
	s.readFunc = func(*corecmd.Context) (string, error) { return "\r", nil }
}

func (s *InteractionsSuite) TestUserConfirmEnter(c *gc.C) {
	c.Assert(cmd.NewUserInteractions(s.ctx, s.readFunc).UserConfirmYes(), jc.Satisfies, cmd.IsUserAbortedError)
	c.Assert(cmdtesting.Stderr(s.ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stdout(s.ctx), gc.Equals, "")
}

func (s *InteractionsSuite) TestUserConfirmFail(c *gc.C) {
	s.readFunc = func(*corecmd.Context) (string, error) { return "", errors.New("kaboom") }
	c.Assert(cmd.NewUserInteractions(s.ctx, s.readFunc).UserConfirmYes(), gc.ErrorMatches, "kaboom")
	c.Assert(cmdtesting.Stderr(s.ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stdout(s.ctx), gc.Equals, "")
}

func (s *InteractionsSuite) TestUserConfirmInvalid(c *gc.C) {
	count := 1
	s.readFunc = func(*corecmd.Context) (string, error) {
		if count > 2 {
			return "y", nil
		}
		count++
		return "s", nil
	}
	c.Assert(cmd.NewUserInteractions(s.ctx, s.readFunc).UserConfirmYes(), jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(s.ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stdout(s.ctx), gc.Equals, `Invalid answer "s". Please answer (y/N) or Enter to default to N: Invalid answer "s". Please answer (y/N) or Enter to default to N: `)
}

func (s *InteractionsSuite) TestUserConfirmExplicitNo(c *gc.C) {
	s.readFunc = func(*corecmd.Context) (string, error) { return "n", nil }
	c.Assert(cmd.NewUserInteractions(s.ctx, s.readFunc).UserConfirmYes(), jc.Satisfies, cmd.IsUserAbortedError)
	c.Assert(cmdtesting.Stderr(s.ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stdout(s.ctx), gc.Equals, "")

	s.readFunc = func(*corecmd.Context) (string, error) { return "N", nil }
	c.Assert(cmd.NewUserInteractions(s.ctx, s.readFunc).UserConfirmYes(), jc.Satisfies, cmd.IsUserAbortedError)
	c.Assert(cmdtesting.Stderr(s.ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stdout(s.ctx), gc.Equals, "")
}

func (s *InteractionsSuite) TestUserConfirmExplicitYes(c *gc.C) {
	s.readFunc = func(*corecmd.Context) (string, error) { return "y", nil }
	c.Assert(cmd.NewUserInteractions(s.ctx, s.readFunc).UserConfirmYes(), jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(s.ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stdout(s.ctx), gc.Equals, "")

	s.readFunc = func(*corecmd.Context) (string, error) { return "Y", nil }
	c.Assert(cmd.NewUserInteractions(s.ctx, s.readFunc).UserConfirmYes(), jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(s.ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stdout(s.ctx), gc.Equals, "")
}

func (s *InteractionsSuite) TestNotify(c *gc.C) {
	cmd.NewUserInteractions(s.ctx, s.readFunc).Notify("must be fun to be on stdout")
	c.Assert(cmdtesting.Stderr(s.ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stdout(s.ctx), gc.Equals, "must be fun to be on stdout")
}
