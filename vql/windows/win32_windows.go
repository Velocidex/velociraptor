package windows

//go:generate go run ../../tools/mksyscall_windows.go -output zwin32_windows.go win32_windows.go

import (
	"golang.org/x/sys/windows"
	"reflect"
	"unsafe"
)

func NewLazySystemDLL(name string) *windows.LazyDLL {
	return windows.NewLazySystemDLL(name)
}

type (
	LPVOID         uintptr
	DWORD          uint32
	LPBYTE         *byte
	PBYTE          *byte
	LPDWORD        *uint32
	LPWSTR         *uint16
	LPCWSTR        *uint16
	NET_API_STATUS DWORD

	USER_INFO_3 struct {
		Name             LPWSTR
		Password         LPWSTR
		Password_age     DWORD
		Priv             DWORD
		Home_dir         LPWSTR
		Comment          LPWSTR
		Flags            DWORD
		Script_path      LPWSTR
		Auth_flags       DWORD
		Full_name        LPWSTR
		Usr_comment      LPWSTR
		Parms            LPWSTR
		Workstations     LPWSTR
		Last_logon       DWORD
		Last_logoff      DWORD
		Acct_expires     DWORD
		Max_storage      DWORD
		Units_per_week   DWORD
		Logon_hours      PBYTE
		Bad_pw_count     DWORD
		Num_logons       DWORD
		Logon_server     LPWSTR
		Country_code     DWORD
		Code_page        DWORD
		User_id          DWORD
		Primary_group_id DWORD
		Profile          LPWSTR
		Home_dir_drive   LPWSTR
		Password_expired DWORD
	}
)

const (
	// from LMaccess.h

	USER_PRIV_GUEST = 0
	USER_PRIV_USER  = 1
	USER_PRIV_ADMIN = 2

	UF_SCRIPT                          = 0x0001
	UF_ACCOUNTDISABLE                  = 0x0002
	UF_HOMEDIR_REQUIRED                = 0x0008
	UF_LOCKOUT                         = 0x0010
	UF_PASSWD_NOTREQD                  = 0x0020
	UF_PASSWD_CANT_CHANGE              = 0x0040
	UF_ENCRYPTED_TEXT_PASSWORD_ALLOWED = 0x0080

	UF_TEMP_DUPLICATE_ACCOUNT    = 0x0100
	UF_NORMAL_ACCOUNT            = 0x0200
	UF_INTERDOMAIN_TRUST_ACCOUNT = 0x0800
	UF_WORKSTATION_TRUST_ACCOUNT = 0x1000
	UF_SERVER_TRUST_ACCOUNT      = 0x2000

	UF_ACCOUNT_TYPE_MASK = UF_TEMP_DUPLICATE_ACCOUNT |
		UF_NORMAL_ACCOUNT |
		UF_INTERDOMAIN_TRUST_ACCOUNT |
		UF_WORKSTATION_TRUST_ACCOUNT |
		UF_SERVER_TRUST_ACCOUNT

	UF_DONT_EXPIRE_PASSWD                     = 0x10000
	UF_MNS_LOGON_ACCOUNT                      = 0x20000
	UF_SMARTCARD_REQUIRED                     = 0x40000
	UF_TRUSTED_FOR_DELEGATION                 = 0x80000
	UF_NOT_DELEGATED                          = 0x100000
	UF_USE_DES_KEY_ONLY                       = 0x200000
	UF_DONT_REQUIRE_PREAUTH                   = 0x400000
	UF_PASSWORD_EXPIRED                       = 0x800000
	UF_TRUSTED_TO_AUTHENTICATE_FOR_DELEGATION = 0x1000000
	UF_NO_AUTH_DATA_REQUIRED                  = 0x2000000
	UF_PARTIAL_SECRETS_ACCOUNT                = 0x4000000
	UF_USE_AES_KEYS                           = 0x8000000

	UF_SETTABLE_BITS = UF_SCRIPT |
		UF_ACCOUNTDISABLE |
		UF_LOCKOUT |
		UF_HOMEDIR_REQUIRED |
		UF_PASSWD_NOTREQD |
		UF_PASSWD_CANT_CHANGE |
		UF_ACCOUNT_TYPE_MASK |
		UF_DONT_EXPIRE_PASSWD |
		UF_MNS_LOGON_ACCOUNT |
		UF_ENCRYPTED_TEXT_PASSWORD_ALLOWED |
		UF_SMARTCARD_REQUIRED |
		UF_TRUSTED_FOR_DELEGATION |
		UF_NOT_DELEGATED |
		UF_USE_DES_KEY_ONLY |
		UF_DONT_REQUIRE_PREAUTH |
		UF_PASSWORD_EXPIRED |
		UF_TRUSTED_TO_AUTHENTICATE_FOR_DELEGATION |
		UF_NO_AUTH_DATA_REQUIRED |
		UF_USE_AES_KEYS |
		UF_PARTIAL_SECRETS_ACCOUNT

	FILTER_TEMP_DUPLICATE_ACCOUNT    = uint32(0x0001)
	FILTER_NORMAL_ACCOUNT            = uint32(0x0002)
	FILTER_INTERDOMAIN_TRUST_ACCOUNT = uint32(0x0008)
	FILTER_WORKSTATION_TRUST_ACCOUNT = uint32(0x0010)
	FILTER_SERVER_TRUST_ACCOUNT      = uint32(0x0020)

	LG_INCLUDE_INDIRECT = (0x0001)

	ERROR_MORE_DATA = (234)
)

//sys NetApiBufferFree(Buffer uintptr) (status NET_API_STATUS) = netapi32.NetApiBufferFree
//sys NetUserEnum(servername *uint16, level uint32, filter uint32, bufptr *uintptr, prefmaxlen uint32, entriesread *uint32, totalentries *uint32, resume_handle *uint32) (status NET_API_STATUS) = netapi32.NetUserEnum
//sys NetUserGetGroups(servername *LPCWSTR, username *LPCWSTR, level DWORD, bufptr *LPBYTE, prefmaxlen DWORD, entriesread *LPDWORD, totalentries *LPDWORD) (status NET_API_STATUS) = netapi32.NetUserGetGroups

// Converts a pointer to a wide string to a regular go string. The
// underlying buffer may be freed afterwards by the Windows API.
func LPWSTRToString(ptr LPWSTR) string {
	p := (*[0xffff]uint16)(unsafe.Pointer(ptr))
	if p == nil {
		return ""
	}

	return windows.UTF16ToString(p[:])
}

// Convert a pointer to buffer and a length into a Go string. NOTE:
// This does not copy the buffer so it should not be kept around after
// the Windows API frees the underlying buffer.
func PointerToString(ptr uintptr, len int) string {
	var s string
	hdr := (*reflect.StringHeader)(unsafe.Pointer(&s))
	hdr.Data = uintptr(unsafe.Pointer(ptr))
	hdr.Len = len

	return s
}
