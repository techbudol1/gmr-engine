package wallet

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"strings"

	"github.com/ethereum/go-ethereum/crypto"
)

type GeneratedWallet struct {
	Address             string
	EncryptedPrivateKey string
}

func Generate(encryptionKey string) (GeneratedWallet, error) {
	if len(encryptionKey) < 32 {
		return GeneratedWallet{}, errors.New("wallet encryption key must be at least 32 characters")
	}
	privateKey, err := crypto.GenerateKey()
	if err != nil {
		return GeneratedWallet{}, err
	}
	privateKeyBytes := crypto.FromECDSA(privateKey)
	encrypted, err := encrypt(privateKeyBytes, encryptionKey)
	if err != nil {
		return GeneratedWallet{}, err
	}
	return GeneratedWallet{
		Address:             crypto.PubkeyToAddress(privateKey.PublicKey).Hex(),
		EncryptedPrivateKey: encrypted,
	}, nil
}

func encrypt(plaintext []byte, encryptionKey string) (string, error) {
	keyHash := sha256.Sum256([]byte(encryptionKey))
	block, err := aes.NewCipher(keyHash[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)
	return "v1:" + base64.RawStdEncoding.EncodeToString(nonce) + ":" + base64.RawStdEncoding.EncodeToString(ciphertext), nil
}

func DecryptPrivateKey(encrypted string, encryptionKey string) (string, error) {
	if len(encryptionKey) < 32 {
		return "", errors.New("wallet encryption key must be at least 32 characters")
	}
	parts := strings.Split(strings.TrimSpace(encrypted), ":")
	if len(parts) != 3 || parts[0] != "v1" {
		return "", errors.New("unsupported encrypted wallet format")
	}
	nonce, err := base64.RawStdEncoding.DecodeString(parts[1])
	if err != nil {
		return "", err
	}
	ciphertext, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil {
		return "", err
	}
	keyHash := sha256.Sum256([]byte(encryptionKey))
	block, err := aes.NewCipher(keyHash[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return "0x" + hex.EncodeToString(plaintext), nil
}
