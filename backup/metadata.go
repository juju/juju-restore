// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backup

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"time"

	"github.com/juju/errors"
	"github.com/juju/version"

	"github.com/juju/juju-restore/core"
)

func readMetadataJSON(path string) (core.BackupMetadata, error) {
	source, err := os.Open(path)
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
	return flatV0ToBackupMetadata(targetV0), nil
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

func flatV0ToBackupMetadata(source flatMetadataV0) core.BackupMetadata {
	return core.BackupMetadata{
		FormatVersion:       0,
		ControllerModelUUID: source.Environment,
		JujuVersion:         source.Version,
		Series:              source.Series,
		BackupCreated:       source.Started,
		Hostname:            source.Hostname,
	}
}
