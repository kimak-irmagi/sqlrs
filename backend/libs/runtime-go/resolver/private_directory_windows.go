//go:build windows

package resolver

import (
	"os"
	"syscall"
	"unsafe"
)

const (
	seFileObject             = 1
	ownerSecurityInformation = 0x00000001
	daclSecurityInformation  = 0x00000004
	accessAllowedACEType     = 0x00
	objectAllowedACEType     = 0x05
	writeLikeAccessMask      = 0x10000000 | 0x40000000 | 0x00080000 | 0x00040000 | 0x00010000 | 0x00000100 | 0x00000010 | 0x00000004 | 0x00000002
)

var (
	advapi32Security     = syscall.NewLazyDLL("advapi32.dll")
	getNamedSecurityInfo = advapi32Security.NewProc("GetNamedSecurityInfoW")
	getAce               = advapi32Security.NewProc("GetAce")
	equalSID             = advapi32Security.NewProc("EqualSid")
)

type windowsACL struct {
	Revision byte
	Sbz1     byte
	Size     uint16
	Count    uint16
	Sbz2     uint16
}

type windowsACEHeader struct {
	Type  byte
	Flags byte
	Size  uint16
}

type windowsAllowedACE struct {
	Header   windowsACEHeader
	Mask     uint32
	SIDStart uint32
}

// preparePrivateDirectoryPlatform validates the pre-established Windows ACL
// ownership boundary. It deliberately never creates a root whose inherited DACL
// has not been selected by the engine deployment.
func preparePrivateDirectoryPlatform(root string) error {
	if _, err := os.Lstat(root); err != nil {
		return err
	}
	return validateWindowsPrivateDirectory(root)
}

func validateWindowsPrivateDirectory(root string) error {
	path, err := syscall.UTF16PtrFromString(root)
	if err != nil {
		return err
	}
	var owner *syscall.SID
	var dacl *windowsACL
	var descriptor uintptr
	result, _, _ := getNamedSecurityInfo.Call(
		uintptr(unsafe.Pointer(path)), seFileObject,
		ownerSecurityInformation|daclSecurityInformation,
		uintptr(unsafe.Pointer(&owner)), 0, uintptr(unsafe.Pointer(&dacl)), 0,
		uintptr(unsafe.Pointer(&descriptor)),
	)
	if result != 0 {
		return syscall.Errno(result)
	}
	defer syscall.LocalFree(syscall.Handle(descriptor))
	if owner == nil || dacl == nil {
		return ErrUnsafePath
	}
	token, err := syscall.OpenCurrentProcessToken()
	if err != nil {
		return err
	}
	defer token.Close()
	user, err := token.GetTokenUser()
	if err != nil {
		return err
	}
	if !sameWindowsSID(owner, user.User.Sid) {
		return ErrUnsafePath
	}
	broad := broadWindowsSIDs()
	for index := uint16(0); index < dacl.Count; index++ {
		var ace *windowsAllowedACE
		ok, _, callErr := getAce.Call(uintptr(unsafe.Pointer(dacl)), uintptr(index), uintptr(unsafe.Pointer(&ace)))
		if ok == 0 {
			if callErr != nil && callErr != syscall.Errno(0) {
				return callErr
			}
			return ErrUnsafePath
		}
		if ace == nil || (ace.Header.Type != accessAllowedACEType && ace.Header.Type != objectAllowedACEType) || ace.Mask&writeLikeAccessMask == 0 {
			continue
		}
		// ACCESS_ALLOWED_OBJECT_ACE has optional GUIDs before its SID. Rejecting a
		// broad write-capable object ACE is safer than attempting a partial parse.
		if ace.Header.Type == objectAllowedACEType {
			return ErrUnsafePath
		}
		sid := (*syscall.SID)(unsafe.Pointer(&ace.SIDStart))
		for _, candidate := range broad {
			if sameWindowsSID(sid, candidate) {
				return ErrUnsafePath
			}
		}
	}
	return nil
}

func sameWindowsSID(left, right *syscall.SID) bool {
	if left == nil || right == nil {
		return false
	}
	result, _, _ := equalSID.Call(uintptr(unsafe.Pointer(left)), uintptr(unsafe.Pointer(right)))
	return result != 0
}

func broadWindowsSIDs() []*syscall.SID {
	values := []string{"S-1-1-0", "S-1-5-11", "S-1-5-32-545"}
	result := make([]*syscall.SID, 0, len(values))
	for _, value := range values {
		// These are fixed, well-known SID literals rather than external input.
		sid, _ := syscall.StringToSid(value)
		result = append(result, sid)
	}
	return result
}
