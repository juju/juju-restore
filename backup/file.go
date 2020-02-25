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
	"github.com/juju/utils/tar"

	"github.com/juju/juju-restore/core"
)

var logger = loggo.GetLogger("juju-restore.backup")

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
	extractedDir := filepath.Join(destDir, "juju-backup")
	err = extractFiles(filepath.Join(extractedDir, "root.tar"), extractedDir)
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
	// TODO(babbageclunk): this
	return core.BackupMetadata{}, nil
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
