all:
	go run make.go -v autoDev

auto:
	go run make.go -v auto

test:
	go test -race -v --tags server_vql ./...

golden:
	./output/velociraptor -v --config artifacts/testdata/windows/test.config.yaml golden artifacts/testdata/server/testcases/ --env srcDir=`pwd` --filter=${GOLDEN}

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

linux_musl:
	go run make.go -v LinuxMusl

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

clean:
	go run make.go -v clean

generate:
	go generate ./vql/windows/
	go generate ./api/mock/

check:
	staticcheck ./...

debug:
	dlv debug --wd=. --build-flags="-tags 'server_vql extras'" ./bin/ -- frontend --disable-panic-guard -v --debug

debug_client:
	dlv debug --build-flags="-tags 'server_vql extras'" ./bin/ -- client -v

debug_golden:
	dlv debug --build-flags="-tags 'server_vql extras'" ./bin/ -- --config artifacts/testdata/windows/test.config.yaml golden artifacts/testdata/server/testcases/ --env srcDir=`pwd` --disable_alarm -v --filter=${GOLDEN}

lint:
	golangci-lint run

KapeFilesSync:
	python3 scripts/kape_files.py -t win ~/projects/KapeFiles/ > artifacts/definitions/Windows/KapeFiles/Targets.yaml
	python3 scripts/kape_files.py -t nix ~/projects/KapeFiles/ > artifacts/definitions/Linux/KapeFiles/CollectFromDirectory.yaml

SQLECmdSync:
	python3 scripts/sqlecmd_convert.py ~/projects/SQLECmd/ ~/projects/KapeFiles/ artifacts/definitions/Generic/Collectors/SQLECmd.yaml

# Do this after fetching the build artifacts with `gh run download <RunID>`
UpdateCIArtifacts:
	mv artifact/server/* artifacts/testdata/server/testcases/
	mv artifact/windows/* artifacts/testdata/windows/

UpdateCerts:
	cp /etc/ssl/certs/ca-certificates.crt crypto/ca-certificates.crt
	fileb0x crypto/b0x.yaml

# Use this to propare artifact packs at specific versions:
# First git checkout origin/v0.6.3
archive_artifacts:
	zip -r release_artifacts_$(basename "$(git status | head -1)").zip artifacts/definitions/ -i \*.yaml
