// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/loggo"
	"github.com/juju/utils"

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
	openBackup func(path, tempRoot string) (core.BackupFile, error),
	machineConverter func(member core.ReplicaSetMember) core.ControllerNode,
	readFunc func(*cmd.Context) (string, error),
	loadCreds func() (string, string, error),
	devMode bool,
) cmd.Command {
	return &restoreCommand{
		connect:     dbConnect,
		openBackup:  openBackup,
		converter:   machineConverter,
		readOneChar: readFunc,
		loadCreds:   loadCreds,
		devMode:     devMode,
	}
}

type restoreCommand struct {
	cmd.CommandBase

	connect     func(info db.DialInfo) (core.Database, error)
	openBackup  func(path, tempRoot string) (core.BackupFile, error)
	converter   func(member core.ReplicaSetMember) core.ControllerNode
	readOneChar func(*cmd.Context) (string, error)
	loadCreds   func() (string, string, error)
	devMode     bool

	hostname string
	port     string
	ssl      bool
	username string
	password string

	verbose              bool
	loggingConfig        string
	backupFile           string
	tempRoot             string
	restoreLog           string
	includeStatusHistory bool

	// manualAgentControl determines if 'juju-restore' or the operator
	// manages - stops and starts juju and mongo agents - on
	// other, non-primary controller nodes.
	// If true, the control is manual and 'juju-restore' will do nothing
	// to other controller nodes.
	manualAgentControl bool

	ui       *UserInteractions
	restorer *core.Restorer

	// To be used as an option during development to enable an easier
	// way to re-start all agents in HA federation.
	// TODO: Remove once complete.
	restart bool
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
	f.StringVar(&c.username, "username", "", "user for connecting to MongoDB (omit to get credentials from agent.conf)")
	f.StringVar(&c.password, "password", "", "password for connecting to MongoDB")
	f.StringVar(&c.loggingConfig, "logging-config", defaultLogConfig, "set logging levels")
	f.BoolVar(&c.verbose, "verbose", false, "more output from restore (debug logging)")
	f.BoolVar(&c.manualAgentControl, "manual-agent-control", false, "operator manages secondary controller nodes in HA, e.g stops/starts Juju and Mongo agents")
	f.StringVar(&c.tempRoot, "temp-root", "/tmp", "location to unpack backup file")
	f.StringVar(&c.restoreLog, "restore-log", "restore.log", "location to write mongorestore logging output")
	f.BoolVar(&c.includeStatusHistory, "include-status-history", false, "restore status history for machines and units (can be large)")
	if c.devMode {
		f.BoolVar(&c.restart, "rs", false, "just restart agents that were stopped (JUJU_RESTORE_DEV_MODE)")
	}
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

	username := c.username
	password := c.password
	if c.username == "" {
		username, password, err = c.loadCreds()
		if err != nil {
			return errors.Annotate(err, "loading credentials")
		}
	}

	c.ui = NewUserInteractions(ctx, c.readOneChar)
	c.ui.Notify("Connecting to database...\n")
	database, err := c.connect(db.DialInfo{
		Hostname: c.hostname,
		Port:     c.port,
		Username: username,
		Password: password,
		SSL:      c.ssl,
	})
	if err != nil {
		return errors.Trace(err)
	}
	defer database.Close()

	backup, err := c.openBackup(c.backupFile, c.tempRoot)
	if err != nil {
		return errors.Annotatef(err, "unpacking backup file %q under %q", c.backupFile, c.tempRoot)
	}
	defer backup.Close()

	restorer, err := core.NewRestorer(database, backup, c.converter)
	if err != nil {
		return errors.Trace(err)
	}
	c.restorer = restorer

	if c.restart {
		return errors.Trace(c.runPostChecks())
	}

	// Pre-checks
	if err := c.runPreChecks(); err != nil {
		return errors.Trace(err)
	}
	// Actual restore
	if err := c.restore(); err != nil {
		return errors.Trace(err)
	}
	// Post-checks
	if err := c.runPostChecks(); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (c *restoreCommand) runPreChecks() error {
	c.ui.Notify("Checking database and replica set health...\n")
	if err := c.restorer.CheckDatabaseState(); err != nil {
		return errors.Trace(err)
	}
	c.ui.Notify(dbHealthComplete)

	precheckResult, err := c.restorer.CheckRestorable()
	if err != nil {
		return errors.Annotate(err, "precheck")
	}

	c.ui.Notify(populate(backupFileTemplate, precheckResult))

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
				c.ui.Notify(populate(nodesTemplate, connections))
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

func (c *restoreCommand) restore() error {
	// Stop juju agents.
	c.ui.Notify("\nStopping Juju agents...\n")
	if err := c.manipulateAgents(c.restorer.StopAgents); err != nil {
		return errors.Trace(err)
	}
	c.ui.Notify("\nRunning restore...\n")
	c.ui.Notify(fmt.Sprintf("Detailed mongorestore output in %s.\n", c.restoreLog))
	if err := c.restorer.Restore(c.restoreLog, c.includeStatusHistory); err != nil {
		return errors.Trace(err)
	}

	c.ui.Notify("\nDatabase restore complete.")
	return nil
}

func (c *restoreCommand) runPostChecks() error {
	c.ui.Notify("\nStarting Juju agents...\n")
	if err := c.manipulateAgents(c.restorer.StartAgents); err != nil {
		return errors.Trace(err)
	}

	if c.restorer.IsHA() {
		c.ui.Notify("Primary node may have shifted.\n")
	}
	return nil
}

func (c *restoreCommand) manipulateAgents(operation func(bool) map[string]error) error {
	connections := operation(!c.manualAgentControl)
	c.ui.Notify(populate(nodesTemplate, connections))
	for _, e := range connections {
		if e != nil {
			// If even one connection failed, we cannot proceed.
			return errors.Errorf("'juju-restore' could not manipulate all necessary agents: controllers' agents cannot be managed")
		}
	}
	return nil
}

const agentConfPattern = "/var/lib/juju/agents/machine-*/agent.conf"

// ReadCredsFromAgentConf tries to load a mongo username and password
// from the standard agent.conf location on a controller machine.
func ReadCredsFromAgentConf() (string, string, error) {
	return ReadCredsFromPattern(agentConfPattern)
}

// ReadCredsFromPattern tries to load a mongo username and password
// from the first file it finds matching the pattern passed in.
func ReadCredsFromPattern(pattern string) (string, string, error) {
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return "", "", errors.Trace(err)
	}
	if len(matches) == 0 {
		return "", "", errors.Errorf("couldn't find an agent.conf - please specify username and password")
	}
	conf := matches[0]

	var creds struct {
		Username string `yaml:"tag"`
		Password string `yaml:"statepassword"`
	}
	err = utils.ReadYaml(conf, &creds)
	if err != nil {
		return "", "", errors.Annotatef(err, "reading %q", conf)
	}

	if creds.Username == "" {
		return "", "", errors.Errorf("no username found in %q - tag field is missing or blank", conf)
	}
	if creds.Password == "" {
		return "", "", errors.Errorf("no password found in %q - statepassword field is missing or blank", conf)
	}

	return creds.Username, creds.Password, nil
}
