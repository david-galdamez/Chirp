package auth

import (
	"crypto/rand"
	"encoding/hex"
)

func MakeRefreshToken() (string, error) {

	data := make([]byte, 32)

	_, err := rand.Read(data)
	if err != nil {
		return "", err
	}

	encoded := hex.EncodeToString(data)

	return encoded, nil
}
