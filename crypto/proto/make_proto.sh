#!/bin/bash
# Used to regenrate proto bindings. This script should be run if any
# of the .proto files are modified.

protoc -I /usr/local/include/ -I. --go_out=. *.proto
