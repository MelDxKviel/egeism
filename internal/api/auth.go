package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"egeism/internal/domain"
)

// ctxKey is the private type for context values.
type ctxKey int

const userKey ctxKey = iota

const tokenTTL = 30 * 24 * time.Hour

// issueToken mints a signed session JWT for a user. The role is embedded for
// convenience but authorization always re-checks the loaded user.
func (s *Server) issueToken(u domain.User) (string, error) {
	claims := jwt.MapClaims{
		"sub":  u.ID.String(),
		"role": string(u.Role),
		"exp":  time.Now().Add(tokenTTL).Unix(),
		"iat":  time.Now().Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return tok.SignedString([]byte(s.jwtSecret))
}

// parseToken validates a JWT and returns the subject user id.
func (s *Server) parseToken(raw string) (uuid.UUID, error) {
	tok, err := jwt.Parse(raw, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return []byte(s.jwtSecret), nil
	})
	if err != nil || !tok.Valid {
		return uuid.Nil, fmt.Errorf("invalid token")
	}
	claims, ok := tok.Claims.(jwt.MapClaims)
	if !ok {
		return uuid.Nil, fmt.Errorf("invalid claims")
	}
	sub, _ := claims["sub"].(string)
	return uuid.Parse(sub)
}

// withUser resolves the acting user from the Authorization: Bearer token and
// stores it in the request context. Real session auth (replaces the earlier
// X-User-ID placeholder).
func (s *Server) withUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := s.bearerUserID(r)
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "authentication required")
			return
		}
		user, err := s.store.GetUser(r.Context(), id)
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "unknown user")
			return
		}
		ctx := context.WithValue(r.Context(), userKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) bearerUserID(r *http.Request) (uuid.UUID, error) {
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		return uuid.Nil, fmt.Errorf("missing bearer token")
	}
	return s.parseToken(strings.TrimSpace(strings.TrimPrefix(h, "Bearer ")))
}

// userFrom returns the acting user placed by withUser.
func userFrom(ctx context.Context) (domain.User, bool) {
	u, ok := ctx.Value(userKey).(domain.User)
	return u, ok
}
