package service

import (
	"crypto"
	"crypto/ecdsa"

	"github.com/go-acme/lego/v5/acme"
	"github.com/go-acme/lego/v5/registration"
)

type acmeUser struct {
	Email        string
	Registration *acme.ExtendedAccount
	Key          *ecdsa.PrivateKey
}

type acmeUserWire struct {
	DirectoryURL string
	Email        string
	Key          []byte
	Registration *acme.ExtendedAccount
}

type ACMEAccount struct {
	ID              string `json:"id"`
	DirectoryURL    string `json:"directory_url,omitempty"`
	Email           string `json:"email"`
	Status          string `json:"status"`
	RegistrationURL string `json:"registration_url,omitempty"`
	Current         bool   `json:"current"`
}

var _ registration.User = (*acmeUser)(nil)

func (u *acmeUser) GetEmail() string                       { return u.Email }
func (u *acmeUser) GetRegistration() *acme.ExtendedAccount { return u.Registration }
func (u *acmeUser) GetPrivateKey() crypto.Signer           { return u.Key }
