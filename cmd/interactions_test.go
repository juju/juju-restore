// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd_test

import (
	"strings"

	corecmd "github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju-restore/cmd"
)

type InteractionsSuite struct {
	testing.IsolationSuite

	ctx *corecmd.Context
}

var _ = gc.Suite(&InteractionsSuite{})

func (s *InteractionsSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.ctx = cmdtesting.Context(c)
	s.ctx.Stdin = strings.NewReader("\n")
}

func (s *InteractionsSuite) TestUserConfirmEnter(c *gc.C) {
	c.Assert(cmd.NewUserInteractions(s.ctx).UserConfirmYes(), jc.Satisfies, cmd.IsUserAbortedError)
	c.Assert(cmdtesting.Stderr(s.ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stdout(s.ctx), gc.Equals, "")
}

type kaboomReader struct{}

func (r kaboomReader) Read(p []byte) (n int, err error) {
	return 0, errors.Errorf("kaboom")
}

func (s *InteractionsSuite) TestUserConfirmFail(c *gc.C) {
	s.ctx.Stdin = kaboomReader{}
	c.Assert(cmd.NewUserInteractions(s.ctx).UserConfirmYes(), gc.ErrorMatches, "kaboom")
	c.Assert(cmdtesting.Stderr(s.ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stdout(s.ctx), gc.Equals, "")
}

func (s *InteractionsSuite) TestUserConfirmInvalid(c *gc.C) {
	s.ctx.Stdin = strings.NewReader("foo\nbar bazz\ny\n")
	c.Assert(cmd.NewUserInteractions(s.ctx).UserConfirmYes(), jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(s.ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stdout(s.ctx), gc.Equals, `Invalid response "foo". Please answer (y/N): Invalid response "bar bazz". Please answer (y/N): `)
}

func (s *InteractionsSuite) TestUserConfirmExplicitNo(c *gc.C) {
	for _, input := range []string{"n\n", "N\n", "no\n", "NO\n"} {
		s.ctx.Stdin = strings.NewReader(input)
		c.Assert(cmd.NewUserInteractions(s.ctx).UserConfirmYes(), jc.Satisfies, cmd.IsUserAbortedError)
		c.Assert(cmdtesting.Stderr(s.ctx), gc.Equals, "")
		c.Assert(cmdtesting.Stdout(s.ctx), gc.Equals, "")
	}
}

func (s *InteractionsSuite) TestUserConfirmExplicitYes(c *gc.C) {
	for _, input := range []string{"y\n", "Y\n", "yes\n", "YES\n"} {
		s.ctx.Stdin = strings.NewReader(input)
		c.Assert(cmd.NewUserInteractions(s.ctx).UserConfirmYes(), jc.ErrorIsNil)
		c.Assert(cmdtesting.Stderr(s.ctx), gc.Equals, "")
		c.Assert(cmdtesting.Stdout(s.ctx), gc.Equals, "")
	}
}

func (s *InteractionsSuite) TestConfirmMultiple(c *gc.C) {
	s.ctx.Stdin = strings.NewReader("y\ny\ny\n")
	ui := cmd.NewUserInteractions(s.ctx)
	for i := 0; i < 3; i++ {
		c.Assert(ui.UserConfirmYes(), jc.ErrorIsNil)
		c.Assert(cmdtesting.Stderr(s.ctx), gc.Equals, "")
		c.Assert(cmdtesting.Stdout(s.ctx), gc.Equals, "")
	}
}

func (s *InteractionsSuite) TestNotify(c *gc.C) {
	cmd.NewUserInteractions(s.ctx).Notify("must be fun to be on stdout")
	c.Assert(cmdtesting.Stderr(s.ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stdout(s.ctx), gc.Equals, "must be fun to be on stdout")
}
