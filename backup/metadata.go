// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backup

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/juju/errors"
	"github.com/juju/mgo/v2/bson"
	"github.com/juju/version/v2"

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
	// the machines dump file instead.
	haNodes, err := countHANodes(directory, targetV0.Environment)
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

func eachBsonDoc(source io.Reader, callback func([]byte) error) error {
	var size uint32
	var buf bytes.Buffer
	for {
		// Each bson document starts with a 32-bit little-endian size.
		err := binary.Read(source, binary.LittleEndian, &size)
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return errors.Trace(err)
		}
		buf.Grow(int(size))
		err = binary.Write(&buf, binary.LittleEndian, size)
		if err != nil {
			return errors.Trace(err)
		}
		_, err = io.CopyN(&buf, source, int64(size-4))
		if err != nil {
			return errors.Trace(err)
		}

		// Pass the bytes rather than unmarshalling so the callback
		// can decide how (or whether) to unmarshal it.
		err = callback(buf.Bytes())
		if err != nil {
			return errors.Trace(err)
		}

		buf.Reset()
	}
}

func countBsonDocs(path string) (int, error) {
	source, err := os.Open(path)
	if err != nil {
		return 0, errors.Trace(err)
	}
	defer source.Close()

	var count int
	err = eachBsonDoc(source, func(_ []byte) error {
		count++
		return nil
	})
	if err != nil {
		return 0, errors.Trace(err)
	}
	return count, nil
}

const jobManageModel = 2

func countHANodes(directory, modelUUID string) (int, error) {
	// If we have a controllerNodes collection dump, use that.
	count, err := countBsonDocs(filepath.Join(directory, controllerNodesFile))
	if err == nil {
		return count, nil
	} else if !os.IsNotExist(errors.Cause(err)) {
		return 0, errors.Trace(err)
	}

	// Fall back to counting machines in the right model with the
	// right job.
	source, err := os.Open(filepath.Join(directory, machinesFile))
	if err != nil {
		return 0, errors.Trace(err)
	}
	defer source.Close()

	var haNodes, docCount int
	err = eachBsonDoc(source, func(data []byte) error {
		docCount++
		var doc struct {
			ModelUUID string `bson:"model-uuid"`
			Jobs      []int  `bson:"jobs"`
		}

		err := bson.Unmarshal(data, &doc)
		if err != nil {
			return errors.Annotatef(err, "reading machine doc %d", docCount)
		}

		if doc.ModelUUID != modelUUID {
			return nil
		}
		for _, job := range doc.Jobs {
			if job == jobManageModel {
				haNodes++
				break
			}
		}
		return nil
	})

	if err != nil {
		return 0, errors.Trace(err)
	}
	return haNodes, nil
}
