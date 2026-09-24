//go:build windows

package resolver

import (
	"encoding/binary"
	"os"
	"strconv"
	"strings"
	"syscall"
	"unsafe"
)

const fsctlReadFileUSNData = 0x000900eb

var getVolumeInformationByHandle = syscall.NewLazyDLL("kernel32.dll").NewProc("GetVolumeInformationByHandleW")
var readWindowsFileUSN = windowsFileUSN

func nativeContinuityEvidence(file *os.File, info os.FileInfo) continuityEvidence {
	var handleInfo syscall.ByHandleFileInformation
	if err := syscall.GetFileInformationByHandle(syscall.Handle(file.Fd()), &handleInfo); err != nil {
		return continuityEvidence{Class: "windows-unknown"}
	}
	if !strings.EqualFold(windowsFilesystemName(syscall.Handle(file.Fd())), "NTFS") {
		return continuityEvidence{Class: "windows-unknown", VolumeID: strconv.FormatUint(uint64(handleInfo.VolumeSerialNumber), 10), FileID: windowsFileID(handleInfo)}
	}
	// Request V2 explicitly so the 64-bit file reference and USN layout remain
	// stable across the supported NTFS Windows versions.
	changeToken, ok := readWindowsFileUSN(syscall.Handle(file.Fd()))
	if !ok {
		return continuityEvidence{Class: "windows-unknown", VolumeID: strconv.FormatUint(uint64(handleInfo.VolumeSerialNumber), 10), FileID: windowsFileID(handleInfo)}
	}
	return continuityEvidence{
		Class: "ntfs-usn", Revision: "ntfs-usn-v1",
		VolumeID:    strconv.FormatUint(uint64(handleInfo.VolumeSerialNumber), 10),
		FileID:      windowsFileID(handleInfo),
		ChangeToken: changeToken,
	}
}

func windowsFileUSN(handle syscall.Handle) (string, bool) {
	input := [4]byte{2, 0, 2, 0}
	output := make([]byte, 512)
	var returned uint32
	err := syscall.DeviceIoControl(handle, fsctlReadFileUSNData, &input[0], uint32(len(input)), &output[0], uint32(len(output)), &returned, nil)
	if err != nil || returned < 32 || binary.LittleEndian.Uint32(output[0:4]) < 32 || binary.LittleEndian.Uint16(output[4:6]) != 2 {
		return "", false
	}
	return strconv.FormatUint(binary.LittleEndian.Uint64(output[24:32]), 10), true
}

func windowsFilesystemName(handle syscall.Handle) string {
	buffer := make([]uint16, 32)
	result, _, _ := getVolumeInformationByHandle.Call(uintptr(handle), 0, 0, 0, 0, 0, uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)))
	if result == 0 {
		return ""
	}
	return syscall.UTF16ToString(buffer)
}

func windowsFileID(info syscall.ByHandleFileInformation) string {
	return strconv.FormatUint(uint64(info.FileIndexHigh)<<32|uint64(info.FileIndexLow), 10)
}
