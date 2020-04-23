// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd

import (
	"bytes"
	"text/template"
)

const (
	restoreDoc = `

juju-restore must be executed on the MongoDB primary host of a Juju
controller.

The command will check the state of the target database and the
details of the backup file provided, and restore the contents of the
backup into the controller database.

`

	dbHealthComplete = `
Replica set is healthy     ✓
Running on primary HA node ✓
`

	releaseAgentsControl = `
This controller is in HA and to restore into it successfully, 
'juju-restore' needs to manage Juju and Mongo agents on  
secondary controller nodes.
However, on the bigger systems, the operator might want to manage 
these agents manually.

Do you want 'juju-restore' to manage these agents automatically? (y/N): `

	nodesTemplate = `{{range $k,$v := . }} 
    {{$k}} {{if $v}}✗ error: {{ $v }}{{else}}✓ {{end}}{{end}}
`

	backupFileTemplate = `
You are about to restore a controller from a backup file taken on {{.BackupDate}}. 
It contains a controller {{.ControllerModelUUID}} at Juju version {{.BackupJujuVersion}} with {{.ModelCount}} models.
`

	preChecksCompleted = `
All restore pre-checks are completed.

Restore cannot be cleanly aborted from here on.

Are you sure you want to proceed? (y/N): `

	secondaryAgentsMustStop = `
Juju agents on secondary controller machines must be stopped by this point.
To stop the agents, login into each secondary controller and run:
    $ systemctl stop jujud-machine-*
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
