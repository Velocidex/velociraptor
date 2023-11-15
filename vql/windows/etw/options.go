package etw

type ETWOptions struct {
	AnyKeyword, AllKeyword uint64
	Level                  int64
	CaptureState           bool
	EnableMapInfo          bool
}
