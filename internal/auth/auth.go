package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	UserID string `json:"user_id"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

type contextKey struct{}

type Validator struct {
	secret   []byte
	issuer   string
	audience string
}

func NewValidator(secret, issuer, audience string) *Validator {
	return &Validator{secret: []byte(secret), issuer: issuer, audience: audience}
}

func (v *Validator) Parse(raw string) (*Claims, error) {
	claims := new(Claims)
	token, err := jwt.ParseWithClaims(
		raw,
		claims,
		func(token *jwt.Token) (any, error) {
			if token.Method != jwt.SigningMethodHS256 {
				return nil, fmt.Errorf("unexpected signing algorithm %q", token.Method.Alg())
			}
			return v.secret, nil
		},
		jwt.WithIssuer(v.issuer),
		jwt.WithAudience(v.audience),
		jwt.WithExpirationRequired(),
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
	)
	if err != nil || !token.Valid {
		return nil, fmt.Errorf("invalid token: %w", err)
	}
	if claims.UserID == "" || claims.Role == "" {
		return nil, errors.New("user_id and role claims are required")
	}
	return claims, nil
}

func (v *Validator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		scheme, token, ok := strings.Cut(header, " ")
		if !ok || !strings.EqualFold(scheme, "Bearer") || token == "" {
			writeUnauthorized(w, "missing bearer token")
			return
		}
		claims, err := v.Parse(token)
		if err != nil {
			writeUnauthorized(w, "invalid bearer token")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), contextKey{}, claims)))
	})
}

func FromContext(ctx context.Context) (*Claims, bool) {
	claims, ok := ctx.Value(contextKey{}).(*Claims)
	return claims, ok
}

func writeUnauthorized(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(`{"error":"` + message + `"}`))
}
