package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
)

func signHMACSHA256(message []byte, secret []byte) []byte {
	mac := hmac.New(sha256.New, secret)
	mac.Write(message)
	return mac.Sum(nil)
}

func hmacEqual(a, b []byte) bool {
	return hmac.Equal(a, b)
}
