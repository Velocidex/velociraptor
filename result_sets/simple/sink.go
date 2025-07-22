package simple

import "github.com/Velocidex/ordereddict"

type NullResultSetWriter struct{}

func (self NullResultSetWriter) WriteJSONL(
	serialized []byte, total_rows uint64) {
}

func (self NullResultSetWriter) WriteCompressedJSONL(
	serialized []byte, byte_offset uint64, uncompressed_size int,
	total_rows uint64) {
}

func (self NullResultSetWriter) Update(index uint64, row *ordereddict.Dict) error {
	return nil
}

func (self NullResultSetWriter) SetStartRow(int64) error     { return nil }
func (self NullResultSetWriter) Write(row *ordereddict.Dict) {}
func (self NullResultSetWriter) Flush()                      {}
func (self NullResultSetWriter) Close()                      {}
func (self NullResultSetWriter) SetCompletion(f func())      {}
func (self NullResultSetWriter) SetSync()                    {}
