all: build

test:
	go test ./...

# Add some build time constants to make sure the binary knows about itself.
COMMIT := $(shell git rev-parse HEAD)
DATE := $(shell date -Iseconds)
LDFLAGS := \
   -s -w \
   -X www.velocidex.com/golang/velociraptor/config.build_time=$(DATE) \
   -X www.velocidex.com/golang/velociraptor/config.commit_hash=$(COMMIT)

# Just regular binaries for local testing. The GUI will be serving
# files from the filesystem.
build:
	GOOS=linux GOARCH=amd64 \
            go build \
            -ldflags "$(LDFLAGS)" \
	    -o output/velociraptor ./bin/
windows:
	GOOS=windows GOARCH=amd64 \
            go build \
            -ldflags "$(LDFLAGS)" \
	    -o output/velociraptor.exe ./bin/

# Build release binaries. The GUI will embed assets and ship with
# everything in it.
release:
	fileb0x gui/b0x.yaml
	GOOS=linux GOARCH=amd64 \
            go build \
            -ldflags "$(LDFLAGS)" \
            -tags release \
	    -o output/velociraptor ./bin/
	strip output/velociraptor

install: release
	install -D output/velociraptor \
                $(DESTDIR)$(prefix)/usr/bin/velociraptor

clean:
	rm -f gui/assets/ab0x.go
