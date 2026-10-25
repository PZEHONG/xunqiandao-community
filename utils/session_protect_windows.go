//go:build windows

package utils

import (
	"golang.org/x/sys/windows"
	"unsafe"
)

// DPAPI binds decryption to the current Windows user on this machine.
// CRYPTPROTECT_UI_FORBIDDEN keeps background renewal non-interactive.
func protectSessionCredential(plain, entropy []byte, _ string) ([]byte, error) {
	in := windows.DataBlob{Size: uint32(len(plain)), Data: &plain[0]}
	salt := windows.DataBlob{Size: uint32(len(entropy)), Data: &entropy[0]}
	var out windows.DataBlob
	if err := windows.CryptProtectData(&in, nil, &salt, 0, nil, 1, &out); err != nil {
		return nil, err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return append([]byte(nil), unsafe.Slice(out.Data, int(out.Size))...), nil
}

func unprotectSessionCredential(sealed, entropy []byte, _ string) ([]byte, error) {
	if len(sealed) == 0 || len(entropy) == 0 {
		return nil, windows.ERROR_INVALID_DATA
	}
	in := windows.DataBlob{Size: uint32(len(sealed)), Data: &sealed[0]}
	salt := windows.DataBlob{Size: uint32(len(entropy)), Data: &entropy[0]}
	var out windows.DataBlob
	if err := windows.CryptUnprotectData(&in, nil, &salt, 0, nil, 1, &out); err != nil {
		return nil, err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return append([]byte(nil), unsafe.Slice(out.Data, int(out.Size))...), nil
}
