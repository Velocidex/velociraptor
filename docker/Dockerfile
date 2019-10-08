FROM ubuntu:18.04

MAINTAINER Velocidex Innovations <support@velocidex.com>

RUN \
  apt-get update && \
  apt-get install -y automake autogen build-essential ca-certificates                    \
    gcc-mingw-w64 gcc-mingw-w64-x86-64 git pkg-config patch  \
    make xz-utils cpio wget zip unzip p7zip  \
    --no-install-recommends

# Configure the Go environment, since it's not going to change
ENV PATH   /usr/local/go/bin:/tmp/go/bin:$PATH
ENV GOPATH /tmp/go
ENV HOME /tmp/

# Inject the remote file fetcher and checksum verifier
ADD fetch.sh /fetch.sh
ENV FETCH /fetch.sh
RUN chmod +x $FETCH

# Inject the new Go root distribution downloader and bootstrapper
ADD bootstrap_pure.sh /bootstrap_pure.sh
ENV BOOTSTRAP_PURE /bootstrap_pure.sh
RUN chmod +x $BOOTSTRAP_PURE

# Configure the root Go distribution and bootstrap based on it
ENV GO_VERSION 1131

RUN \
  export ROOT_DIST=https://dl.google.com/go/go1.13.1.linux-amd64.tar.gz && \
  export ROOT_DIST_SHA=94f874037b82ea5353f4061e543681a0e79657f787437974214629af8407d124 && \
  export NODE_DIST=https://nodejs.org/dist/v12.11.1/node-v12.11.1-linux-x64.tar.gz && \
  export NODE_DIST_SHA=ac6c76af7c13cc3688aba072c4c728cb6fa2c40b340b1dcc4795e2705b1869dc && \
  $BOOTSTRAP_PURE

# Inject the container entry point, the build script
ADD build.sh /build.sh
ENV BUILD /build.sh
RUN chmod +x $BUILD

ENTRYPOINT ["/build.sh"]
