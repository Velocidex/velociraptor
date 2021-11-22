package simple

import "github.com/Velocidex/ordereddict"

type NullResultSetWriter struct{}

func (self NullResultSetWriter) WriteJSONL(
	serialized []byte, total_rows uint64) {
}
func (self NullResultSetWriter) Write(row *ordereddict.Dict) {}
func (self NullResultSetWriter) Flush()                      {}
func (self NullResultSetWriter) Close()                      {}
func (self NullResultSetWriter) SetCompletion(f func())      {}
func (self NullResultSetWriter) SetSync()                    {}
