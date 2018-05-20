test:
	go test ./...

# Add some build time constants to make sure the binary knows about itself.
COMMIT := $(shell git rev-parse HEAD)
DATE := $(shell date -Iseconds)
LDFLAGS := \
   -X www.velocidex.com/golang/velociraptor/config.build_time=$(DATE) \
   -X www.velocidex.com/golang/velociraptor/config.commit_hash=$(COMMIT)

# Build templates for all supported operating systems.
build:
	GOOS=linux GOARCH=amd64 \
            go build \
            -ldflags "$(LDFLAGS)" \
	    -o debian/velociraptor/usr/lib/velociraptor/velociraptor ./bin/

	zip -r templates/velociraptor_linux_amd64.zip debian/
