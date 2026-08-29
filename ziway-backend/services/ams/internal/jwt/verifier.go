package jwt

import (
	"crypto/rsa"
	"fmt"
	"os"

	"github.com/golang-jwt/jwt/v5"
)

// Verifier JWT验签器（其他服务只需要公钥）
type Verifier struct {
	publicKey *rsa.PublicKey
}

// NewVerifier 从PEM文件加载公钥创建验签器
func NewVerifier(publicKeyPath string) (*Verifier, error) {
	keyData, err := os.ReadFile(publicKeyPath)
	if err != nil {
		return nil, fmt.Errorf("read public key: %w", err)
	}

	publicKey, err := jwt.ParseRSAPublicKeyFromPEM(keyData)
	if err != nil {
		return nil, fmt.Errorf("parse public key: %w", err)
	}

	return &Verifier{publicKey: publicKey}, nil
}

// Verify 验证Token并返回Claims
func (v *Verifier) Verify(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		// 强制检查签名算法，防止alg=none攻击
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
