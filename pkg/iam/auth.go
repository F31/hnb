package iam

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

type PasswordHasher struct{}

func NewPasswordHasher() *PasswordHasher {
	return &PasswordHasher{}
}

func (ph *PasswordHasher) Hash(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}

	hash := sha256.Sum256(append(salt, []byte(password)...))
	return hex.EncodeToString(salt) + ":" + hex.EncodeToString(hash[:]), nil
}

func (ph *PasswordHasher) Verify(password, hash string) bool {
	parts := strings.SplitN(hash, ":", 2)
	if len(parts) != 2 {
		return false
	}
	salt, err := hex.DecodeString(parts[0])
	if err != nil {
		return false
	}
	expected, err := hex.DecodeString(parts[1])
	if err != nil {
		return false
	}

	computed := sha256.Sum256(append(salt, []byte(password)...))
	return hmacEqual(computed[:], expected)
}

type Authenticator struct {
	hasher    *PasswordHasher
	userStore UserStore
}

type UserStore interface {
	GetByUsername(username string) (*User, error)
	GetByID(id string) (*User, error)
	CreateUser(user *User) error
	UpdateUser(user *User) error
	DeleteUser(id string) error
	ListUsers() ([]User, error)
}

func NewAuthenticator(userStore UserStore) *Authenticator {
	return &Authenticator{
		hasher:    NewPasswordHasher(),
		userStore: userStore,
	}
}

func (a *Authenticator) Authenticate(username, password string) (*User, error) {
	user, err := a.userStore.GetByUsername(username)
	if err != nil {
		return nil, fmt.Errorf("user not found")
	}
	if !user.IsActive {
		return nil, fmt.Errorf("user is disabled")
	}
	if !a.hasher.Verify(password, user.PasswordHash) {
		return nil, fmt.Errorf("invalid password")
	}
	return user, nil
}

func (a *Authenticator) NewPasswordHash(password string) (string, error) {
	return a.hasher.Hash(password)
}

func (a *Authenticator) CreateUser(username, password, email, phone, displayName string) (*User, error) {
	hash, err := a.hasher.Hash(password)
	if err != nil {
		return nil, err
	}

	user := &User{
		ID:           generateID(),
		Username:     username,
		Email:        email,
		Phone:        phone,
		DisplayName:  displayName,
		PasswordHash: hash,
		Source:       "local",
		IsActive:     true,
	}

	if err := a.userStore.CreateUser(user); err != nil {
		return nil, err
	}
	return user, nil
}

func hmacEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	result := 0
	for i := range a {
		result |= int(a[i]) ^ int(b[i])
	}
	return result == 0
}

func init() {
	_ = rand.Reader
}
