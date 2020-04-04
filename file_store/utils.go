package file_store

type WriterAdapter struct {
	FileWriter
}

func (self *WriterAdapter) Write(data []byte) (int, error) {
	err := self.Append(data)
	return len(data), err
}
