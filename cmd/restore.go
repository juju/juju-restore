// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/loggo"

	"github.com/juju/juju-restore/core"
	"github.com/juju/juju-restore/db"
)

var logger = loggo.GetLogger("juju-restore.cmd")

const (
	defaultLogConfig = "<root>=INFO"
	verboseLogConfig = "<root>=DEBUG"
)

// NewRestoreCommand creates a cmd.Command to check the database and
// restore the Juju backup.
func NewRestoreCommand(dbConnect func(info db.DialInfo) (core.Database, error)) cmd.Command {
	command := &restoreCommand{}
	command.connect = dbConnect
	return command
}

type restoreCommand struct {
	cmd.CommandBase

	hostname string
	port     string
	ssl      bool
	username string
	password string

	verbose       bool
	loggingConfig string
	backupFile    string

	connect func(info db.DialInfo) (core.Database, error)
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
	f.StringVar(&c.loggingConfig, "logging-config", defaultLogConfig, "set logging levels")
	f.BoolVar(&c.verbose, "verbose", false, "more output from restore (debug logging)")
}

// Init is part of cmd.Command.
func (c *restoreCommand) Init(args []string) error {
	if len(args) == 0 {
		return errors.New("missing backup file")
	}
	c.backupFile, args = args[0], args[1:]
	if c.verbose && c.loggingConfig != defaultLogConfig {
		return errors.New("verbose and logging-config conflict - use one or the other")
	}
	if c.verbose {
		c.loggingConfig = verboseLogConfig
	}
	return c.CommandBase.Init(args)
}

// Run is part of cmd.Command.
func (c *restoreCommand) Run(ctx *cmd.Context) error {
	err := loggo.ConfigureLoggers(c.loggingConfig)
	if err != nil {
		return errors.Trace(err)
	}
	database, err := c.connect(db.DialInfo{
		Hostname: c.hostname,
		Port:     c.port,
		Username: c.username,
		Password: c.password,
		SSL:      c.ssl,
	})
	if err != nil {
		return errors.Trace(err)
	}
	defer database.Close()

	restorer := core.NewRestorer(database)

	// Pre-checks
	if err := restorer.CheckDatabaseState(); err != nil {
		return errors.Trace(err)
	}

	Notify(ctx, prechecksCompleted(&core.PrecheckResult{}))
	if err := UserConfirmYes(ctx); err != nil {
		return errors.Annotate(err, "restore operation")
	}

	// Actual restorations
	// Post-checks

	return nil
}
