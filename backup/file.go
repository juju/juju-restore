// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package backup provides a concrete implementation of core.BackupFile.
package backup

import (
	"compress/gzip"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/v3/tar"

	"github.com/juju/juju-restore/core"
)

var logger = loggo.GetLogger("juju-restore.backup")

const (
	topLevelDir         = "juju-backup"
	rootTarFile         = "root.tar"
	metadataFile        = "juju-backup/metadata.json"
	dumpDir             = "juju-backup/dump"
	logsDir             = "juju-backup/dump/logs"
	modelsFile          = "juju-backup/dump/juju/models.bson"
	machinesFile        = "juju-backup/dump/juju/machines.bson"
	controllerNodesFile = "juju-backup/dump/juju/controllerNodes.bson"
)

// Open unpacks a backup file in a temp location and returns a
// core.BackupFile that gives access to the db dumps, files and
// metadata contained therein. The backup file passed in should be a
// tar.gz file in the standard Juju format.
func Open(path string, tempRoot string) (_ core.BackupFile, err error) {
	destDir, err := ioutil.TempDir(tempRoot, "juju-restore")
	if err != nil {
		return nil, errors.Annotatef(err, "creating temp directory in %q", tempRoot)
	}
	defer func() {
		if err == nil {
			return
		}
		removeErr := os.RemoveAll(destDir)
		if removeErr != nil {
			logger.Errorf("couldn't remove temp dir %q: %s", destDir, removeErr)
		}
	}()

	err = extractFiles(path, destDir)
	if err != nil {
		return nil, errors.Annotatef(err, "extracting backup to %q", destDir)
	}
	// Inside the extracted directory is another root.tar file that we can
	// extract in place.
	extractedDir := filepath.Join(destDir, topLevelDir)
	err = extractFiles(filepath.Join(extractedDir, rootTarFile), extractedDir)
	if err != nil {
		return nil, errors.Annotatef(err, "extracting root.tar in %q", destDir)
	}

	return &expandedBackup{dir: destDir}, nil
}

type expandedBackup struct {
	dir string
}

// Metadata returns the collected info from the backup file. Part of
// core.BackupFile.
func (b *expandedBackup) Metadata() (core.BackupMetadata, error) {
	result, err := readMetadataJSON(b.dir)
	if err != nil {
		return core.BackupMetadata{}, errors.Annotate(err, "reading metadata")
	}
	result.ContainsLogs, err = b.containsLogs()
	if err != nil {
		return core.BackupMetadata{}, errors.Annotate(err, "checking for logs")
	}
	result.ModelCount, err = b.countModels()
	if err != nil {
		return core.BackupMetadata{}, errors.Annotate(err, "counting models")
	}
	return result, nil
}

func (b *expandedBackup) containsLogs() (bool, error) {
	items, err := ioutil.ReadDir(filepath.Join(b.dir, logsDir))
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, errors.Trace(err)
	}
	return len(items) > 0, nil
}

func (b *expandedBackup) countModels() (int, error) {
	return countBsonDocs(filepath.Join(b.dir, modelsFile))
}

// DumpDirectory returns the path of the contained database dump.
func (b *expandedBackup) DumpDirectory() string {
	return filepath.Join(b.dir, dumpDir)
}

// Close is part of core.BackupFile. It removes the temp directory the
// backup file has been extracted into.
func (b *expandedBackup) Close() error {
	return errors.Trace(os.RemoveAll(b.dir))
}

func extractFiles(path string, dest string) error {
	logger.Debugf("extracting %q to %q", path, dest)
	source, err := os.Open(path)
	if err != nil {
		return errors.Trace(err)
	}
	defer source.Close()

	tarSource := io.Reader(source)
	if strings.HasSuffix(path, ".gz") {
		gzReader, err := gzip.NewReader(source)
		if err != nil {
			return errors.Trace(err)
		}
		defer gzReader.Close()
		tarSource = gzReader
	}

	return errors.Trace(tar.UntarFiles(tarSource, dest))
}
