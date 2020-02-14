// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd

import (
	"bytes"
	"text/template"

	"github.com/juju/juju-restore/core"
)

const restoreDoc = `

juju-restore must be executed on the MongoDB primary host of a Juju
controller.

The command will check the state of the target database and the
details of the backup file provided, and restore the contents of the
backup into the controller database.

`

// preChecksCompleteTemplate contains the output of all pre-check runs
// as well as the important details of backup file that is about to be restored.
const preChecksCompleteTemplate = `
Replica set is healthy     ✓
Running on primary HA node ✓

All restore pre-checks are completed.

The restore can now proceed.

Continue [y/N]? 
`

func prechecksCompleted(prechecks *core.PrecheckResult) string {
	t := template.Must(template.New("plugin").Parse(preChecksCompleteTemplate))
	content := bytes.Buffer{}
	t.Execute(&content, prechecks)
	return content.String()
}
