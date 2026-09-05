package jwt

import (
	"crypto/rsa"
	"fmt"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Claims JWT载荷
type Claims struct {
	UserID       string   `json:"user_id"`
	IdentityID   string   `json:"identity_id"`
	IdentityType string   `json:"identity_type"` // human / nhi
	Username     string   `json:"username,omitempty"`
	Role         string   `json:"role"`
	SubRole      string   `json:"sub_role"`
	NHIFlag      bool     `json:"nhi_flag"`
	MSAccess     []string `json:"ms_access"`
	Roles        []string `json:"roles,omitempty"`
	ActiveRole   string   `json:"active_role,omitempty"`
	Domain       string   `json:"domain,omitempty"`
	AgentService string   `json:"agent_service,omitempty"`
	DelegatedBy  string   `json:"delegated_by,omitempty"`
	TokenID      string   `json:"jti,omitempty"`
	jwt.RegisteredClaims
}

// Issuer 签发器
type Issuer struct {
	privateKey      *rsa.PrivateKey
	accessTokenTTL  time.Duration
	refreshTokenTTL time.Duration
	issuer          string
}

func NewIssuer(privateKeyPath string, accessTTL, refreshTTL time.Duration, issuer string) (*Issuer, error) {
	data, err := os.ReadFile(privateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("read private key: %w", err)
	}
	key, err := jwt.ParseRSAPrivateKeyFromPEM(data)
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}
	return &Issuer{privateKey: key, accessTokenTTL: accessTTL, refreshTokenTTL: refreshTTL, issuer: issuer}, nil
}

func (i *Issuer) IssueAccessToken(c *Claims) (string, int64, error) {
	now := time.Now()
	c.RegisteredClaims = jwt.RegisteredClaims{
		Issuer:    i.issuer,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(i.accessTokenTTL)),
		NotBefore: jwt.NewNumericDate(now),
		Subject:   c.UserID,
		ID:        c.TokenID,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, c)
	signed, err := token.SignedString(i.privateKey)
	return signed, int64(i.accessTokenTTL.Seconds()), err
}

func (i *Issuer) IssueAccessTokenWithTTL(c *Claims, ttl time.Duration) (string, int64, error) {
	now := time.Now()
	c.RegisteredClaims = jwt.RegisteredClaims{
		Issuer:    i.issuer,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		NotBefore: jwt.NewNumericDate(now),
		Subject:   c.UserID,
		ID:        c.TokenID,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, c)
	signed, err := token.SignedString(i.privateKey)
	return signed, int64(ttl.Seconds()), err
}

func (i *Issuer) IssueRefreshToken(userID, identityType, tokenID string) (string, error) {
	now := time.Now()
	claims := jwt.RegisteredClaims{
		Issuer:    i.issuer,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(i.refreshTokenTTL)),
		NotBefore: jwt.NewNumericDate(now),
		Subject:   userID,
		ID:        tokenID,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	return token.SignedString(i.privateKey)
}

// Verifier 验签器
type Verifier struct {
	publicKey *rsa.PublicKey
}

func NewVerifier(publicKeyPath string) (*Verifier, error) {
	data, err := os.ReadFile(publicKeyPath)
	if err != nil {
		return nil, fmt.Errorf("read public key: %w", err)
	}
	key, err := jwt.ParseRSAPublicKeyFromPEM(data)
	if err != nil {
		return nil, fmt.Errorf("parse public key: %w", err)
	}
	return &Verifier{publicKey: key}, nil
}

func (v *Verifier) Verify(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return v.publicKey, nil
	})
	if err != nil {
		return nil, fmt.Errorf("parse token: %w", err)
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token claims")
	}
	return claims, nil
}
