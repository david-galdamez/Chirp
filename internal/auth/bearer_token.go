package auth

import (
	"errors"
	"net/http"
	"strings"
)

func GetBearerToken(header http.Header) (string, error) {
	authHeader := header.Get("Authorization")
	if authHeader == "" {
		return "", errors.New("authorization header not provided")
	}

	val := strings.Split(authHeader, " ")
	if len(val) != 2 {
		return "", errors.New("malformed auth header")
	}

	if val[0] != "Bearer" {
		return "", errors.New("malformed first part of auth header")
	}

	return val[1], nil
}
