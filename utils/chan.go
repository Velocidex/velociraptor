package utils

import (
	"context"

	"www.velocidex.com/golang/vfilter"
)

func SendToOutput(
	ctx context.Context,
	output_chan chan vfilter.Row,
	event interface{}) bool {
	select {
	case <-ctx.Done():
		return false
	case output_chan <- vfilter.Row(event):
		return true
	}
}
