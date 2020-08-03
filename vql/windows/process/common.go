// +build windows,cgo

package process

type PidArgs struct {
	Pid int64 `vfilter:"required,field=pid,doc=The PID to dump out."`
}
