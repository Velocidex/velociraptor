package json

import (
	"fmt"
	"os"
	"sync/atomic"
)

func Debug(v interface{}) {
	fmt.Println(StringIndent(v))
}

func Dump(v interface{}) {
	fmt.Println(StringIndent(v))
}

var g_idx uint64

// Write each message into its own file.
func TraceMessage(filename string, message interface{}) {
	idx := atomic.AddUint64(&g_idx, 1)
	file, err := os.OpenFile(fmt.Sprintf("%s_%v.json", filename, idx),
		os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0700)
	if err != nil {
		panic(err)
	}
	_, _ = file.Write(MustMarshalIndent(message))
	file.Close()
}
