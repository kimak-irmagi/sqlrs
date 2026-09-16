package instanceaccess

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

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
}
