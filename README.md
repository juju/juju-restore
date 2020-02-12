# juju-restore

This is a tool to restore a Juju backup file into a Juju
controller. It should be run on the primary controller machine in the
MongoDB replica set. All replica set nodes need to be healthy, in
PRIMARY or SECONDARY state.

The expected usage is to copy juju-restore and the backup file to the
primary controller machine and then run it:

    ./juju-restore --password mongo-password /path/to/backup/file

The other connection options (username, hostname, port and ssl) have
defaults that should be correct unless there is some unusual
configuration for this MongoDB instance.

For additional logging, run with `--verbose`.

## Current status

This is in early development and doesn't yet restore backups. The
first milestone will be to support restoring a backup from the same
controller and Juju version. Next is restoring a backup from the same
controller and an earlier Juju version (to enable rolling back
upgrades). After that is the disaster recovery scenario of restoring a
backup into a new controller.
