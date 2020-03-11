// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backup

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/juju/errors"
	"github.com/juju/version"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju-restore/core"
)

func readMetadataJSON(directory string) (core.BackupMetadata, error) {
	source, err := os.Open(filepath.Join(directory, metadataFile))
	if err != nil {
		return core.BackupMetadata{}, errors.Trace(err)
	}
	defer source.Close()
	data, err := ioutil.ReadAll(source)

	// Try the current version and check the FormatVersion first.
	var target flatMetadata
	err = json.Unmarshal(data, &target)
	if err != nil {
		return core.BackupMetadata{}, errors.Annotate(err, "unmarshalling v1 metadata")
	}

	if target.FormatVersion > 1 {
		return core.BackupMetadata{}, errors.Errorf("unsupported backup format version %d", target.FormatVersion)
	}
	if target.FormatVersion == 1 {
		return flatToBackupMetadata(target), nil
	}

	// No FormatVersion set - it must be a version 0 structure
	// instead.
	var targetV0 flatMetadataV0
	err = json.Unmarshal(data, &targetV0)
	if err != nil {
		return core.BackupMetadata{}, errors.Annotate(err, "unmarshalling v0 metadata")
	}

	// There's no HANodes field in version 0 metadata - get it from
	// the agent.conf instead.
	haNodes, err := countHANodes(directory)
	if err != nil {
		return core.BackupMetadata{}, errors.Annotate(err, "counting HA nodes")
	}
	return flatV0ToBackupMetadata(targetV0, haNodes), nil
}

// Duplicating the flat metadata formats from the juju codebase for
// now - we'll need to share this between the two projects.

type flatMetadata struct {
	ID            string
	FormatVersion int64

	// file storage

	Checksum       string
	ChecksumFormat string
	Size           int64
	Stored         time.Time

	// backup

	Started                     time.Time
	Finished                    time.Time
	Notes                       string
	ModelUUID                   string
	Machine                     string
	Hostname                    string
	Version                     version.Number
	Series                      string
	ControllerUUID              string
	HANodes                     int64
	ControllerMachineID         string
	ControllerMachineInstanceID string
	CACert                      string
	CAPrivateKey                string
}

func flatToBackupMetadata(source flatMetadata) core.BackupMetadata {
	return core.BackupMetadata{
		FormatVersion:       source.FormatVersion,
		ControllerModelUUID: source.ModelUUID,
		JujuVersion:         source.Version,
		Series:              source.Series,
		BackupCreated:       source.Started,
		Hostname:            source.Hostname,
		HANodes:             int(source.HANodes),
	}
}

type flatMetadataV0 struct {
	ID string

	// file storage

	Checksum       string
	ChecksumFormat string
	Size           int64
	Stored         time.Time

	// backup

	Started     time.Time
	Finished    time.Time
	Notes       string
	Environment string
	Machine     string
	Hostname    string
	Version     version.Number
	Series      string

	CACert       string
	CAPrivateKey string
}

func flatV0ToBackupMetadata(source flatMetadataV0, haNodes int) core.BackupMetadata {
	return core.BackupMetadata{
		FormatVersion:       0,
		ControllerModelUUID: source.Environment,
		JujuVersion:         source.Version,
		Series:              source.Series,
		BackupCreated:       source.Started,
		Hostname:            source.Hostname,
		HANodes:             haNodes,
	}
}

const agentConfPattern = "var/lib/juju/agents/machine-*/agent.conf"

func countHANodes(directory string) (int, error) {
	matches, err := filepath.Glob(filepath.Join(directory, topLevelDir, agentConfPattern))
	if err != nil {
		return 0, errors.Trace(err)
	}
	if len(matches) != 1 {
		return 0, errors.Errorf("expected one machine agent.conf, found %d: %#v", len(matches), matches)
	}
	agentConf, err := os.Open(matches[0])
	if err != nil {
		return 0, errors.Trace(err)
	}
	defer agentConf.Close()

	contents, err := ioutil.ReadAll(agentConf)
	if err != nil {
		return 0, errors.Annotatef(err, "reading agent.conf file %q", matches[0])
	}
	var data map[string]interface{}
	err = yaml.Unmarshal(contents, &data)
	if err != nil {
		return 0, errors.Annotatef(err, "reading config file %q", matches[0])
	}
	value, ok := data["apiaddresses"]
	if !ok {
		return 0, errors.Errorf("no apiaddresses in %q", matches[0])
	}
	addresses, ok := value.([]interface{})
	if !ok {
		return 0, errors.Errorf("apiaddresses not a list in %q", matches[0])
	}
	return len(addresses), nil
}
