// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd

import (
	"bytes"
	"text/template"
)

const (
	restoreDoc = `

juju-restore must be executed on the MongoDB primary host of a Juju controller.

The command will check the state of the target database and the details of the 
backup file provided, and restore the contents of the backup into the 
controller database.

The --copy-controller option is used to clone key aspects of an existing controller
set up into a new controller. The main reason for using this option is when upgrading Juju.
This option will prepare a new controller so that models can be migrated off the source controller.
The target controller will be configured with these options from the source backup:
- core controller config
- hosted clouds and credentials
- users and credentials
- user controller and cloud permissions
Note that when copying controller config across, the target controller name, login password,
CA certificate remain unchanged. 
`

	dbHealthComplete = `
Replica set is healthy     ✓
Running on primary HA node ✓
`

	releaseAgentsControl = `
This controller is in HA and to restore into it successfully, 'juju-restore' 
needs to manage Juju and Mongo agents on secondary controller nodes.
However on bigger systems the user might want to manage these agents manually.

Do you want 'juju-restore' to manage these agents automatically? (y/N): `

	nodesTemplate = `{{range $k,$v := . }} 
    {{$k}} {{if $v}}✗ error: {{ $v }}{{else}}✓ {{end}}{{end}}
`

	backupFileTemplate = `
You are about to restore this backup:
    Created at:   {{.BackupDate}}
    Controller:   {{.ControllerModelUUID}}
    Juju version: {{.BackupJujuVersion}}
    Models:       {{.ModelCount}}
`

	backupFileControllerTemplate = `
You are about to copy this controller:
    Created at:   {{.BackupDate}}
    Controller:   {{.ControllerUUID}}
    Juju version: {{.BackupJujuVersion}}
    Clouds:       {{.CloudCount}}
`

	preChecksCompleted = `
All restore pre-checks are completed.

Restore cannot be cleanly aborted from here on.

Are you sure you want to proceed? (y/N): `

	secondaryAgentsMustStop = `
Juju agents on secondary controller machines must be stopped by this point.
To stop the agents, login into each secondary controller and run:
    $ sudo systemctl stop jujud-machine-*
`
)

func populate(aTemplate string, data interface{}) string {
	t := template.Must(template.New("fragment").Parse(aTemplate))
	content := bytes.Buffer{}
	err := t.Execute(&content, data)
	if err != nil {
		logger.Errorf("creating user message: %v", err)
	}
	return content.String()
}
