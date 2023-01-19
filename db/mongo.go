// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package db

import (
	"crypto/tls"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/mgo/v2"
	"github.com/juju/mgo/v2/bson"
	"github.com/juju/replicaset/v2"
	"github.com/juju/version/v2"

	"github.com/juju/juju-restore/core"
)

var logger = loggo.GetLogger("juju-restore.db")

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
	return &database{session: session, info: args}, nil
}

const readPreferenceNearest = 6

type database struct {
	info    DialInfo
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

const jobManageModel = 2
const alive = 0

const (
	jujuDBName           = "juju"
	jujuControllerDBName = "jujucontroller"
)

// ControllerInfo is part of core.Database.
func (db *database) ControllerInfo() (core.ControllerInfo, error) {
	var result core.ControllerInfo

	jujuDB := db.session.DB(jujuDBName)
	num, err := jujuDB.C("models").Find(nil).Count()
	if err != nil {
		return core.ControllerInfo{}, errors.Annotate(err, "getting model count")
	}
	result.Models = num

	var modelDoc struct {
		ID              string `bson:"_id"`
		ControllerUUID  string `bson:"controller-uuid"`
		Cloud           string `bson:"cloud"`
		CloudCredential string `bson:"cloud-credential"`
	}
	err = jujuDB.C("models").Find(bson.M{"name": "controller"}).One(&modelDoc)
	if err != nil {
		return core.ControllerInfo{}, errors.Annotate(err, "getting controller model")
	}
	result.ControllerModelUUID = modelDoc.ID
	result.ControllerUUID = modelDoc.ControllerUUID
	result.ControllerModelCloud = modelDoc.Cloud
	result.ControllerModelCloudCredential = strings.Replace(modelDoc.CloudCredential, "/", "#", -1)

	var settingsDoc struct {
		Settings map[string]interface{} `bson:"settings"`
	}
	modelSettingsKey := modelDoc.ID + ":e"
	err = jujuDB.C("settings").FindId(modelSettingsKey).One(&settingsDoc)
	if err != nil {
		return core.ControllerInfo{}, errors.Annotate(err, "getting controller settings")
	}
	versionVal, ok := settingsDoc.Settings["agent-version"]
	if !ok {
		return core.ControllerInfo{}, errors.Errorf("no agent-version in controller settings")
	}
	versionStr, ok := versionVal.(string)
	if !ok {
		return core.ControllerInfo{}, errors.Errorf("expected agent-version to be a string, got %#v", versionVal)
	}
	result.JujuVersion, err = version.Parse(versionStr)
	if err != nil {
		return core.ControllerInfo{}, errors.Trace(err)
	}

	var machineDoc struct {
		Series string `bson:"series"`
	}
	query := bson.M{
		"model-uuid": modelDoc.ID,
		"jobs":       bson.M{"$in": []int{jobManageModel}},
		"life":       alive,
	}
	iter := jujuDB.C("machines").Find(query).Iter()
	allSeries := set.NewStrings()
	for iter.Next(&machineDoc) {
		result.HANodes++
		allSeries.Add(machineDoc.Series)
	}
	if err := iter.Close(); err != nil {
		return core.ControllerInfo{}, errors.Annotate(err, "getting controller series")
	}

	allSeriesNames := allSeries.SortedValues()
	if len(allSeriesNames) != 1 {
		return core.ControllerInfo{}, errors.Errorf("expected one series, got %#v", allSeriesNames)
	}

	result.Series = allSeriesNames[0]
	return result, nil
}

// settingsDoc is the mongo document representation for settings.
type settingsDoc struct {
	DocID     string      `bson:"_id"`
	ModelUUID string      `bson:"model-uuid"`
	Settings  settingsMap `bson:"settings"`
}

type settingsMap map[string]interface{}

func (m *settingsMap) SetBSON(raw bson.Raw) error {
	rawMap := make(map[string]interface{})
	if err := raw.Unmarshal(rawMap); err != nil {
		return err
	}
	*m = UnescapeKeys(rawMap)
	return nil
}

func (m settingsMap) GetBSON() (interface{}, error) {
	escapedMap := EscapeKeys(m)
	return escapedMap, nil
}

func (db *database) copyCollection(collName, skipID string) error {
	jujuControllerDB := db.session.DB(jujuControllerDBName)

	var data []bson.M
	sourceColl := jujuControllerDB.C(collName)
	err := sourceColl.Find(nil).All(&data)
	if err != nil {
		return errors.Annotatef(err, "reading source %s", collName)
	}

	jujuDB := db.session.DB(jujuDBName)
	col := jujuDB.C(collName)
	bulk := col.Bulk()
	for _, u := range data {
		if u["_id"] == skipID {
			continue
		}
		bulk.Upsert(bson.M{"_id": u["_id"]}, bson.M{"$set": u})
	}
	_, err = bulk.Run()
	if err != nil {
		return errors.Annotatef(err, "writing target %s", collName)
	}
	return nil
}

func (db *database) copyPermissions(controller core.ControllerInfo) error {
	jujuControllerDB := db.session.DB(jujuControllerDBName)

	var data []bson.M
	sourceUsers := jujuControllerDB.C("permissions")
	err := sourceUsers.Find(nil).All(&data)
	if err != nil {
		return errors.Annotatef(err, "reading source permissions")
	}

	jujuDB := db.session.DB(jujuDBName)
	col := jujuDB.C("permissions")
	bulk := col.Bulk()
	for _, u := range data {
		id, ok := u["_id"].(string)
		if !ok {
			continue
		}
		if strings.HasPrefix(id, "ao#") {
			// We don't currently copy cross model artefacts.
			continue
		}
		if strings.HasPrefix(id, "cloud#") {
			bulk.Upsert(bson.M{"_id": u["_id"]}, bson.M{"$set": u})
			continue
		}
		if strings.HasPrefix(id, "c#") {
			if strings.HasSuffix(id, "#admin") {
				continue
			}
			object_key, ok := u["object-global-key"].(string)
			if !ok {
				continue
			}
			u["_id"] = strings.Replace(id, object_key, "c#"+controller.ControllerUUID, 1)
			u["object-global-key"] = "c#" + controller.ControllerUUID
			bulk.Upsert(bson.M{"_id": u["_id"]}, bson.M{"$set": u})
			bulk.Remove(bson.M{"_id": id})
		}
		if strings.HasPrefix(id, "e#") {
			if strings.HasSuffix(id, "#admin") {
				continue
			}
			object_key, ok := u["object-global-key"].(string)
			if !ok {
				continue
			}
			u["_id"] = strings.Replace(id, object_key, "e#"+controller.ControllerModelUUID, 1)
			u["object-global-key"] = "e#" + controller.ControllerModelUUID
			bulk.Upsert(bson.M{"_id": u["_id"]}, bson.M{"$set": u})
			bulk.Remove(bson.M{"_id": id})
		}
	}
	_, err = bulk.Run()
	if err != nil {
		return errors.Annotate(err, "writing permissions")
	}
	return nil
}

var controllerReadOnlyAttributes = set.NewStrings(
	"api-port",
	"ReadOnlyMethods",
	"state-port",
	"ca-cert",
	"charmstore-url",
	"controller-uuid",
	"identity-url",
	"identity-public-key",
	"set-numa-control-policy",
	"autocert-dns-name",
	"autocert-url",
	"allow-model-access",
	"juju-db-snap-channel",
	"max-txn-log-size",
	"caas-image-repo",
	"metering-url",
	"controller-api-port",
	"controller-name",
)

func (db *database) copySettings() error {
	const (
		controllers        = "controllers"
		controllerSettings = "controllerSettings"
	)
	var source settingsDoc
	jujuControllerDB := db.session.DB(jujuControllerDBName)
	sourceSettings := jujuControllerDB.C(controllers)
	err := sourceSettings.FindId(controllerSettings).One(&source)
	if err != nil {
		return errors.Annotate(err, "reading source settings")
	}

	var target settingsDoc
	jujuDB := db.session.DB(jujuDBName)
	targetSettings := jujuDB.C(controllers)
	err = targetSettings.FindId(controllerSettings).One(&target)
	if err != nil {
		return errors.Annotate(err, "reading target settings")
	}
	for attr, v := range source.Settings {
		// Retain controller name and ca-cert.
		if controllerReadOnlyAttributes.Contains(attr) {
			continue
		}
		target.Settings[attr] = v
	}

	err = targetSettings.UpdateId(controllerSettings, target)
	if err != nil {
		return errors.Annotate(err, "writing settings")
	}
	return nil
}

func (db *database) CopyController(controller core.ControllerInfo) error {
	logger.Debugf("copying controller data")

	err := db.copySettings()
	if err != nil {
		return errors.Annotate(err, "copying target settings")
	}

	err = db.copyCollection("users", "admin")
	if err != nil {
		return errors.Annotate(err, "updating target users")
	}
	err = db.copyCollection("controllerusers", "admin")
	if err != nil {
		return errors.Annotate(err, "copying target global users")
	}
	err = db.copyCollection("clouds", controller.ControllerModelCloud)
	if err != nil {
		return errors.Annotate(err, "copying target clouds")
	}
	err = db.copyCollection("cloudCredentials", controller.ControllerModelCloudCredential)
	if err != nil {
		return errors.Annotate(err, "copying target cloud credentials")
	}
	err = db.copyCollection("globalSettings", "")
	if err != nil {
		return errors.Annotate(err, "copying target cloud settings")
	}
	err = db.copyCollection("externalControllers", "")
	if err != nil {
		return errors.Annotate(err, "copying target external controllers")
	}
	err = db.copyCollection("secretBackends", "")
	if err != nil {
		return errors.Annotate(err, "copying target secret backends")
	}
	err = db.copyPermissions(controller)
	if err != nil {
		return errors.Annotate(err, "copying target permissions")
	}

	logger.Debugf("controller data copied, dropping staging database")
	err = db.session.DB(jujuControllerDBName).DropDatabase()
	if err != nil {
		return errors.Annotate(err, "dropping staging controller database")
	}
	return nil
}

const (
	restoreBinary     = "mongorestore"
	snapRestoreBinary = "juju-db.mongorestore"
	homeSnapDir       = "snap/juju-db/common" // relative to $HOME
)

func (db *database) buildRestoreArgs(dumpPath string, includeStatusHistory bool) []string {
	args := []string{
		"-vvvvv",
		"--drop",
		"--writeConcern=majority",
		"--host", db.info.Hostname,
		"--port", db.info.Port,
		"--authenticationDatabase=admin",
		"--username", db.info.Username,
		"--password", db.info.Password,
		"--ssl",
		"--sslAllowInvalidCertificates",
		"--stopOnError",
		"--maintainInsertionOrder",
		"--nsExclude=logs.*",
	}
	if !includeStatusHistory {
		args = append(args, "--nsExclude=juju.statuseshistory")
	}
	return append(args, dumpPath)
}

func (db *database) buildControllerRestoreArgs(dumpPath string) []string {
	args := []string{
		"-vvvvv",
		"--drop",
		"--writeConcern=majority",
		"--host", db.info.Hostname,
		"--port", db.info.Port,
		"--authenticationDatabase=admin",
		"--username", db.info.Username,
		"--password", db.info.Password,
		"--ssl",
		"--sslAllowInvalidCertificates",
		"--stopOnError",
		"--maintainInsertionOrder",
		"--nsFrom=juju.*",
		"--nsTo=jujucontroller.*",
		"--nsInclude=juju.controllers",
		"--nsInclude=juju.users",
		"--nsInclude=juju.controllerusers",
		"--nsInclude=juju.clouds",
		"--nsInclude=juju.cloudCredentials",
		"--nsInclude=juju.globalSettings",
		"--nsInclude=juju.permissions",
		"--nsInclude=juju.externalControllers",
		"--nsInclude=juju.secretBackends",
	}
	return append(args, dumpPath)
}

// RestoreFromDump uses mongorestore to load the dump from a backup.
func (db *database) RestoreFromDump(dumpDir, logFile string, includeStatusHistory, copyController bool) error {
	binary, isSnap, err := db.getRestoreBinary()
	if err != nil {
		return errors.Trace(err)
	}

	// Snap mongorestore can only access certain directories, so move the dump
	// from /tmp to under $HOME/snap before running restore, and delete after.
	if isSnap {
		dumpDir, err = db.moveToHomeSnap(dumpDir)
		if err != nil {
			return errors.Trace(err)
		}
		defer func() {
			err := os.RemoveAll(dumpDir)
			if err != nil {
				logger.Warningf("error removing snap dump dir: %v", err)
			}
		}()
	}

	command := exec.Command(
		binary,
		db.buildRestoreArgs(dumpDir, includeStatusHistory)...,
	)
	// If we are copying a controller, we restore a subset of the collections
	// to a staging database and later copy the relevant data.
	if copyController {
		command = exec.Command(
			binary,
			db.buildControllerRestoreArgs(dumpDir)...,
		)
	}
	logger.Debugf("running restore command: %s", strings.Join(command.Args, " "))

	// Use CombinedOutput and then write the bytes ourselves instead of
	// passing a file for command.Stdout/Stderr -- this avoids a permissions
	// issue with the Snap mongorestore writing to the file.
	output, err := command.CombinedOutput()
	if err != nil {
		logger.Debugf("%s output:\n%s", binary, output)
		return errors.Annotatef(err, "running %s", binary)
	}
	err = ioutil.WriteFile(logFile, output, 0664)
	if err != nil {
		logger.Debugf("%s output:\n%s", binary, output)
		return errors.Annotatef(err, "writing output to %s", logFile)
	}
	return nil
}

func (db *database) getRestoreBinary() (binary string, isSnap bool, err error) {
	if _, err := exec.LookPath(snapRestoreBinary); err == nil {
		return snapRestoreBinary, true, nil
	}
	if _, err := exec.LookPath(restoreBinary); err == nil {
		return restoreBinary, false, nil
	}
	return "", false, errors.Errorf("couldn't find %s or %s in PATH (%s)",
		snapRestoreBinary, restoreBinary, os.Getenv("PATH"))
}

func (db *database) moveToHomeSnap(dumpDir string) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", errors.Trace(err)
	}
	snapDumpDir := filepath.Join(homeDir, homeSnapDir, dumpDir)
	snapDumpParent, _ := filepath.Split(snapDumpDir)
	logger.Debugf("creating snap dump parent %q", snapDumpParent)
	err = os.MkdirAll(snapDumpParent, 0755)
	if err != nil {
		return "", errors.Annotate(err, "creating snap dump parent")
	}
	logger.Debugf("moving %q to snap dump dir %q", dumpDir, snapDumpDir)
	err = os.Rename(dumpDir, snapDumpDir)
	if err != nil {
		return "", errors.Annotate(err, "moving dump to snap dump dir")
	}
	return snapDumpDir, nil
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
