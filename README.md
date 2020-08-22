# Velociraptor - Endpoint visibility and collection tool.

Velociraptor is a tool for collecting host based state information
using Velocidex Query Language (VQL) queries.

To learn more about Velociraptor, read the documentation on:

   https://www.velocidex.com/docs/

## Quick start

1. Download the binary from the release page.

2. You need to generate a server config file. This will generate new
   key material. Simply follow the prompts:

```bash
  $ velociraptor config generate -i
```

3. Start the server:

```bash
 $ velociraptor --config server.config.yaml frontend -v
```

4. Point a browser at the GUI port that you set in the config
   file. You should be able to log in with the password set earlier.

5. Launch the client on any system with the generated client config file.

```bash
 $ velociraptor --config client.conf.yaml client -v
```

6. You should be able to search for the client in the GUI, browse VFS,
   download files etc.

To deploy the windows executable:

1. Install the released MSI installer.

2. Drop the client configuration into `C:\Program Files\Velociraptor\Velociraptor.config.yaml`
using any system administration method (e.g. group policy, SCCM etc).

See more information for deployment options at
https://www.velocidex.com/docs/getting-started

## Running Velociraptor via Docker
To run a Velociraptor server via Docker, follow the instructions here:   
https://github.com/weslambert/velociraptor-docker

## Running Velociraptor locally

Velociraptor is also useful as a local triage tool. In particular you
might find Velociraptor's artifacts especially useful for quickly
capturing important information about a running system. You can
collect artifacts by using the "artifacts collect" command:

```bash
    $ velociraptor artifacts list
    INFO:2018/08/20 22:28:56 Loaded 18 built in artifacts
    INFO:2018/08/20 22:28:56 Loaded 18 artifacts from artifacts/definitions/
    Linux.Applications.Chrome.Extensions
    Linux.Applications.Chrome.Extensions.Upload
    Linux.Applications.Docker.Info
    Linux.Applications.Docker.Version
    Linux.Debian.AptSources

    $ velociraptor artifacts list -v Linux.Debian.AptSources
    .... displays the artifacts

    $ velociraptor artifacts collect -v Linux.Debian.AptSources --output myfile.zip
    ... Collects all the named artifacts into myfile.zip
```

Explore more of Velociraptor's options using the -h flag.

## Building from source

To build from source, make sure you have a recent Golang installed
from https://golang.org/dl/ (Currently at least Go 1.13):

```bash

    $ git clone https://github.com/Velocidex/velociraptor.git
    $ cd velociraptor

    # This will build the GUI elements. You will need to have node
    # installed first. For example on Windows get it from
    # https://nodejs.org/en/download/ . You also need to have JAVA
    # installed from https://www.java.com because the js compiler
    # needs it.
    $ cd gui/static/
    $ npm install

    # If gulp is not on your path you need to run it using node:
    # node node_modules\gulp\bin\gulp.js compile
    $ gulp compile
    $ cd -

    # This builds a release (i.e. it will embed the GUI files in the
    # binary). If you dont care about the GUI a simple "make" will
    # build a bare debug binary.
    $ go run make.go -v release
    $ go run make.go -v windows
```

If you want to rebuild the protobuf you will need to install protobuf
compiler (This is only necessary when editing any `*.proto` file):

```bash

   $ wget https://github.com/protocolbuffers/protobuf/releases/download/v3.8.0/protoc-3.8.0-linux-x86_64.zip
   $ unzip protoc-3.8.0-linux-x86_64.zip
   $ sudo mv include/google/ /usr/local/include/
   $ sudo mv bin/protoc /usr/local/bin/
   $ go get -u github.com/golang/protobuf/protoc-gen-go/
   $ go install github.com/golang/protobuf/protoc-gen-go/
   $ go get -u github.com/grpc-ecosystem/grpc-gateway/protoc-gen-grpc-gateway
   $ go install github.com/grpc-ecosystem/grpc-gateway/protoc-gen-grpc-gateway
   $ ./make_proto.sh

```

## Getting the latest version

We have a pretty frequent release schedule but if you see a new
feature submitted that you are really interested in, we would love to
have more testing prior to the official release.

We have a CI pipeline managed by GitHub actions. You can see the
pipeline by clicking the actions tab on our GitHub project. There are
two workflows:

1. Windows Test: this workflow build a minimal version of the
   Velociraptor binary (without the GUI) and runs all the tests on
   it. We also test various windows support functions in this
   pipeline. This pipeline builds on every push in each PR.

2. Linux Build All Arches: This pipeline builds complete binaries for
   many supported architectures. It only runs when the PR is merged
   into the master branch.

If you fork the project on GitHub, the pipelines will run on your own
fork as well. If you need to prepare a PR for a new feature or modify
an existing feature you can use this to build your own binaries for
testing on all architectures.


## Getting help

Questions and feedback are welcome at velociraptor-discuss@googlegroups.com

You can also chat with us directly on discord https://www.velocidex.com/discord

File issues on https://github.com/Velocidex/velociraptor

Read more about Velociraptor on our blog:

https://www.velocidex.com/blog/

Hang out on Medium https://medium.com/velociraptor-ir
