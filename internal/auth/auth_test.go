package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func signedToken(t *testing.T, secret, issuer, audience, userID, role string, expires time.Time) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, Claims{
		UserID: userID,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    issuer,
			Audience:  jwt.ClaimStrings{audience},
			ExpiresAt: jwt.NewNumericDate(expires),
		},
	})
	raw, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestValidatorMiddleware(t *testing.T) {
	const secret = "a-secret-long-enough-for-tests"
	validator := NewValidator(secret, "test-issuer", "test-audience")
	token := signedToken(t, secret, "test-issuer", "test-audience", "user-42", "admin", time.Now().Add(time.Hour))

	handler := validator.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := FromContext(r.Context())
		if !ok || claims.UserID != "user-42" || claims.Role != "admin" {
			t.Fatalf("unexpected claims: %#v", claims)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNoContent)
	}
}

func TestValidatorRejectsExpiredToken(t *testing.T) {
	const secret = "a-secret-long-enough-for-tests"
	validator := NewValidator(secret, "test-issuer", "test-audience")
	token := signedToken(t, secret, "test-issuer", "test-audience", "user-42", "user", time.Now().Add(-time.Minute))
	if _, err := validator.Parse(token); err == nil {
		t.Fatal("expected expired token error")
	}
}
