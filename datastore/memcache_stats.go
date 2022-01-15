package datastore

type MemcacheStats struct {
	DataItemCount int
	DataItemSize  int
	DirItemCount  int
	DirItemSize   int
}

type MemcacheStater interface {
	Stats() *MemcacheStats
}
