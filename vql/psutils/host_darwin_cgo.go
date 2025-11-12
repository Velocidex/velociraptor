//go:build darwin && cgo
// +build darwin,cgo

package psutils

// #cgo LDFLAGS: -framework CoreFoundation -framework IOKit
// #include <CoreFoundation/CoreFoundation.h>
// #include <IOKit/IOKitLib.h>
//
// const char *
// getUUID()
// {
//     CFMutableDictionaryRef matching = IOServiceMatching("IOPlatformExpertDevice");
//     io_service_t service = IOServiceGetMatchingService(0, matching);
//     CFStringRef serialNumber = IORegistryEntryCreateCFProperty(service,
//         CFSTR("IOPlatformUUID"), kCFAllocatorDefault, 0);
//     const char *str = CFStringGetCStringPtr(serialNumber, kCFStringEncodingUTF8);
//     IOObjectRelease(service);
//
//     return str;
// }
import "C"

func HostID() string {
	return C.GoString(C.getUUID())
}
