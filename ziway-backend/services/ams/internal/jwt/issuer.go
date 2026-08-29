package jwt

import (
	"crypto/rsa"
	"fmt"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Claims JWT Claims，包含戴帽子信息和NHI标识
type Claims struct {
	UserID       string   `json:"user_id"`
	IdentityType string   `json:"identity_type"` // human / nhi
	Roles        []string `json:"roles"`
	ActiveRole   string   `json:"active_role,omitempty"` // 当前佩戴的主角色/帽子
	Domain       string   `json:"domain,omitempty"`
	// NHI专属：Agent服务名 + 委托方
	AgentService string `json:"agent_service,omitempty"`
	DelegatedBy  string `json:"delegated_by,omitempty"`
	TokenID      string `json:"jti,omitempty"`
	jwt.RegisteredClaims
}

// Issuer JWT签发器
type Issuer struct {
	privateKey      *rsa.PrivateKey
	accessTokenTTL  time.Duration
	refreshTokenTTL time.Duration
	issuer          string
}

// NewIssuer 从PEM文件加载私钥创建签发器
func NewIssuer(privateKeyPath string, accessTTL, refreshTTL time.Duration, issuer string) (*Issuer, error) {
	keyData, err := os.ReadFile(privateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("read private key: %w", err)
	}

	privateKey, err := jwt.ParseRSAPrivateKeyFromPEM(keyData)
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}

	return &Issuer{
		privateKey:      privateKey,
		accessTokenTTL:  accessTTL,
		refreshTokenTTL: refreshTTL,
		issuer:          issuer,
	}, nil
}

// IssueAccessToken 签发Access Token
func (i *Issuer) IssueAccessToken(c *Claims) (string, int64, error) {
	now := time.Now()
	exp := now.Add(i.accessTokenTTL)

	c.RegisteredClaims = jwt.RegisteredClaims{
		Issuer:    i.issuer,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(exp),
		NotBefore: jwt.NewNumericDate(now),
		Subject:   c.UserID,
		ID:        c.TokenID,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, c)
	signed, err := token.SignedString(i.privateKey)
	if err != nil {
		return "", 0, fmt.Errorf("sign access token: %w", err)
	}

	return signed, int64(i.accessTokenTTL.Seconds()), nil
}

// IssueRefreshToken 签发Refresh Token（不含roles，只做续期）
func (i *Issuer) IssueRefreshToken(userID, identityType, tokenID string) (string, error) {
	now := time.Now()
	exp := now.Add(i.refreshTokenTTL)

	claims := jwt.RegisteredClaims{
		Issuer:    i.issuer,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(exp),
		NotBefore: jwt.NewNumericDate(now),
		Subject:   userID,
		ID:        tokenID,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	return token.SignedString(i.privateKey)
}
