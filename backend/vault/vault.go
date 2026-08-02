package vault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"io"
)

var magic = []byte("CV01")

func Encrypt(key, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	out := append(append([]byte{}, magic...), nonce...)
	return gcm.Seal(out, nonce, plaintext, nil), nil
}

func Decrypt(key, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	n := gcm.NonceSize()
	if len(ciphertext) < len(magic)+n || string(ciphertext[:len(magic)]) != string(magic) {
		return nil, errors.New("invalid encrypted data")
	}
	return gcm.Open(nil, ciphertext[len(magic):len(magic)+n], ciphertext[len(magic)+n:], nil)
}
