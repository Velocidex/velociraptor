all:
	go run make.go -v autoDev

auto:
	go run make.go -v auto

test:
	go test ./...
	./output/velociraptor --config artifacts/testdata/windows/test.config.yaml \
	     golden artifacts/testdata/server/testcases/ --env srcDir=`pwd`

release:
	go run make.go -v release

windows:
	go run make.go -v windowsDev

clean:
	go run make.go -v clean

generate:
	go generate ./vql/windows/win32_windows.go

check:
	staticcheck ./...
