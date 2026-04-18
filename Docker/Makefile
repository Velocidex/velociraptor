build:
	docker build -t velociraptor-server .

run: build
	docker run \
		-p 127.0.0.1:8000:8000 \
		-p 127.0.0.1:8889:8889 \
		--mount type=bind,source=./etc/,target=/etc/velociraptor \
		--mount type=bind,source=./datastore/,target=/datastore/ \
		--name velociraptor-server \
		velociraptor-server:latest

kill:
	docker kill velociraptor-server; docker rm velociraptor-server

clean_datastore:
	rm -rf ./datastore/*
