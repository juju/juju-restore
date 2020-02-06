// Copyright 2020 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package main

import (
	"os"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/loggo"

	"github.com/juju/juju-restore/core"
	"github.com/juju/juju-restore/db"
)

var logger = loggo.GetLogger("juju-restore")

const defaultLogConfig = "<root>=DEBUG"

var loggingConfig = defaultLogConfig

func main() {
	_, err := loggo.ReplaceDefaultWriter(cmd.NewWarningWriter(os.Stderr))
	if err != nil {
		panic(err)
	}
	os.Exit(Run(os.Args))
}

// Run creates and runs the restore command.
func Run(args []string) int {
	ctx, err := cmd.DefaultContext()
	if err != nil {
		logger.Errorf("%v", err)
		return 2
	}

	restorer := NewRestoreCommand(ctx)
	return cmd.Main(restorer, ctx, args[1:])
}

// NewRestoreCommand creates a cmd.Command to check the database and
// restore the Juju backup.
func NewRestoreCommand(ctx *cmd.Context) cmd.Command {
	return &restoreCommand{}
}

type restoreCommand struct {
	cmd.CommandBase

	hostname string
	port     string
	ssl      bool
	username string
	password string

	backupFile string
}

// Info is part of cmd.Command.
func (c *restoreCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "juju-restore",
		Args:    "<backup file>",
		Purpose: "Restore a Juju backup file into a specified controller",
		Doc:     restoreDoc,
	}
}

// SetFlags is part of cmd.Command.
func (c *restoreCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	f.StringVar(&c.hostname, "hostname", "localhost", "hostname of the Juju MongoDB server")
	f.StringVar(&c.port, "port", "37017", "port of the Juju MongoDB server")
	f.BoolVar(&c.ssl, "ssl", true, "use SSL to connect to MongoDB")
	f.StringVar(&c.username, "username", "admin", "user for connecting to MongoDB (\"\" for no authentication)")
	f.StringVar(&c.password, "password", "", "password for connecting to MongoDB")
	f.StringVar(&loggingConfig, "logging-config", defaultLogConfig, "set logging levels")
}

// Init is part of cmd.Command.
func (c *restoreCommand) Init(args []string) error {
	if len(args) == 0 {
		return errors.New("missing backup file")
	}
	c.backupFile, args = args[0], args[1:]
	return c.CommandBase.Init(args)
}

// Run is part of cmd.Command.
func (c *restoreCommand) Run(ctx *cmd.Context) error {
	database, err := db.Dial(db.DialInfo{
		Hostname: c.hostname,
		Port:     c.port,
		Username: c.username,
		Password: c.password,
		SSL:      c.ssl,
	})
	if err != nil {
		return errors.Trace(err)
	}
	defer func() {
		err := database.Close()
		if err != nil {
			logger.Errorf("error while closing database: %s", err)
		}
	}()

	restorer := core.NewRestorer(database)
	if err := restorer.CheckDatabaseState(); err != nil {
		return errors.Trace(err)
	}
	return nil
}

const restoreDoc = `

juju-restore must be executed on the MongoDB primary host of a Juju
controller.

The command will check the state of the target database and the
details of the backup file provided, and restore the contents of the
backup into the controller database.

`
