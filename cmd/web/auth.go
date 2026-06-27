package web

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

const sessionCookie = "mythic_ctrl_session"
const tokenTTL = 8 * time.Hour

// Authentication is stateless: on login we issue an HS256 JWT signed with the
// Mythic `JWT_SECRET` config value and store it in an HttpOnly cookie. Every
// protected request re-verifies the signature and expiry — there is no
// server-side session store to lose on restart.

type jwtHeader struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
}

type jwtClaims struct {
	Sub string `json:"sub"` // username
	Iat int64  `json:"iat"`
	Exp int64  `json:"exp"`
}

func b64url(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func signHS256(signingInput string, secret []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(signingInput))
	return b64url(mac.Sum(nil))
}

// issueToken builds a signed HS256 JWT for the given user.
func issueToken(user string, secret []byte) string {
	header, _ := json.Marshal(jwtHeader{Alg: "HS256", Typ: "JWT"})
	now := time.Now()
	claims, _ := json.Marshal(jwtClaims{
		Sub: user,
		Iat: now.Unix(),
		Exp: now.Add(tokenTTL).Unix(),
	})
	signingInput := b64url(header) + "." + b64url(claims)
	return signingInput + "." + signHS256(signingInput, secret)
}

// parseToken verifies the signature, algorithm, and expiry, returning the claims.
func parseToken(token string, secret []byte) (*jwtClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("malformed token")
	}
	signingInput := parts[0] + "." + parts[1]

	// Verify the signature in constant time before trusting any field.
	want := signHS256(signingInput, secret)
	if subtle.ConstantTimeCompare([]byte(want), []byte(parts[2])) != 1 {
		return nil, errors.New("invalid signature")
	}

	// Reject anything that isn't the HS256 we issue (defends against alg swaps).
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, err
	}
	var h jwtHeader
	if err := json.Unmarshal(headerBytes, &h); err != nil {
		return nil, err
	}
	if h.Alg != "HS256" {
		return nil, errors.New("unexpected signing algorithm")
	}

	claimBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, err
	}
	var c jwtClaims
	if err := json.Unmarshal(claimBytes, &c); err != nil {
		return nil, err
	}
	if time.Now().Unix() >= c.Exp {
		return nil, errors.New("token expired")
	}
	return &c, nil
}

// authenticated reports whether the request carries a valid, unexpired JWT.
func authenticated(r *http.Request) bool {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return false
	}
	secret := jwtSecret()
	if len(secret) == 0 {
		return false
	}
	_, err = parseToken(c.Value, secret)
	return err == nil
}

// requireAuth gates protected handlers. Unauthenticated browser requests are
// redirected to /login; htmx requests get a 401 with an HX-Redirect header so
// the client navigates instead of swapping a login page into a fragment.
func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !authenticated(r) {
			if r.Header.Get("HX-Request") == "true" {
				w.Header().Set("HX-Redirect", "/login")
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleLoginForm(w http.ResponseWriter, r *http.Request) {
	if authenticated(r) {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	s.renderer.page(w, "login", data("login"))
}

func (s *Server) handleLoginSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	user := r.FormValue("username")
	pass := r.FormValue("password")

	secret := jwtSecret()
	if len(secret) == 0 {
		w.WriteHeader(http.StatusInternalServerError)
		s.renderer.page(w, "login", data("login", "Error",
			"Server misconfiguration: JWT_SECRET is not set in the Mythic config"))
		return
	}

	if !validateAdminCredentials(user, pass) {
		w.WriteHeader(http.StatusUnauthorized)
		s.renderer.page(w, "login", data("login", "Error", "Invalid credentials"))
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    issueToken(user, secret),
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   r.TLS != nil,
		MaxAge:   int(tokenTTL.Seconds()),
	})
	http.Redirect(w, r, "/", http.StatusFound)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	// Stateless tokens can't be revoked server-side without a denylist; clearing
	// the cookie logs this browser out, which is the expected behaviour here.
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
	http.Redirect(w, r, "/login", http.StatusFound)
}

// validateAdminCredentials checks the submitted username/password against the
// Mythic admin credentials stored in the local .env (read via the config
// package). Comparison is constant-time. See adapter.go for the config access.
func validateAdminCredentials(user, pass string) bool {
	wantUser, wantPass := adminCredentials()
	if wantPass == "" {
		// No admin password configured: refuse rather than allow blank logins.
		return false
	}
	userOK := subtle.ConstantTimeCompare([]byte(user), []byte(wantUser)) == 1
	passOK := subtle.ConstantTimeCompare([]byte(pass), []byte(wantPass)) == 1
	return userOK && passOK
}
