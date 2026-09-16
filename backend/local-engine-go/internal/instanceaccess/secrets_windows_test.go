package instanceaccess

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func TestWindowsPrivateDirectoryOwnership(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private")
	if err := createPrivateDirectory(path); err != nil {
		t.Fatal(err)
	}
	secrets, err := OpenSecrets(path)
	if err == nil {
		secrets.Close()
		return
	}
	sid, _ := currentSID()
	sd, sdErr := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if sdErr == nil {
		t.Logf("current owner SID=%s; descriptor=%s", sid, sd.String())
	} else {
		t.Logf("descriptor read: %v", sdErr)
	}
	for current := path; ; current = filepath.Dir(current) {
		info, statErr := os.Lstat(current)
		if statErr != nil {
			t.Logf("ancestor %s: %v", current, statErr)
		} else {
			t.Logf("ancestor %s mode=%v", current, info.Mode())
		}
		if filepath.Dir(current) == current {
			break
		}
	}
	t.Fatal("new private directory rejected", err)
}

func TestSecretsRefuseBroadWindowsACLWithoutRepair(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secrets")
	secrets, err := OpenSecrets(path)
	if err != nil {
		t.Fatal(err)
	}
	secrets.Close()
	sid, err := currentSID()
	if err != nil {
		t.Fatal(err)
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;" + sid + ")(A;OICI;FR;;;WD)")
	if err != nil {
		t.Fatal(err)
	}
	acl, _, err := sd.DACL()
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil); err != nil {
		t.Fatal(err)
	}
	before, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := OpenSecrets(path); err != ErrInvalid {
		t.Fatal("broad inherited ACL accepted", err)
	}
	after, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	if after.String() != before.String() {
		t.Fatal("invalid ACL silently repaired")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := checkPrivatePath(filepath.Join(path, "missing"), info); err != ErrUnavailable {
		t.Fatal(err)
	}
	if err := createPrivateDirectory(path + "\x00"); err != ErrInvalid {
		t.Fatal(err)
	}
	if err := ownPrivateFile(filepath.Join(path, "missing")); err != ErrUnavailable {
		t.Fatal("ownership failure must prevent secret publication", err)
	}
	for _, dacl := range []string{"D:P(A;OICI;FR;;;" + sid + ")", "D:P(A;OICIIO;FA;;;" + sid + ")", "D:P(A;OICI;FA;;;WD)"} {
		descriptor, err := windows.SecurityDescriptorFromString(dacl)
		if err != nil {
			t.Fatal(err)
		}
		acl, _, err := descriptor.DACL()
		if err != nil {
			t.Fatal(err)
		}
		if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil); err != nil {
			t.Fatal(err)
		}
		if _, err := OpenSecrets(path); err != ErrInvalid {
			t.Fatal("non-private ACL admitted", err)
		}
	}
}
