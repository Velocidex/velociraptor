test:
	go test ./...

# Build templates for all supported operating systems.
build:
	GOOS=linux GOARCH=amd64 go build -o debian/velociraptor/usr/lib/velociraptor/velociraptor ./bin/
	zip -r templates/velociraptor_linux_amd64.zip debian/
