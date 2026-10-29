//go:build windows

package authsession

import (
	"context"
	"errors"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	credTypeGeneric         = 1
	credPersistLocalMachine = 2
)

var (
	advapi32       = windows.NewLazySystemDLL("advapi32.dll")
	procCredRead   = advapi32.NewProc("CredReadW")
	procCredWrite  = advapi32.NewProc("CredWriteW")
	procCredDelete = advapi32.NewProc("CredDeleteW")
	procCredFree   = advapi32.NewProc("CredFree")

	windowsCredRead = func(target *uint16, typ uint32, flags uint32, cred **winCredential) error {
		ret, _, callErr := procCredRead.Call(
			uintptr(unsafe.Pointer(target)),
			uintptr(typ),
			uintptr(flags),
			uintptr(unsafe.Pointer(cred)),
		)
		if ret == 0 {
			return callErr
		}
		return nil
	}
	windowsCredWrite = func(cred *winCredential) error {
		ret, _, callErr := procCredWrite.Call(uintptr(unsafe.Pointer(cred)), 0)
		if ret == 0 {
			return callErr
		}
		return nil
	}
	windowsCredDelete = func(target *uint16, typ uint32, flags uint32) error {
		ret, _, callErr := procCredDelete.Call(uintptr(unsafe.Pointer(target)), uintptr(typ), uintptr(flags))
		if ret == 0 {
			return callErr
		}
		return nil
	}
	windowsCredFree = func(cred *winCredential) {
		procCredFree.Call(uintptr(unsafe.Pointer(cred)))
	}
)

type winCredential struct {
	Flags              uint32
	Type               uint32
	TargetName         *uint16
	Comment            *uint16
	LastWritten        windows.Filetime
	CredentialBlobSize uint32
	CredentialBlob     *byte
	Persist            uint32
	AttributeCount     uint32
	Attributes         uintptr
	TargetAlias        *uint16
	UserName           *uint16
}

// SystemCredentialStore stores session JSON in Windows Credential Manager.
type SystemCredentialStore struct{}

func NewSystemCredentialStore() CredentialStore {
	return SystemCredentialStore{}
}

func (SystemCredentialStore) Get(_ context.Context, key CredentialKey) (Session, bool, error) {
	// The target is a constant prefix plus a hex SHA-256 digest; it cannot contain NUL.
	target, _ := windows.UTF16PtrFromString(credentialService + ":" + key.stableName())
	var cred *winCredential
	if err := windowsCredRead(target, credTypeGeneric, 0, &cred); err != nil {
		if errors.Is(err, windows.ERROR_NOT_FOUND) {
			return Session{}, false, nil
		}
		return Session{}, false, err
	}
	defer windowsCredFree(cred)
	if cred.CredentialBlobSize == 0 || cred.CredentialBlob == nil {
		return Session{}, false, nil
	}
	data := unsafe.Slice(cred.CredentialBlob, cred.CredentialBlobSize)
	session, err := decodeSession(string(data))
	if err != nil {
		return Session{}, false, err
	}
	return session, true, nil
}

func (SystemCredentialStore) Put(_ context.Context, key CredentialKey, session Session) error {
	// stableName contains hex digits only, so UTF-16 conversion cannot fail.
	target, _ := windows.UTF16PtrFromString(credentialService + ":" + key.stableName())
	account, err := windows.UTF16PtrFromString(key.displayAccount())
	if err != nil {
		return err
	}
	encoded := encodeSession(session)
	blob := []byte(encoded)
	cred := winCredential{
		Type:               credTypeGeneric,
		TargetName:         target,
		CredentialBlobSize: uint32(len(blob)),
		Persist:            credPersistLocalMachine,
		UserName:           account,
	}
	if len(blob) > 0 {
		cred.CredentialBlob = &blob[0]
	}
	return windowsCredWrite(&cred)
}

func (SystemCredentialStore) Delete(_ context.Context, key CredentialKey) error {
	// stableName contains hex digits only, so UTF-16 conversion cannot fail.
	target, _ := windows.UTF16PtrFromString(credentialService + ":" + key.stableName())
	if err := windowsCredDelete(target, credTypeGeneric, 0); err != nil && !errors.Is(err, windows.ERROR_NOT_FOUND) {
		return err
	}
	return nil
}
