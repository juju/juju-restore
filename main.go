// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"os"

	corecmd "github.com/juju/cmd"
	"github.com/juju/loggo"

	"github.com/juju/juju-restore/cmd"
	"github.com/juju/juju-restore/db"
)

var logger = loggo.GetLogger("juju-restore")

func main() {
	_, err := loggo.ReplaceDefaultWriter(NewColorWriter(os.Stderr))
	if err != nil {
		panic(err)
	}
	os.Exit(Run(os.Args))
}

// Run creates and runs the restore command.
func Run(args []string) int {
	ctx, err := corecmd.DefaultContext()
	if err != nil {
		logger.Errorf("%v", err)
		return 2
	}

	restorer := cmd.NewRestoreCommand(db.Dial)
	return corecmd.Main(restorer, ctx, args[1:])
}
