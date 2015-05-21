package token

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"time"
)

const (
	EXP_1HOUR  = 60 * 60
	EXP_6HOUR  = EXP_1HOUR * 6
	EXP_12HOUR = EXP_1HOUR * 12
	EXP_1DAY   = EXP_1HOUR * 24
	EXP_2DAY   = EXP_1DAY * 2
	EXP_3DAY   = EXP_1DAY * 3
	EXP_1WEEK  = EXP_1DAY * 7
	EXP_2WEEK  = EXP_1WEEK * 2
	EXP_3WEEK  = EXP_1WEEK * 3
	EXP_4WEEK  = EXP_1WEEK * 4
	EXP_NEVER  = 1<<63 - 1
)

var (
	ERR_MALFORMED_TOKEN = errors.New("Token does not contain header, body, and signature only")
	ERR_INVALID_SIG     = errors.New("Token signature is not valid.")
	ERR_EXPIRED         = errors.New("Token is expired")
)

var jwt_header []byte

func init() {
	jwth := []byte(`{"alg":"HS256","typ":"JWT"}`)
	jwt_header = make([]byte, base64.URLEncoding.EncodedLen(len(jwth)))
	base64.URLEncoding.Encode(jwt_header, jwth)
}

type Token struct {
	Contents map[string]string `json:"contents"`
	Exp      int64             `json:"exp"`
}

func NewToken(exp int64) *Token {
	return &Token{
		Contents: make(map[string]string),
		Exp:      time.Now().Unix() + exp,
	}
}

func (t *Token) Sign(key []byte) ([]byte, error) {
	// Convert the token to json
	jsonBytes, err := json.Marshal(t)
	if err != nil {
		return nil, err
	}

	// Convert the json bytes to base64
	body := make([]byte, base64.URLEncoding.EncodedLen(len(jsonBytes)))
	base64.URLEncoding.Encode(body, jsonBytes)

	// Append the body to the JWT header
	payload := append(append(jwt_header, '.'), body...)

	// Create an HMAC signature of the payload.
	mac := hmac.New(sha256.New, key)
	mac.Write(payload)
	sig := mac.Sum(nil)
	b64sig := make([]byte, base64.URLEncoding.EncodedLen(len(sig)))
	base64.URLEncoding.Encode(b64sig, sig)

	// Append the signature and return
	return append(append(payload, '.'), b64sig...), nil
}

func Validate(token []byte, key []byte) (*Token, error) {
	// Split the token by . ensuring len is 3 (toss header for now)
	tokenPieces := bytes.Split(token, []byte{'.'})
	if len(tokenPieces) != 3 {
		return nil, ERR_MALFORMED_TOKEN
	}

	// Validate the HMAC signature
	mac := hmac.New(sha256.New, key)
	mac.Write(tokenPieces[0])
	mac.Write([]byte{'.'})
	mac.Write(tokenPieces[1])
	expectedSig := mac.Sum(nil)
	b64expectedSig := make([]byte, base64.URLEncoding.EncodedLen(len(expectedSig)))
	base64.URLEncoding.Encode(b64expectedSig, expectedSig)
	if !hmac.Equal(b64expectedSig, tokenPieces[2]) {
		return nil, ERR_INVALID_SIG
	}

	// Decode the body into a Token
	jsonBytes := make([]byte, base64.URLEncoding.DecodedLen(len(tokenPieces[1])))
	_, err := base64.URLEncoding.Decode(jsonBytes, tokenPieces[1])
	if err != nil {
		return nil, err
	}
	jsonBytes = bytes.TrimRight(jsonBytes, "\x00")
	t := new(Token)
	err = json.Unmarshal(jsonBytes[:len(jsonBytes)], t) // bytes.Split leaves null character at the end of the string that json does not like
	if err != nil {
		return nil, err
	}

	// Check exp date
	if t.Exp < time.Now().Unix() {
		return nil, ERR_EXPIRED
	}

	// Return the validated token
	return t, nil
}
