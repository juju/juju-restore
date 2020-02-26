// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backup_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju-restore/backup"
	"github.com/juju/juju-restore/core"
)

type backupSuite struct {
	testing.IsolationSuite

	dir string
}

var _ = gc.Suite(&backupSuite{})

func (s *backupSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	dir, err := ioutil.TempDir("", "juju-restore-backup-tests")
	c.Assert(err, jc.ErrorIsNil)
	s.dir = dir
	s.AddCleanup(func(c *gc.C) {
		err := os.RemoveAll(s.dir)
		c.Assert(err, jc.ErrorIsNil)
	})
}

func (s *backupSuite) TestOpen(c *gc.C) {
	path := filepath.Join("testdata", "valid-backup.tar.gz")
	opened, err := backup.Open(path, s.dir)
	c.Assert(err, jc.ErrorIsNil)
	defer opened.Close()

	names := set.NewStrings()
	err = filepath.Walk(s.dir, func(path string, finfo os.FileInfo, err error) error {
		remainder := path[len(s.dir):]
		parts := strings.Split(remainder, string(filepath.Separator))
		if len(parts) <= 1 {
			return nil
		}
		names.Add(filepath.Join(parts[2:]...))
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(names.Contains("juju-backup"), gc.Equals, true)
	c.Assert(names.Contains("juju-backup/metadata.json"), gc.Equals, true)
	c.Assert(names.Contains("juju-backup/dump"), gc.Equals, true)
	c.Assert(names.Contains("juju-backup/home"), gc.Equals, true)

	err = opened.Close()
	c.Assert(err, jc.ErrorIsNil)

	items, err := ioutil.ReadDir(s.dir)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(items, gc.HasLen, 0)
}

func (s *backupSuite) TestOpenMissingRoot(c *gc.C) {
	path := filepath.Join("testdata", "missing-root-backup.tar.gz")
	opened, err := backup.Open(path, s.dir)
	c.Assert(err, gc.ErrorMatches, `extracting root.tar in ".*": open .*/root.tar: no such file or directory`)
	c.Assert(opened, gc.Equals, nil)
}

func (s *backupSuite) TestMetadata(c *gc.C) {
	path := filepath.Join("testdata", "valid-backup.tar.gz")
	opened, err := backup.Open(path, s.dir)
	c.Assert(err, jc.ErrorIsNil)
	defer opened.Close()

	metadata, err := opened.Metadata()
	c.Assert(err, jc.ErrorIsNil)
	expectCreated, err := time.Parse(time.RFC3339, "2020-02-25T04:12:41.038760008Z")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metadata, gc.Equals, core.BackupMetadata{
		FormatVersion:       0,
		ControllerModelUUID: "e2a6a1e5-abea-4393-8593-5a45ae53ab97",
		JujuVersion:         version.MustParse("2.8-beta1.1"),
		Series:              "bionic",
		BackupCreated:       expectCreated,
		Hostname:            "juju-53ab97-0",
		ContainsLogs:        false,
		ModelCount:          2,
	})
}
