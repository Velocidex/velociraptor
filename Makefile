all:
	go run make.go -v autoDev

assets:
	go run make.go -v assets

auto:
	go run make.go -v auto

test:
	go test -race -v --tags server_vql ./...

test_light:
	go test -v --tags server_vql ./...

golden:
	./output/velociraptor -v --config artifacts/testdata/windows/test.config.yaml golden artifacts/testdata/server/testcases/ --env srcDir=`pwd` --filter=${GOLDEN}

debug_golden:
	dlv debug --init ./scripts/dlv.init --build-flags="-tags 'server_vql extras'" ./bin/ -- --config artifacts/testdata/windows/test.config.yaml golden artifacts/testdata/server/testcases/ --env srcDir=`pwd` --disable_alarm -v --debug --filter=${GOLDEN}

references:
	./output/velociraptor vql export docs/references/vql.yaml > docs/references/vql.yaml.tmp
	mv docs/references/vql.yaml.tmp docs/references/vql.yaml

release:
	go run make.go -v release

# Basic darwin binary - no yara.
darwin:
	go run make.go -v DarwinBase

darwin_intel:
	go run make.go -v Darwin

darwin_m1:
	go run make.go -v DarwinM1

linux_m1:
	go run make.go -v LinuxM1

linux_arm64:
	go run make.go -v LinuxArm64

# For raspberi pi.
linux_armf:
	go run make.go -v LinuxArmhf

linux_musl:
	go run make.go -v LinuxMusl

linux_musl_debug:
	go run make.go -v LinuxMuslDebug

linux:
	go run make.go -v linux

linux_bare:
	go run make.go -v linuxBare

freebsd:
	go run make.go -v freebsd

windows:
	go run make.go -v windowsDev

windows_bare:
	go run make.go -v windowsBare

windowsx86:
	go run make.go -v windowsx86

windowsarm:
	go run make.go -v windowsarm

clean:
	go run make.go -v clean

generate:
	go generate ./vql/windows/
	go generate ./api/mock/

check:
	staticcheck ./...

debug:
	dlv debug --init ./scripts/dlv.init --wd=. --build-flags="-tags 'server_vql extras'" ./bin/ -- frontend --disable-panic-guard -v --debug

debug_minion:
	dlv debug --init ./scripts/dlv.init --wd=. --build-flags="-tags 'server_vql extras'" ./bin/ -- frontend --disable-panic-guard -v --debug --minion --node ${NODE}

debug_client:
	dlv debug --init ./scripts/dlv.init --build-flags="-tags 'server_vql extras'" ./bin/ -- client -v --debug --debug_port 6061

lint:
	golangci-lint run

# Do this after fetching the build artifacts with `gh run download <RunID>`
UpdateCIArtifacts:
	mv artifact/server/* artifacts/testdata/server/testcases/
	mv artifact/windows/* artifacts/testdata/windows/

UpdateCerts:
	cp /etc/ssl/certs/ca-certificates.crt crypto/ca-certificates.crt
	fileb0x crypto/b0x.yaml

# Use this to prepare artifact packs at specific versions:
# First git checkout origin/v0.6.3
archive_artifacts:
	zip -r release_artifacts_$(basename "$(git status | head -1)").zip artifacts/definitions/ -i \*.yaml

translations:
	python3 ./scripts/find_i8n_translations.py ./gui/velociraptor/src/components/i8n/

config_check:
	go run ./docs/references/sample_config/main.go ./docs/references/server.config.yaml

deadcode:
	go run make.go -v deadcode

api_check:
	python ./scripts/api_checker.py .
