module github.com/juju/juju-restore

go 1.12

require (
	github.com/juju/ansiterm v0.0.0-20180109212912-720a0952cc2a
	github.com/juju/clock v0.0.0-20190205081909-9c5c9712527c // indirect
	github.com/juju/cmd v0.0.0-20200108104440-8e43f3faa5c9
	github.com/juju/errors v0.0.0-20190930114154-d42613fe1ab9
	github.com/juju/gnuflag v0.0.0-20171113085948-2ce1bb71843d
	github.com/juju/loggo v0.0.0-20190526231331-6e530bcce5d8
	github.com/juju/replicaset v0.0.0-20190321104350-501ab59799b1
	github.com/juju/retry v0.0.0-20180821225755-9058e192b216 // indirect
	github.com/juju/testing v0.0.0-20191001232224-ce9dec17d28b // indirect
	github.com/juju/utils v0.0.0-20200116185830-d40c2fe10647 // indirect
	github.com/juju/version v0.0.0-20191219164919-81c1be00b9a6 // indirect
	github.com/kr/pretty v0.2.0 // indirect
	github.com/lunixbochs/vtclean v1.0.0 // indirect
	github.com/mattn/go-colorable v0.1.4 // indirect
	github.com/mattn/go-isatty v0.0.12 // indirect
	golang.org/x/crypto v0.0.0-20200128174031-69ecbb4d6d5d // indirect
	golang.org/x/net v0.0.0-20200114155413-6afb5195e5aa // indirect
	gopkg.in/check.v1 v1.0.0-20190902080502-41f04d3bba15 // indirect
	gopkg.in/mgo.v2 v2.0.0-20190816093944-a6b53ec6cb22
	gopkg.in/yaml.v2 v2.2.8 // indirect
)

replace gopkg.in/mgo.v2 => github.com/juju/mgo v0.0.0-20190418114320-e9d4866cb7fc
