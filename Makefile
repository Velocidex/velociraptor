all:
	go run make.go -v dev

test:
	go test ./...

release:
	go run make.go -v linux

windows:
	go run make.go -v windows

xgo:
	go run make.go -v xgo

clean:
	go run make.go -v clean

generate:
	go generate ./vql/windows/win32_windows.go
