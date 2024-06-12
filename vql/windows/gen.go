package windows

//go:generate go run ../../tools/mkwinsyscall.go -output zwin32_windows_amd64.go win32_windows.go
//go:generate go run ../../tools/mkwinsyscall.go -output zwin32_windows_arm64.go win32_windows.go
//go:generate go run ../../tools/mkwinsyscall.go -output zwin32_windows_386.go win32_windows_32.go
