//go:build darwin && cgo

package authsession

/*
#cgo LDFLAGS: -framework Security -framework CoreFoundation
#include <CoreFoundation/CoreFoundation.h>
#include <Security/Security.h>
#include <stdlib.h>
*/
import "C"

import (
	"context"
	"fmt"
	"unsafe"
)

// SystemCredentialStore uses macOS Keychain Services directly so refresh-token
// session data never appears in process arguments.
type SystemCredentialStore struct{}

var findDarwinGenericPasswordFn = findDarwinGenericPassword
var putDarwinGenericPasswordFn = putDarwinGenericPassword
var deleteDarwinGenericPasswordFn = deleteDarwinGenericPassword

func NewSystemCredentialStore() CredentialStore {
	return SystemCredentialStore{}
}

func (SystemCredentialStore) Get(_ context.Context, key CredentialKey) (Session, bool, error) {
	data, ok, err := findDarwinGenericPasswordFn(credentialService, key.displayAccount())
	if err != nil || !ok {
		return Session{}, ok, err
	}
	session, err := decodeSession(string(data))
	if err != nil {
		return Session{}, false, err
	}
	return session, true, nil
}

func (SystemCredentialStore) Put(_ context.Context, key CredentialKey, session Session) error {
	return putDarwinGenericPasswordFn(credentialService, key.displayAccount(), []byte(encodeSession(session)))
}

func (SystemCredentialStore) Delete(_ context.Context, key CredentialKey) error {
	return deleteDarwinGenericPasswordFn(credentialService, key.displayAccount())
}

func findDarwinGenericPassword(service, account string) ([]byte, bool, error) {
	cService := C.CString(service)
	defer C.free(unsafe.Pointer(cService))
	cAccount := C.CString(account)
	defer C.free(unsafe.Pointer(cAccount))

	var passwordLength C.UInt32
	var passwordData unsafe.Pointer
	var item C.SecKeychainItemRef
	status := C.SecKeychainFindGenericPassword(
		darwinDefaultKeychainOrArray(),
		C.UInt32(len(service)),
		cService,
		C.UInt32(len(account)),
		cAccount,
		&passwordLength,
		&passwordData,
		&item,
	)
	if status == C.errSecItemNotFound {
		return nil, false, nil
	}
	if status != C.errSecSuccess {
		return nil, false, darwinKeychainError("find generic password", status)
	}
	defer C.SecKeychainItemFreeContent(nil, passwordData)
	if item != darwinZeroKeychainItem() {
		defer C.CFRelease(C.CFTypeRef(item))
	}
	return append([]byte(nil), unsafe.Slice((*byte)(passwordData), int(passwordLength))...), true, nil
}

func putDarwinGenericPassword(service, account string, password []byte) error {
	if item, ok, err := findDarwinGenericPasswordItem(service, account); err != nil {
		return err
	} else if ok {
		defer C.CFRelease(C.CFTypeRef(item))
		return modifyDarwinGenericPasswordItem(item, password)
	}

	cService := C.CString(service)
	defer C.free(unsafe.Pointer(cService))
	cAccount := C.CString(account)
	defer C.free(unsafe.Pointer(cAccount))
	passwordData := C.CBytes(password)
	defer C.free(passwordData)

	status := C.SecKeychainAddGenericPassword(
		darwinDefaultKeychain(),
		C.UInt32(len(service)),
		cService,
		C.UInt32(len(account)),
		cAccount,
		C.UInt32(len(password)),
		passwordData,
		nil,
	)
	if status != C.errSecSuccess {
		return darwinKeychainError("add generic password", status)
	}
	return nil
}

func modifyDarwinGenericPasswordItem(item C.SecKeychainItemRef, password []byte) error {
	passwordData := C.CBytes(password)
	defer C.free(passwordData)
	status := C.SecKeychainItemModifyAttributesAndData(item, nil, C.UInt32(len(password)), passwordData)
	if status != C.errSecSuccess {
		return darwinKeychainError("update generic password", status)
	}
	return nil
}

func findDarwinGenericPasswordItem(service, account string) (C.SecKeychainItemRef, bool, error) {
	cService := C.CString(service)
	defer C.free(unsafe.Pointer(cService))
	cAccount := C.CString(account)
	defer C.free(unsafe.Pointer(cAccount))

	var item C.SecKeychainItemRef
	status := C.SecKeychainFindGenericPassword(
		darwinDefaultKeychainOrArray(),
		C.UInt32(len(service)),
		cService,
		C.UInt32(len(account)),
		cAccount,
		nil,
		nil,
		&item,
	)
	if status == C.errSecItemNotFound {
		return darwinZeroKeychainItem(), false, nil
	}
	if status != C.errSecSuccess {
		return darwinZeroKeychainItem(), false, darwinKeychainError("find generic password item", status)
	}
	return item, true, nil
}

func deleteDarwinGenericPassword(service, account string) error {
	item, ok, err := findDarwinGenericPasswordItem(service, account)
	if err != nil || !ok {
		return err
	}
	defer C.CFRelease(C.CFTypeRef(item))
	status := C.SecKeychainItemDelete(item)
	if status != C.errSecSuccess && status != C.errSecItemNotFound {
		return darwinKeychainError("delete generic password", status)
	}
	return nil
}

func darwinKeychainError(operation string, status C.OSStatus) error {
	return fmt.Errorf("macOS Keychain %s failed with status %d", operation, int32(status))
}

func darwinDefaultKeychain() C.SecKeychainRef {
	var keychain C.SecKeychainRef
	return keychain
}

func darwinDefaultKeychainOrArray() C.CFTypeRef {
	var keychainOrArray C.CFTypeRef
	return keychainOrArray
}

func darwinZeroKeychainItem() C.SecKeychainItemRef {
	var item C.SecKeychainItemRef
	return item
}
