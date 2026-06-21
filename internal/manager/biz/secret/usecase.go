// Package secret is the biz tier for the credential vault (HLD-017). It
// owns encryption (seal field map before storage, unseal only for the
// in-process injection path) and redaction (the list/get API exposes field
// NAMES, never values).
package secret

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	model "github.com/ongridio/ongrid/internal/manager/model/secret"
	"github.com/ongridio/ongrid/internal/pkg/errs"
	"github.com/ongridio/ongrid/internal/pkg/secretbox"
)

// Repo is the persistence contract (data/secret/store).
type Repo interface {
	Create(ctx context.Context, s *model.Secret) error
	Update(ctx context.Context, id uint64, data, description string) error
	Delete(ctx context.Context, id uint64) error
	List(ctx context.Context) ([]*model.Secret, error)
	GetByName(ctx context.Context, name string) (*model.Secret, error)
}

// View is the redacted shape returned to API callers — field NAMES only,
// never the values.
type View struct {
	ID          uint64    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	FieldKeys   []string  `json:"field_keys"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Usecase is the credential-vault facade.
type Usecase struct{ repo Repo }

// NewUsecase wires the repo.
func NewUsecase(repo Repo) *Usecase { return &Usecase{repo: repo} }

// Create seals the field map and stores a new named credential.
func (u *Usecase) Create(ctx context.Context, name, description string, fields map[string]string) (*View, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("%w: name required", errs.ErrInvalid)
	}
	fields = clean(fields)
	if len(fields) == 0 {
		return nil, fmt.Errorf("%w: at least one field required", errs.ErrInvalid)
	}
	sealed, err := seal(fields)
	if err != nil {
		return nil, err
	}
	s := &model.Secret{Name: name, Data: sealed, Description: strings.TrimSpace(description)}
	if err := u.repo.Create(ctx, s); err != nil {
		return nil, err
	}
	return toView(s, fields), nil
}

// Update re-seals the field map (when non-nil/non-empty) and/or updates the
// description. Passing nil fields edits only the description.
func (u *Usecase) Update(ctx context.Context, id uint64, description string, fields map[string]string) error {
	sealed := ""
	if fields != nil {
		fields = clean(fields)
		if len(fields) == 0 {
			return fmt.Errorf("%w: at least one field required", errs.ErrInvalid)
		}
		var err error
		if sealed, err = seal(fields); err != nil {
			return err
		}
	}
	return u.repo.Update(ctx, id, sealed, strings.TrimSpace(description))
}

// Delete removes a credential.
func (u *Usecase) Delete(ctx context.Context, id uint64) error { return u.repo.Delete(ctx, id) }

// List returns all credentials, redacted (field keys, no values).
func (u *Usecase) List(ctx context.Context) ([]*View, error) {
	rows, err := u.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*View, 0, len(rows))
	for _, s := range rows {
		fields, _ := unseal(s.Data) // best-effort: a decrypt failure still lists the row (no keys)
		out = append(out, toView(s, fields))
	}
	return out, nil
}

// ResolveFields returns the decrypted field map for the named credential —
// the in-process injection path only (never serialized over an API).
func (u *Usecase) ResolveFields(ctx context.Context, name string) (map[string]string, error) {
	s, err := u.repo.GetByName(ctx, name)
	if err != nil {
		return nil, err
	}
	return unseal(s.Data)
}

// --- helpers ---

func seal(fields map[string]string) (string, error) {
	b, err := json.Marshal(fields)
	if err != nil {
		return "", err
	}
	return secretbox.Encrypt(string(b))
}

func unseal(data string) (map[string]string, error) {
	plain, err := secretbox.Decrypt(data)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	if strings.TrimSpace(plain) == "" {
		return out, nil
	}
	if err := json.Unmarshal([]byte(plain), &out); err != nil {
		return nil, err
	}
	return out, nil
}

// clean drops blank keys/values and trims keys.
func clean(in map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range in {
		k = strings.TrimSpace(k)
		if k == "" || v == "" {
			continue
		}
		out[k] = v
	}
	return out
}

func toView(s *model.Secret, fields map[string]string) *View {
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return &View{
		ID:          s.ID,
		Name:        s.Name,
		Description: s.Description,
		FieldKeys:   keys,
		CreatedAt:   s.CreatedAt,
		UpdatedAt:   s.UpdatedAt,
	}
}
