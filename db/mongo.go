// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package db

import (
	"crypto/tls"
	"net"

	"github.com/juju/errors"
	"github.com/juju/replicaset"
	"gopkg.in/mgo.v2"

	"github.com/juju/juju-restore/core"
)

// DialInfo holds information needed to connect to the database.
type DialInfo struct {
	Hostname string
	Port     string
	Username string
	Password string
	SSL      bool
}

// Dial creates a new connection to the specified database.
func Dial(args DialInfo) (core.Database, error) {
	info := mgo.DialInfo{
		Addrs:    []string{net.JoinHostPort(args.Hostname, args.Port)},
		Database: "admin",
		Username: args.Username,
		Password: args.Password,
		Direct:   true,
	}
	if args.SSL {
		info.DialServer = dialSSL
	}
	session, err := mgo.DialWithInfo(&info)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// We need to set preference to nearest since we're connecting
	// directly, not to all the nodes in the replicaset.
	session.SetMode(readPreferenceNearest, false)
	return &database{session: session}, nil
}

const readPreferenceNearest = 6

type database struct {
	session *mgo.Session
}

// ReplicaSet is part of core.Database.
func (db *database) ReplicaSet() (core.ReplicaSet, error) {
	status, err := replicaset.CurrentStatus(db.session)
	if err != nil {
		return core.ReplicaSet{}, errors.Trace(err)
	}
	// Current members collection of replicaset contains additional
	// information for the nodes, including machine IDs.
	members, err := replicaset.CurrentMembers(db.session)
	if err != nil {
		return core.ReplicaSet{}, errors.Trace(err)
	}
	mapped := map[int]replicaset.Member{}
	for _, v := range members {
		mapped[v.Id] = v
	}
	machineID := func(member replicaset.Member) string {
		t, k := member.Tags["juju-machine-id"]
		if !k {
			return ""
		}
		return t
	}

	result := core.ReplicaSet{
		Name:    status.Name,
		Members: make([]core.ReplicaSetMember, len(status.Members)),
	}
	for i, m := range status.Members {
		result.Members[i] = core.ReplicaSetMember{
			ID:            m.Id,
			Name:          m.Address,
			Self:          m.Self,
			Healthy:       m.Healthy,
			State:         m.State.String(),
			JujuMachineID: machineID(mapped[m.Id]),
		}
	}
	return result, nil

}

// Close is part of core.Database.
func (db *database) Close() {
	db.session.Close()
}

func dialSSL(addr *mgo.ServerAddr) (net.Conn, error) {
	c, err := net.Dial("tcp", addr.String())
	if err != nil {
		return nil, err
	}
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true,
	}
	cc := tls.Client(c, tlsConfig)
	if err := cc.Handshake(); err != nil {
		return nil, err
	}
	return cc, nil
}
