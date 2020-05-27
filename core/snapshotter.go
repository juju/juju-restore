// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package core

import "github.com/juju/errors"

// NewSnapshotter returns a new snapshotter to allow taking and
// restoring/discarding database snapshots.
func NewSnapshotter(db Database, primary ControllerNode, others []ControllerNode) *Snapshotter {
	return &Snapshotter{
		db:        db,
		primary:   primary,
		others:    others,
		snapshots: make(map[string]string),
	}
}

// Snapshotter manages database snapshotting across multiple
// controller nodes.
type Snapshotter struct {
	db      Database
	primary ControllerNode
	others  []ControllerNode

	// snapshots maps from IP address of each machine to the name of
	// that machine's snapshot.
	snapshots map[string]string
}

func (s *Snapshotter) apply(machines []ControllerNode, f func(ControllerNode) error) error {
	for _, machine := range machines {
		err := f(machine)
		if err != nil {
			return errors.Annotatef(err, "on %s", machine)
		}
	}
	return nil
}

func (s *Snapshotter) primaryLast() []ControllerNode {
	result := append([]ControllerNode{}, s.others...)
	return append(result, s.primary)
}

func (s *Snapshotter) primaryFirst() []ControllerNode {
	result := []ControllerNode{s.primary}
	return append(result, s.others...)
}

func (s *Snapshotter) tryRestartAll() {
	for _, n := range s.primaryFirst() {
		status, err := n.Status()
		if err != nil {
			logger.Errorf("couldn't get status on %s: %s", n, err)
			continue
		}
		if status.DatabaseRunning {
			continue
		}
		err = n.StartService(DatabaseService)
		if err != nil {
			logger.Errorf("couldn't restart database on %s: %s", n, err)
		}
	}
}

func (s *Snapshotter) stopAll() error {
	return errors.Trace(s.apply(s.primaryLast(), func(n ControllerNode) error {
		return errors.Trace(n.StopService(DatabaseService))
	}))
}

func (s *Snapshotter) startAll() error {
	return errors.Trace(s.apply(s.primaryFirst(), func(n ControllerNode) error {
		return errors.Trace(n.StartService(DatabaseService))
	}))
}

// Snapshot takes a snapshot on each machine (stopping and restarting
// the database).
func (s *Snapshotter) Snapshot() (err error) {
	if len(s.snapshots) != 0 {
		return errors.Errorf("snapshots have already been created")
	}

	defer func() {
		// Make a best-efforts attempt to leave mongo running on the
		// nodes if something bad happened.
		if err == nil {
			return
		}
		s.tryRestartAll()
	}()

	// Stop mongo on each machine.
	if err := s.stopAll(); err != nil {
		return errors.Annotate(err, "stopping databases")
	}

	err = s.apply(s.primaryFirst(), func(n ControllerNode) error {
		name, err := n.SnapshotDatabase()
		if err != nil {
			return errors.Trace(err)
		}
		s.snapshots[n.IP()] = name
		return nil
	})
	if err != nil {
		return errors.Annotate(err, "snapshotting databases")
	}

	if err := s.startAll(); err != nil {
		return errors.Annotate(err, "starting databases")
	}
	return errors.Annotate(s.db.Reconnect(), "reconnecting to db")
}

// Discard gets rid of un-needed snapshots.
func (s *Snapshotter) Discard() error {
	errs := 0
	for _, machine := range s.primaryFirst() {
		name, found := s.snapshots[machine.IP()]
		if !found {
			continue
		}
		err := machine.DiscardSnapshot(name)
		if err != nil {
			logger.Errorf("error discarding snapshot %q on %s: %s", name, machine, err)
			errs++
		}
	}
	if errs > 0 {
		return errors.Errorf("errors discarding snapshots: %d", errs)
	}
	return nil
}

// Restore restores the snapshot on all nodes.
func (s *Snapshotter) Restore() error {
	if len(s.snapshots) != len(s.others)+1 {
		return errors.Errorf("not all machines have snapshots so only discarding is allowed")
	}
	for _, machine := range s.primaryFirst() {
		_, found := s.snapshots[machine.IP()]
		if !found {
			return errors.Errorf("no snapshot found for %s", machine)
		}
	}

	// Stop mongo on each machine.
	if err := s.stopAll(); err != nil {
		// Don't leave the databases stopped if we only managed to
		// stop some.
		s.tryRestartAll()
		return errors.Annotate(err, "stopping databases")
	}

	err := s.apply(s.primaryFirst(), func(n ControllerNode) error {
		name := s.snapshots[n.IP()]
		err := n.RestoreSnapshot(name)
		if err != nil {
			return errors.Annotatef(err, "restoring snapshot %q", name)
		}
		// Restoring the snapshot successfully removes it too - no
		// need to discard it later.
		delete(s.snapshots, n.IP())
		return nil
	})
	if err != nil {
		// If we didn't manage to restore any snapshots then we can
		// restart the databases.
		if len(s.snapshots) == len(s.others)+1 {
			s.tryRestartAll()
		}
		// TODO(babbageclunk): What do we do if we've partially
		// restored the snapshots? Retry until we've managed to
		// restore all of them or we hit some timeout?
		return errors.Trace(err)
	}

	if err := s.startAll(); err != nil {
		return errors.Annotate(err, "starting databases")
	}
	return errors.Annotate(s.db.Reconnect(), "reconnecting to db")
}
