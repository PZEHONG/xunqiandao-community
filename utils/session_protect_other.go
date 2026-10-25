//go:build !windows

package utils

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"os"
	"sync"
)

var sessionKeyMu sync.Mutex

func sessionCipher(path string, create bool) (cipher.AEAD, error) {
	sessionKeyMu.Lock()
	defer sessionKeyMu.Unlock()
	key, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) && create {
		key = make([]byte, 32)
		if _, err = rand.Read(key); err != nil {
			return nil, err
		}
		file, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if errors.Is(e, os.ErrExist) {
			key, err = os.ReadFile(path)
		} else if e != nil {
			return nil, e
		} else {
			_, err = file.Write(key)
			closeErr := file.Close()
			if err == nil {
				err = closeErr
			}
		}
	}
	if err != nil {
		return nil, err
	}
	if len(key) != 32 {
		return nil, errors.New("invalid session encryption key")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
func protectSessionCredential(plain, entropy []byte, path string) ([]byte, error) {
	aead, err := sessionCipher(path, true)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return nil, err
	}
	return aead.Seal(nonce, nonce, plain, entropy), nil
}
func unprotectSessionCredential(sealed, entropy []byte, path string) ([]byte, error) {
	aead, err := sessionCipher(path, false)
	if err != nil {
		return nil, err
	}
	if len(sealed) < aead.NonceSize() {
		return nil, errors.New("invalid protected session")
	}
	return aead.Open(nil, sealed[:aead.NonceSize()], sealed[aead.NonceSize():], entropy)
}
