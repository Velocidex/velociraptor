# Velociraptor - Endpoint visibility and collection tool.

Velociraptor is a tool for collecting host based state information
using Velocidex Query Language (VQL) queries.

To learn more about Velociraptor, read the documentation on:

   https://www.velocidex.com/docs/

## Quick start

If you want to see what Velociraptor is all about simply:

1. Download the binary from the release page for your favorite platform (Windows/Linux/MacOS).

2. Start the GUI

```bash
  $ velociraptor gui
```

This will bring up the GUI, Frontend and a local client. You can
collect artifacts from the client (which is just running on your own
machine) as normal.

Once you are ready for a full deployment, check out the various deployment options at
https://www.velocidex.com/docs/getting-started

## Running Velociraptor via Docker

To run a Velociraptor server via Docker, follow the instructions here:
https://github.com/weslambert/velociraptor-docker

## Running Velociraptor locally

Velociraptor is also useful as a local triage tool. You can create a self contained local collector using the GUI:

1. Start the GUI as above (`velociraptor gui`).

2. Select the `Server Artifacts` sidebar menu, then `Build Collector`.

3. Select and configure the artifacts you want to collect tnen select
   the `Uploaded Files` tab and download your customized collector.

## Building from source

To build from source, make sure you have a recent Golang installed
from https://golang.org/dl/ (Currently at least Go 1.14):

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

   $ wget https://github.com/protocolbuffers/protobuf/releases/download/v3.13.0/protoc-3.13.0-linux-x86_64.zip
   $ unzip protoc-3.13.0-linux-x86_64.zip
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
fork as well as long as you enable GitHub Actions on your fork. If you
need to prepare a PR for a new feature or modify an existing feature
you can use this to build your own binaries for testing on all
architectures before send us the PR.

## Getting help

Questions and feedback are welcome at velociraptor-discuss@googlegroups.com

You can also chat with us directly on discord https://www.velocidex.com/discord

File issues on https://github.com/Velocidex/velociraptor

Read more about Velociraptor on our blog:

https://www.velocidex.com/blog/

Hang out on Medium https://medium.com/velociraptor-ir
