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

# Check if MINGW_CC cross compiler exists - if it does then enable CGO
# build. Not having the cross compiler will just disable VQL plugins
# which require CGO.
MINGW_CC := x86_64-w64-mingw32-gcc
MINGW_EXISTS := $(shell $(MINGW_CC) --help 2> /dev/null)
ifneq ("$(MINGW_EXISTS)", "")
	CC = "$(MINGW_CC)"
	CGO_ENABLED = 1
endif

# Just regular binaries for local testing. The GUI will be serving
# files from the filesystem.
build:
	GOOS=linux GOARCH=amd64 \
            go build \
            -tags devel \
            -ldflags "$(LDFLAGS)" \
	    -o output/velociraptor ./bin/

windows:
ifeq ("$(MINGW_EXISTS)", "")
	@echo Disabling cgo modules. To enable install $(MINGW_CC)
endif
	fileb0x artifacts/b0x.yaml
	GOOS=windows GOARCH=amd64 \
        CC=$(CC) CGO_ENABLED=$(CGO_ENABLED) \
            go build \
            -ldflags "$(LDFLAGS)" \
	    -o output/velociraptor.exe ./bin/

darwin:
	fileb0x gui/b0x.yaml artifacts/b0x.yaml
	GOOS=darwin GOARCH=amd64 \
            go build \
            -tags release \
            -ldflags "$(LDFLAGS)" \
	    -o output/velociraptor.darwin ./bin/

# Build release binaries. The GUI will embed assets and ship with
# everything in it.
release:
	fileb0x gui/b0x.yaml artifacts/b0x.yaml
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
	rm -f gui/assets/ab0x.go \
              artifacts/assets/ab0x.go


generate:
	go generate ./vql/windows/win32_windows.go
