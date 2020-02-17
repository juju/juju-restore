// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd

import (
	"github.com/juju/cmd"

	"github.com/juju/juju-restore/core"
)

func NewRestoreCommandForTest(connectF func() (core.Database, func(), error)) cmd.Command {
	return &restoreCommand{connectFunc: connectF}
}
