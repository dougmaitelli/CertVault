package vault

import (
	"bytes"
	"crypto/rand"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	plain := []byte("private key material")

	cipher, e := Encrypt(key, plain)
	if e != nil {
		t.Fatal(e)
	}

	if bytes.Contains(cipher, plain) {
		t.Fatal("ciphertext contains plaintext")
	}

	got, e := Decrypt(key, cipher)
	if e != nil {
		t.Fatal(e)
	}

	if !bytes.Equal(got, plain) {
		t.Fatal("round trip mismatch")
	}

	cipher[len(cipher)-1] ^= 1
	if _, e = Decrypt(key, cipher); e == nil {
		t.Fatal("tampering was not detected")
	}
}
