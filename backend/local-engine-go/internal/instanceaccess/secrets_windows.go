package instanceaccess

import (
	"golang.org/x/sys/windows"
	"os"
	"unsafe"
)

func currentSID() (string, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return "", ErrUnavailable
	}
	return user.User.Sid.String(), nil
}

// Set the protected inheritable ACL at creation, before any credential exists.
func createPrivateDirectory(path string) error {
	sid, err := currentSID()
	if err != nil {
		return err
	}
	sd, err := windows.SecurityDescriptorFromString("O:" + sid + "D:P(A;OICI;FA;;;" + sid + ")")
	if err != nil {
		return ErrUnavailable
	}
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return ErrInvalid
	}
	attrs := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
	return windows.CreateDirectory(name, &attrs)
}

// Only newly created empty temporary files use this operation. Existing
// credentials and their permissions are validated, never silently repaired.
func ownPrivateFile(path string) error {
	sid, err := currentSID()
	if err != nil {
		return err
	}
	owner, err := windows.StringToSid(sid)
	if err != nil {
		return ErrUnavailable
	}
	if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION, owner, nil, nil, nil); err != nil {
		return ErrUnavailable
	}
	return nil
}

func checkPrivatePath(path string, _ os.FileInfo) error {
	sid, err := currentSID()
	if err != nil {
		return err
	}
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return ErrUnavailable
	}
	owner, _, err := sd.Owner()
	if err != nil || owner.String() != sid {
		return ErrInvalid
	}
	// Inspect the binary SID, not its SDDL spelling: Windows may abbreviate the
	// local administrator as LA even though TokenUser returns its full SID.
	acl, _, err := sd.DACL()
	if err != nil || acl == nil || acl.AceCount != 1 {
		return ErrInvalid
	}
	var ace *windows.ACCESS_ALLOWED_ACE
	if windows.GetAce(acl, 0, &ace) != nil || ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE || ace.Mask != windows.ACCESS_MASK(0x001f01ff) {
		return ErrInvalid
	}
	switch ace.Header.AceFlags {
	case windows.OBJECT_INHERIT_ACE | windows.CONTAINER_INHERIT_ACE,
		windows.OBJECT_INHERIT_ACE | windows.CONTAINER_INHERIT_ACE | windows.INHERITED_ACE,
		windows.INHERITED_ACE, 0:
	default:
		return ErrInvalid
	}
	allowed := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
	if !allowed.IsValid() || allowed.String() != sid {
		return ErrInvalid
	}
	return nil
}

func syncPrivateDirectory(_ secretRoot, path string) error {
	// FlushFileBuffers on a directory is not supported by Windows. Flush the
	// published file handles before linking; the NTFS link operation is atomic.
	// A missing record after a power loss fails closed during intent recovery.
	return nil
}
