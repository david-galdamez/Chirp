package auth

import (
	"errors"
	"net/http"
	"strings"
)

func GetAPIKey(headers http.Header) (string, error) {
	authHeader := headers.Get("Authorization")
	if authHeader == "" {
		return "", errors.New("authorization header not provided")
	}

	val := strings.Split(authHeader, " ")
	if len(val) != 2 {
		return "", errors.New("malformed auth header")
	}

	if val[0] != "ApiKey" {
		return "", errors.New("malformed first part of auth header")
	}

	return val[1], nil
}
