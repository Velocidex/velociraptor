all:
	go run make.go -v dev

test:
	go test ./...
	./output/velociraptor --config artifacts/testdata/windows/test.config.yaml \
	     golden artifacts/testdata/server/testcases/ --env srcDir=`pwd`

release:
	go run make.go -v linux

darwin:
	go run make.go -v darwin

windows:
	go run make.go -v windows

windows_race:
	go run make.go -v windowsRace

xgo:
	go run make.go -v xgo

xgo-linux:
	go run make.go -v xgolinux

clean:
	go run make.go -v clean

generate:
	go generate ./vql/windows/win32_windows.go

check:
	staticcheck ./...
