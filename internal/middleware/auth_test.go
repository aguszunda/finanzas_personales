package middleware

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func signToken(t *testing.T, secret []byte, sub int64, exp time.Time) string {
	t.Helper()
	claims := jwt.MapClaims{
		"sub": float64(sub),
		"exp": exp.Unix(),
		"iat": time.Now().Unix(),
	}
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return tok
}

func newTestHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		uid := UserIDFromContext(r.Context())
		_, _ = w.Write([]byte("userID=" + strconv.FormatInt(uid, 10)))
	})
}

func TestJWTAuth_NoToken(t *testing.T) {
	h := JWTAuth([]byte("secret"))(newTestHandler())
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestJWTAuth_InvalidToken(t *testing.T) {
	h := JWTAuth([]byte("secret"))(newTestHandler())
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer token-invalido")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestJWTAuth_ExpiredToken(t *testing.T) {
	secret := []byte("secret")
	h := JWTAuth(secret)(newTestHandler())
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+signToken(t, secret, 1, time.Now().Add(-time.Hour)))
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestJWTAuth_ValidHeader(t *testing.T) {
	secret := []byte("secret")
	h := JWTAuth(secret)(newTestHandler())
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+signToken(t, secret, 42, time.Now().Add(time.Hour)))
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if rec.Body.String() != "userID=42" {
		t.Errorf("unexpected body: %q", rec.Body.String())
	}
}

func TestJWTAuth_ValidQueryToken(t *testing.T) {
	secret := []byte("secret")
	h := JWTAuth(secret)(newTestHandler())
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/?token="+signToken(t, secret, 7, time.Now().Add(time.Hour)), nil)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if rec.Body.String() != "userID=7" {
		t.Errorf("unexpected body: %q", rec.Body.String())
	}
}

func TestJWTAuth_ValidCookie(t *testing.T) {
	secret := []byte("secret")
	h := JWTAuth(secret)(newTestHandler())
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "token", Value: signToken(t, secret, 9, time.Now().Add(time.Hour))})
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if rec.Body.String() != "userID=9" {
		t.Errorf("unexpected body: %q", rec.Body.String())
	}
}

func TestJWTAuth_AlgNONE(t *testing.T) {
	// alg=none should be rejected (only HS256 family accepted)
	tok := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{"sub": float64(1), "exp": time.Now().Add(time.Hour).Unix()})
	tokenStr, err := tok.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	h := JWTAuth([]byte("secret"))(newTestHandler())
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for alg=none, got %d", rec.Code)
	}
}

func TestUserIDFromContext_Missing(t *testing.T) {
	if got := UserIDFromContext(httptest.NewRequest(http.MethodGet, "/", nil).Context()); got != 0 {
		t.Errorf("expected 0, got %d", got)
	}
}
