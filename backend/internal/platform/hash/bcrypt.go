// Package hash implements domain.PasswordHasher with bcrypt.
package hash

import "golang.org/x/crypto/bcrypt"

type BcryptHasher struct{ Cost int }

func NewBcryptHasher() BcryptHasher {
	return BcryptHasher{Cost: bcrypt.DefaultCost}
}

func (h BcryptHasher) Hash(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), h.Cost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (h BcryptHasher) Compare(hash, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}
