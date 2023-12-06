package etw

type ETWOptions struct {
	AnyKeyword, AllKeyword uint64
	Level                  int64
	CaptureState           bool
	EnableMapInfo          bool

	// A description string to be associated with the registration.
	Description string
}
