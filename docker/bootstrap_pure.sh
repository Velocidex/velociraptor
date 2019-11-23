#!/bin/bash
#
# Contains the Go tool-chain pure-Go bootstrapper, that as of Go 1.5, initiates
# not only a few pre-built Go cross compilers, but rather bootstraps all of the
# supported platforms from the origin Linux amd64 distribution.
#
# Usage: bootstrap_pure.sh
#
# Environment variables for remote bootstrapping:
#   FETCH         - Remote file fetcher and checksum verifier (injected by image)
#   ROOT_DIST     - 64 bit Linux Go binary distribution package
#   ROOT_DIST_SHA - 64 bit Linux Go distribution package checksum
#
# Environment variables for local bootstrapping:
#   GOROOT - Path to the lready installed Go runtime
set -e

# Download, verify and install the root distribution if pulled remotely
if [ "$GOROOT" == "" ]; then
  $FETCH $ROOT_DIST $ROOT_DIST_SHA

  tar -C /usr/local -xzf `basename $ROOT_DIST`
  rm -f `basename $ROOT_DIST`

  $FETCH $NODE_DIST $NODE_DIST_SHA

  tar -C /usr/local --strip-components=1 -xzf `basename $NODE_DIST`
  rm -f `basename $NODE_DIST`

  export GOROOT=/usr/local/go
fi
export GOROOT_BOOTSTRAP=$GOROOT

# Pre-build all guest distributions based on the root distribution
echo "Bootstrapping windows/amd64..."
GOOS=windows GOARCH=amd64 CGO_ENABLED=1 CC=x86_64-w64-mingw32-gcc go install std

echo "Bootstrapping windows/386..."
GOOS=windows GOARCH=386 CGO_ENABLED=1 CC=i686-w64-mingw32-gcc go install std
