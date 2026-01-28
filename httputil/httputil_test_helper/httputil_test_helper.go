package httputil_test_helper

import (
	"github.com/ProtonMail/gopenpgp/v3/crypto"
)

func GenerateTestKey(name, email string) (string, error) {
	pgp := crypto.PGP()
	handle := pgp.KeyGeneration().
		AddUserId(name, email).
		New()
	key, err := handle.GenerateKey()
	if err != nil {
		return "", err
	}
	return key.Armor()
}

func SignMessage(message []byte, armoredKey string) (string, error) {
	pgp := crypto.PGP()
	key, err := crypto.NewKeyFromArmored(armoredKey)
	if err != nil {
		return "", err
	}
	keyring, err := crypto.NewKeyRing(key)
	if err != nil {
		return "", err
	}
	signer, err := pgp.Sign().
		SigningKeys(keyring).
		Detached().
		New()
	if err != nil {
		return "", err
	}
	signature, err := signer.Sign(message, crypto.Armor)
	if err != nil {
		return "", err
	}
	return string(signature), nil
}

func GetExpiredTestKey(name, email string) (string, error) {
	pgp := crypto.PGP()
	handle := pgp.KeyGeneration().
		AddUserId(name, email).
		Lifetime(1).
		New()
	key, err := handle.GenerateKey()
	if err != nil {
		return "", err
	}
	return key.Armor()
}
