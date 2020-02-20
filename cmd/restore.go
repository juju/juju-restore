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
func NewRestoreCommand(
	dbConnect func(info db.DialInfo) (core.Database, error),
	machineConverter func(member core.ReplicaSetMember) core.ControllerNode,
	readFunc func(*cmd.Context) (string, error),
) cmd.Command {
	return &restoreCommand{
		connect:     dbConnect,
		converter:   machineConverter,
		readOneChar: readFunc,
	}
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

	// manualAgentControl determines if 'juju-restore' or the operator
	// manages - stops and starts juju and mongo agents - on
	// other, non-primary controller nodes.
	// If true, the control is manual and 'juju-restore' will do nothing
	// to other controller nodes.
	manualAgentControl bool

	ui          *UserInteractions
	restorer    *core.Restorer
	converter   func(member core.ReplicaSetMember) core.ControllerNode
	readOneChar func(*cmd.Context) (string, error)
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
	f.BoolVar(&c.manualAgentControl, "manual-agent-control", false, "operator stops/starts Juju and Mongo agents on secondary controller nodes in HA")
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

	restorer, err := core.NewRestorer(database, c.converter)
	if err != nil {
		return errors.Trace(err)
	}
	c.restorer = restorer
	c.ui = NewUserInteractions(ctx, c.readOneChar)

	// Pre-checks
	if err := c.runPreChecks(); err != nil {
		return errors.Trace(err)
	}
	// Actual restorations
	// Post-checks

	return nil
}

func (c *restoreCommand) runPreChecks() error {
	c.ui.Notify("Checking database and replica set health...\n")
	if err := c.restorer.CheckDatabaseState(); err != nil {
		return errors.Trace(err)
	}
	c.ui.Notify(dbHealthComplete)

	// TODO Backup file checks go here?
	c.ui.Notify(populate(backupFileTemplate, &core.PrecheckResult{}))

	if c.restorer.IsHA() {
		if !c.manualAgentControl {
			c.ui.Notify(releaseAgentsControl)
			if err := c.ui.UserConfirmYes(); err != nil {
				if !IsUserAbortedError(err) {
					return errors.Annotate(err, "releasing controller over agents")
				}
				c.manualAgentControl = true
			}
			if !c.manualAgentControl {
				c.ui.Notify("\n\nChecking connectivity to secondary controller machines...\n")
				connections := c.restorer.CheckSecondaryControllerNodes()
				c.ui.Notify(populate(nodeConnectivityTemplate, connections))
				for _, e := range connections {
					if e != nil {
						// If even one connection failed, we cannot proceed.
						return errors.Errorf("'juju-restore' could not connect to all controller machines: controllers' agents cannot be managed")
					}
				}
			}
		} else {
			c.ui.Notify(secondaryAgentsMustStop)
		}

	}
	c.ui.Notify(preChecksCompleted)
	if err := c.ui.UserConfirmYes(); err != nil {
		return errors.Annotate(err, "restore operation")
	}
	return nil
}
