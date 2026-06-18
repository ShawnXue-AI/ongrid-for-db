// Package credentials provides AES-GCM encrypted storage for database
// instance credentials (user/password). Credentials are keyed by the
// database instance ID and stored in a dedicated table so the
// DatabaseInstance model stays free of plaintext secrets.
//
// The encryption key is derived at process start from ongrid's
// internal secret key. This is a defence-in-depth measure: the primary
// protection is that credentials never leave the manager process
// boundary (they are NOT passed through the LLM prompt context).
//
// Phase 1 (Jun 2026): minimal encrypted store. Phase 2 should replace
// the derived key with a KMS-backed key (Vault, AWS KMS, etc.).
package credentials

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"sync"

	"gorm.io/gorm"
)

// Credential holds the encrypted user/password for one database instance.
type Credential struct {
	InstanceID uint64 `gorm:"primaryKey;column:instance_id"`
	DBUser     string `gorm:"type:text;not null;column:db_user"`
	DBPassword string `gorm:"type:text;not null;column:db_password"`
}

// TableName pins the table name for gorm.
func (Credential) TableName() string { return "database_credentials" }

// Store provides encrypted read/write access to database credentials.
type Store struct {
	db *gorm.DB
	aead cipher.AEAD
	mu   sync.Mutex
}

// NewStore creates a credential store. gcmKey must be exactly 32 bytes
// (AES-256). The caller should derive it from the application secret.
func NewStore(db *gorm.DB, gcmKey []byte) (*Store, error) {
	if len(gcmKey) != 32 {
		return nil, fmt.Errorf("credentials: gcmKey must be 32 bytes (got %d)", len(gcmKey))
	}
	block, err := aes.NewCipher(gcmKey)
	if err != nil {
		return nil, fmt.Errorf("credentials: new cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("credentials: new gcm: %w", err)
	}
	if err := db.AutoMigrate(&Credential{}); err != nil {
		return nil, fmt.Errorf("credentials: migrate: %w", err)
	}
	return &Store{db: db, aead: aead}, nil
}

// Set stores (or replaces) credentials for an instance. Plaintext is
// encrypted before writing to the database.
func (s *Store) Set(ctx context.Context, instanceID uint64, user, password string) error {
	encUser, err := s.encrypt(user)
	if err != nil {
		return fmt.Errorf("credentials: encrypt user: %w", err)
	}
	encPass, err := s.encrypt(password)
	if err != nil {
		return fmt.Errorf("credentials: encrypt password: %w", err)
	}
	return s.db.WithContext(ctx).Save(&Credential{
		InstanceID: instanceID,
		DBUser:     encUser,
		DBPassword: encPass,
	}).Error
}

// Get retrieves and decrypts credentials for an instance. Returns
// (false, nil) when no credentials are stored.
func (s *Store) Get(ctx context.Context, instanceID uint64) (user, password string, found bool, err error) {
	var cred Credential
	if err := s.db.WithContext(ctx).First(&cred, instanceID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", "", false, nil
		}
		return "", "", false, fmt.Errorf("credentials: get: %w", err)
	}
	user, err = s.decrypt(cred.DBUser)
	if err != nil {
		return "", "", false, fmt.Errorf("credentials: decrypt user: %w", err)
	}
	password, err = s.decrypt(cred.DBPassword)
	if err != nil {
		return "", "", false, fmt.Errorf("credentials: decrypt password: %w", err)
	}
	return user, password, true, nil
}

// LookupCredentials implements aiopstools.CredentialResolver. Delegates to Get.
func (s *Store) LookupCredentials(ctx context.Context, instanceID uint64) (user, password string, found bool, err error) {
	return s.Get(ctx, instanceID)
}

// Delete removes credentials for an instance.
func (s *Store) Delete(ctx context.Context, instanceID uint64) error {
	return s.db.WithContext(ctx).Delete(&Credential{}, instanceID).Error
}

func (s *Store) encrypt(plaintext string) (string, error) {
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := s.aead.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func (s *Store) decrypt(encoded string) (string, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	nonceSize := s.aead.NonceSize()
	if len(ciphertext) < nonceSize {
		return "", errors.New("ciphertext too short")
	}
	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := s.aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}
	return string(plaintext), nil
}
