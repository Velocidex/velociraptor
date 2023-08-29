module www.velocidex.com/golang/config_check

go 1.18

require (
	github.com/Velocidex/yaml/v2 v2.2.8
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc
	gopkg.in/alecthomas/kingpin.v2 v2.2.6
	www.velocidex.com/golang/velociraptor v0.6.7-rc1.0.20221109004914-52bbd57d7c91
)

require (
	github.com/alecthomas/template v0.0.0-20190718012654-fb15b899a751 // indirect
	github.com/alecthomas/units v0.0.0-20211218093645-b94a6e3cc137 // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	google.golang.org/protobuf v1.30.0 // indirect
)

replace www.velocidex.com/golang/velociraptor => /home/mic/projects/velociraptor
