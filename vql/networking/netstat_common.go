package networking

import "time"

// Addr is implemented compatibility to psutil
type Addr struct {
	IP   string
	Port uint32
}

type ConnectionStat struct {
	Fd        uint32
	Family    uint32
	Type      uint32
	Laddr     Addr
	Raddr     Addr
	Status    string
	Pid       int32
	timestamp time.Time
}

type NetstatArgs struct{}
