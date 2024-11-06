package debug

import (
	"context"
	"strings"
	"sync"

	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/vfilter"
)

type ProfileWriter func(
	ctx context.Context, scope vfilter.Scope,
	output_chan chan vfilter.Row)

type ProfileWriterInfo struct {
	Name, Description string
	ProfileWriter     ProfileWriter
}

var (
	mu       sync.Mutex
	handlers []ProfileWriterInfo
)

func RegisterProfileWriter(writer ProfileWriterInfo) {
	mu.Lock()
	defer mu.Unlock()

	handlers = append(handlers, writer)
}

func GetProfileWriters() (result []ProfileWriterInfo) {
	mu.Lock()
	defer mu.Unlock()

	for _, i := range handlers {
		result = append(result, i)
	}

	return result
}

func DumpWriter(
	ctx context.Context, scope vfilter.Scope, name string) string {
	var rows []string
	for _, w := range GetProfileWriters() {
		if w.Name == name {
			output_chan := make(chan vfilter.Row)

			go func() {
				defer close(output_chan)

				for r := range output_chan {
					line := json.MustMarshalString(r)
					if !strings.Contains(line, "Destroyed") {
						rows = append(rows, line)
					}
				}
			}()

			w.ProfileWriter(ctx, scope, output_chan)
		}
	}

	return strings.Join(rows, "\n")
}
