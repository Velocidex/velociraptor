package server_artifacts

import (
	"context"
	"time"

	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/utils"
)

func ResultSetFlusher(ctx context.Context, rs_writer result_sets.ResultSetWriter) func() {
	sub_ctx, cancel := context.WithCancel(ctx)
	go func() {
		for {
			select {
			case <-sub_ctx.Done():
				return

			case <-time.After(utils.Jitter(time.Duration(10) * time.Second)):
				rs_writer.Flush()
			}
		}
	}()

	return cancel
}
