package common

import (
	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/accessors"
)

type YaraHit struct {
	Name    string
	Offset  uint64
	HexData []string
	Data    []byte
}

type YaraResult struct {
	Rule     string
	Meta     *ordereddict.Dict
	Tags     []string
	String   *YaraHit
	File     accessors.FileInfo
	FileName *accessors.OSPath
}
