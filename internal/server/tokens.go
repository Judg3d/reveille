package server

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const waitTokenTTL = 24 * time.Hour

type waitTokenClaims struct {
	Host      string `json:"host"`
	ReturnTo  string `json:"returnTo"`
	ExpiresAt int64  `json:"expiresAt"`
}

func tokenKey(configured []byte) []byte {
	if len(configured) > 0 {
		return configured
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		panic(fmt.Errorf("generate wait token key: %w", err))
	}
	return key
}

func (s *Server) ensureTokenKey() []byte {
	if len(s.tokenKey) == 0 {
		s.tokenKey = tokenKey(nil)
	}
	return s.tokenKey
}

func (s *Server) newWaitToken(host, returnTo string) (string, error) {
	claims := waitTokenClaims{
		Host:      strings.ToLower(strings.TrimSpace(host)),
		ReturnTo:  sanitizeReturnTo(returnTo),
		ExpiresAt: s.now().Add(waitTokenTTL).Unix(),
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)
	signature := s.sign(encodedPayload)
	return encodedPayload + "." + signature, nil
}

func (s *Server) verifyWaitToken(raw string) (waitTokenClaims, error) {
	parts := strings.Split(raw, ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return waitTokenClaims{}, errors.New("invalid token format")
	}
	if !hmac.Equal([]byte(parts[1]), []byte(s.sign(parts[0]))) {
		return waitTokenClaims{}, errors.New("invalid token signature")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return waitTokenClaims{}, errors.New("invalid token payload")
	}
	var claims waitTokenClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return waitTokenClaims{}, errors.New("invalid token claims")
	}
	claims.Host = strings.ToLower(strings.TrimSpace(claims.Host))
	claims.ReturnTo = sanitizeReturnTo(claims.ReturnTo)
	if claims.Host == "" || claims.ExpiresAt <= 0 {
		return waitTokenClaims{}, errors.New("invalid token claims")
	}
	if !s.now().Before(time.Unix(claims.ExpiresAt, 0)) {
		return waitTokenClaims{}, errors.New("expired token")
	}
	return claims, nil
}

func (s *Server) sign(payload string) string {
	mac := hmac.New(sha256.New, s.ensureTokenKey())
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (s *Server) now() time.Time {
	if s.deps.StartClock != nil {
		return s.deps.StartClock()
	}
	return time.Now()
}

func requestToken(r *http.Request) string {
	if token := r.URL.Query().Get("token"); token != "" {
		return token
	}
	if token := r.FormValue("token"); token != "" {
		return token
	}
	return r.Header.Get("X-Reveille-Token")
}
