// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd_test

import (
	"bytes"
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

	ctx   *corecmd.Context
	stdin bytes.Buffer
	ui    *cmd.UserInteractions
}

var _ = gc.Suite(&InteractionsSuite{})

func (s *InteractionsSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.ctx = cmdtesting.Context(c)
	s.ctx.Stdin = &s.stdin
	s.ui = cmd.NewUserInteractions(s.ctx)
	s.PatchValue(&cmd.ReadOneChar, func(cmd.UserInteractions) (string, error) { return "\r", nil })
}

func (s *InteractionsSuite) TestUserConfirmEnter(c *gc.C) {
	c.Assert(s.ui.UserConfirmYes(), jc.Satisfies, cmd.IsUserAbortedError)
	c.Assert(cmdtesting.Stderr(s.ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stdout(s.ctx), gc.Equals, "")
}

func (s *InteractionsSuite) TestUserConfirmFail(c *gc.C) {
	s.PatchValue(&cmd.ReadOneChar, func(cmd.UserInteractions) (string, error) { return "", errors.New("kaboom") })
	c.Assert(s.ui.UserConfirmYes(), gc.ErrorMatches, "kaboom")
	c.Assert(cmdtesting.Stderr(s.ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stdout(s.ctx), gc.Equals, "")
}

func (s *InteractionsSuite) TestUserConfirmInvalid(c *gc.C) {
	count := 1
	s.PatchValue(&cmd.ReadOneChar, func(cmd.UserInteractions) (string, error) {
		if count > 2 {
			return "y", nil
		}
		count++
		return "s", nil
	})
	c.Assert(s.ui.UserConfirmYes(), jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(s.ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stdout(s.ctx), gc.Equals, `Invalid answer "s". Please answer (y/N) or Enter to default to N: Invalid answer "s". Please answer (y/N) or Enter to default to N: `)
}

func (s *InteractionsSuite) TestUserConfirmExplicitNo(c *gc.C) {
	s.PatchValue(&cmd.ReadOneChar, func(cmd.UserInteractions) (string, error) { return "n", nil })
	c.Assert(s.ui.UserConfirmYes(), jc.Satisfies, cmd.IsUserAbortedError)
	c.Assert(cmdtesting.Stderr(s.ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stdout(s.ctx), gc.Equals, "")

	s.PatchValue(&cmd.ReadOneChar, func(cmd.UserInteractions) (string, error) { return "N", nil })
	c.Assert(s.ui.UserConfirmYes(), jc.Satisfies, cmd.IsUserAbortedError)
	c.Assert(cmdtesting.Stderr(s.ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stdout(s.ctx), gc.Equals, "")
}

func (s *InteractionsSuite) TestUserConfirmExplicitYes(c *gc.C) {
	s.PatchValue(&cmd.ReadOneChar, func(cmd.UserInteractions) (string, error) { return "y", nil })
	c.Assert(s.ui.UserConfirmYes(), jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(s.ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stdout(s.ctx), gc.Equals, "")

	s.PatchValue(&cmd.ReadOneChar, func(cmd.UserInteractions) (string, error) { return "Y", nil })
	c.Assert(s.ui.UserConfirmYes(), jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(s.ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stdout(s.ctx), gc.Equals, "")
}

func (s *InteractionsSuite) TestNotify(c *gc.C) {
	s.ui.Notify("must be fun to be on stdout")
	c.Assert(cmdtesting.Stderr(s.ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stdout(s.ctx), gc.Equals, "must be fun to be on stdout")
}
