# juju-restore

This is a tool to restore a Juju backup file into a Juju
controller. It should be run on the primary controller machine in the
MongoDB replica set. All replica set nodes need to be healthy, in
PRIMARY or SECONDARY state.

The expected usage is to copy the juju-restore binary and the backup
file to the primary controller machine and then run it:

    ./juju-restore /path/to/backup/file

Username and password will be collected automatically from the machine
agent's config file: `/var/lib/juju/agents/machine-<n>/agent.conf`
They can be specified manually with the `--username`/`--password`
options if needed.

By default, a backup taken from an earlier Juju version can't be
restored to prevent downgrading the controller accidentally. If this
is needed (to back out an upgrade that's hitting an error of some kind
for example) pass the `--allow-downgrade` option to override the
version check. (Restoring a backup from a future version of Juju is
still forbidden.)

The other connection options (hostname, port and ssl) have defaults
that should be correct unless there is some unusual configuration for
this MongoDB instance.

For additional logging, run with `--verbose`.

## Current status

This is in development. At the moment it only supports restoring a
backup to the same controller, so it can't be used in a disaster
scenario where all the controller machines are lost. It's still
experimental at this stage and shouldn't be relied on in
production. That said: if you have a staging controller that you can
experiment on, and you'd be alright with rebuilding it in the case of
catastrophic failure, it would really help us to find bugs if you'd
try it out and let the Juju team know.

(We haven't had any controller-destroying failures like that in our
testing so far.)

The next piece of work is to support the disaster recovery scenario of
restoring a backup into a new controller.

## Contacting us

You can post issues here on github, post comments on [our
forum](https://discourse.jujucharms.com/), or talk to us in #juju on
[freenode](https://freenode.net/).
