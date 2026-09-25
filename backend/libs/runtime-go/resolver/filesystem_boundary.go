package resolver

import (
	"os"
	"reflect"
)

// filesystemTransition reports a cross-filesystem component when the platform
// exposes a native device identifier. Platforms without one remain conservative
// and rely on rooted traversal plus reparse-point rejection.
func filesystemTransition(root, current os.FileInfo) bool {
	rootDevice, rootOK := filesystemDevice(root)
	currentDevice, currentOK := filesystemDevice(current)
	return rootOK && currentOK && rootDevice != currentDevice
}

func filesystemDevice(info os.FileInfo) (uint64, bool) {
	if info == nil || info.Sys() == nil {
		return 0, false
	}
	value := reflect.ValueOf(info.Sys())
	if value.Kind() == reflect.Pointer {
		value = value.Elem()
	}
	if !value.IsValid() || value.Kind() != reflect.Struct {
		return 0, false
	}
	field := value.FieldByName("Dev")
	if !field.IsValid() {
		return 0, false
	}
	switch field.Kind() {
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return field.Uint(), true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return uint64(field.Int()), true
	default:
		return 0, false
	}
}
