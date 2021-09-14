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

linux:
	go run make.go -v linux

freebsd:
	go run make.go -v freebsd

windows:
	go run make.go -v windowsDev

windowsx86:
	go run make.go -v windowsx86

clean:
	go run make.go -v clean

generate:
	go generate ./vql/windows/
	go generate ./api/mock/

check:
	staticcheck ./...

build_docker:
	echo Building the initial docker container.
	docker build --tag velo_builder docker

build_release: build_docker
	echo Building release into output directory.
	docker run --rm -v `pwd`:/build/ -u `id -u`:`id -g` -e HOME=/tmp/  velo_builder

debug:
	dlv debug --wd=. --build-flags="-tags 'server_vql extras'" ./bin/ -- frontend --disable-panic-guard -v --debug

debug_client:
	dlv debug --build-flags="-tags 'server_vql extras'" ./bin/ -- client -v

debug_golden:
	dlv debug --build-flags="-tags 'server_vql extras'" ./bin/ -- --config artifacts/testdata/windows/test.config.yaml golden artifacts/testdata/server/testcases/ --env srcDir=`pwd` --disable_alarm --filter=${GOLDEN}

lint:
	golangci-lint run

KapeFilesSync:
	python3 scripts/kape_files.py ~/projects/KapeFiles/ > artifacts/definitions/Windows/KapeFiles/Targets.yaml
