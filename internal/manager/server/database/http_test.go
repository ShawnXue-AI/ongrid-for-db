package database

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	biz "github.com/ongridio/ongrid/internal/manager/biz/database"
	model "github.com/ongridio/ongrid/internal/manager/model/database"
	"github.com/ongridio/ongrid/internal/pkg/errs"
	"github.com/ongridio/ongrid/internal/pkg/tenantctx"
)

// fakeService implements Service for handler tests.
type fakeService struct {
	byID   map[uint64]*model.DatabaseInstance
	nextID uint64
}

func newFakeService(rows ...*model.DatabaseInstance) *fakeService {
	m := map[uint64]*model.DatabaseInstance{}
	maxID := uint64(0)
	for _, r := range rows {
		m[r.ID] = r
		if r.ID > maxID {
			maxID = r.ID
		}
	}
	return &fakeService{byID: m, nextID: maxID + 1}
}

func (s *fakeService) Create(_ context.Context, inst *model.DatabaseInstance) error {
	inst.ID = s.nextID
	s.nextID++
	s.byID[inst.ID] = inst
	return nil
}

func (s *fakeService) GetByID(_ context.Context, id uint64) (*model.DatabaseInstance, error) {
	if v, ok := s.byID[id]; ok {
		return v, nil
	}
	return nil, errs.ErrNotFound
}

func (s *fakeService) List(_ context.Context, _ biz.ListFilter) ([]*model.DatabaseInstance, error) {
	out := make([]*model.DatabaseInstance, 0, len(s.byID))
	for _, v := range s.byID {
		out = append(out, v)
	}
	return out, nil
}

func (s *fakeService) Update(_ context.Context, inst *model.DatabaseInstance) error {
	if _, ok := s.byID[inst.ID]; !ok {
		return errs.ErrNotFound
	}
	s.byID[inst.ID] = inst
	return nil
}

func (s *fakeService) UpdateStatus(_ context.Context, id uint64, status string) error {
	if _, ok := s.byID[id]; !ok {
		return errs.ErrNotFound
	}
	s.byID[id].Status = status
	return nil
}

func (s *fakeService) UpdateVersion(_ context.Context, id uint64, version string) error {
	if _, ok := s.byID[id]; !ok {
		return errs.ErrNotFound
	}
	s.byID[id].Version = version
	return nil
}

func (s *fakeService) Delete(_ context.Context, id uint64) error {
	if _, ok := s.byID[id]; !ok {
		return errs.ErrNotFound
	}
	delete(s.byID, id)
	return nil
}

// fakeAuthzMW implements AuthzMW for handler tests.
type fakeAuthzMW struct {
	allowAll bool
}

func (a *fakeAuthzMW) Require(obj, act string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !a.allowAll {
				http.Error(w, errs.ErrForbidden.Error(), http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// authContext embeds a tenant context into the request for authenticated tests.
func authContext(r *http.Request) *http.Request {
	return r.WithContext(tenantctx.With(r.Context(), tenantctx.Tenant{
		UserID:      1,
		Email:       "test@test.com",
		IsSuperuser: false,
	}))
}

// setupHandler creates a handler with a fake service, attached to a chi router.
func setupHandler(t *testing.T, svc *fakeService, allowWrite bool) *chi.Mux {
	t.Helper()
	h := NewHandler(svc)
	h.SetAuthz(&fakeAuthzMW{allowAll: allowWrite})
	mux := chi.NewRouter()
	h.Register(mux)
	return mux
}

func TestListDatabases(t *testing.T) {
	now := time.Now().UTC()
	t1 := &model.DatabaseInstance{
		ID: 1, EdgeID: 1, Name: "db-1", DBType: model.DBTypeMySQL,
		Host: "10.0.0.1", Port: 3306, Status: model.StatusOnline,
		CreatedAt: now, UpdatedAt: now,
	}
	t2 := &model.DatabaseInstance{
		ID: 2, EdgeID: 1, Name: "db-2", DBType: model.DBTypePostgreSQL,
		Host: "10.0.0.2", Port: 5432, Status: model.StatusOffline,
		CreatedAt: now, UpdatedAt: now,
	}
	svc := newFakeService(t1, t2)
	mux := setupHandler(t, svc, false)

	req := httptest.NewRequest("GET", "/v1/databases", nil)
	req = authContext(req)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var resp listResp
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Total != 2 {
		t.Fatalf("total = %d, want 2", resp.Total)
	}
	if len(resp.Items) != 2 {
		t.Fatalf("items = %d, want 2", len(resp.Items))
	}
}

func TestListDatabasesEmpty(t *testing.T) {
	svc := newFakeService()
	mux := setupHandler(t, svc, false)

	req := httptest.NewRequest("GET", "/v1/databases", nil)
	req = authContext(req)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var resp listResp
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Total != 0 {
		t.Fatalf("total = %d, want 0", resp.Total)
	}
	if len(resp.Items) != 0 {
		t.Fatalf("items = %d, want 0", len(resp.Items))
	}
}

func TestGetDatabase(t *testing.T) {
	now := time.Now().UTC()
	inst := &model.DatabaseInstance{
		ID: 1, EdgeID: 1, Name: "test-db", DBType: model.DBTypeMySQL,
		Host: "10.0.0.1", Port: 3306, Status: model.StatusOnline,
		CreatedAt: now, UpdatedAt: now,
	}
	svc := newFakeService(inst)
	mux := setupHandler(t, svc, false)

	req := httptest.NewRequest("GET", "/v1/databases/1", nil)
	req = authContext(req)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var resp instanceResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Name != "test-db" {
		t.Fatalf("name = %q, want %q", resp.Name, "test-db")
	}
	if resp.DBType != model.DBTypeMySQL {
		t.Fatalf("db_type = %q, want %q", resp.DBType, model.DBTypeMySQL)
	}
}

func TestGetDatabaseNotFound(t *testing.T) {
	svc := newFakeService()
	mux := setupHandler(t, svc, false)

	req := httptest.NewRequest("GET", "/v1/databases/999", nil)
	req = authContext(req)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestCreateDatabase(t *testing.T) {
	svc := newFakeService()
	mux := setupHandler(t, svc, true) // authz allows write

	body := `{"edge_id":1,"name":"new-db","db_type":"mysql","host":"10.0.0.3","port":3306}`
	req := httptest.NewRequest("POST", "/v1/databases", strings.NewReader(body))
	req = authContext(req)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusCreated, w.Body.String())
	}
	var resp instanceResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.ID == 0 {
		t.Fatal("id should not be zero")
	}
	if resp.Name != "new-db" {
		t.Fatalf("name = %q, want %q", resp.Name, "new-db")
	}
	if resp.Status != model.StatusUnknown {
		t.Fatalf("status = %q, want %q", resp.Status, model.StatusUnknown)
	}
}

func TestCreateDatabaseForbidden(t *testing.T) {
	svc := newFakeService()
	mux := setupHandler(t, svc, false) // authz denies write

	body := `{"edge_id":1,"name":"new-db","db_type":"mysql","host":"10.0.0.3","port":3306}`
	req := httptest.NewRequest("POST", "/v1/databases", strings.NewReader(body))
	req = authContext(req)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestCreateDatabaseInvalidBody(t *testing.T) {
	svc := newFakeService()
	mux := setupHandler(t, svc, true)

	// Missing required fields (name, db_type, host)
	body := `{"edge_id":1}`
	req := httptest.NewRequest("POST", "/v1/databases", strings.NewReader(body))
	req = authContext(req)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestCreateDatabaseUnsupportedType(t *testing.T) {
	svc := newFakeService()
	mux := setupHandler(t, svc, true)

	body := `{"edge_id":1,"name":"bad-db","db_type":"oracle","host":"10.0.0.4","port":1521}`
	req := httptest.NewRequest("POST", "/v1/databases", strings.NewReader(body))
	req = authContext(req)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Non-MySQL db_type should be rejected (v1 supports MySQL only)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

func TestUpdateDatabase(t *testing.T) {
	now := time.Now().UTC()
	inst := &model.DatabaseInstance{
		ID: 1, EdgeID: 1, Name: "old-name", DBType: model.DBTypeMySQL,
		Host: "10.0.0.1", Port: 3306, Status: model.StatusOnline,
		CreatedAt: now, UpdatedAt: now,
	}
	svc := newFakeService(inst)
	mux := setupHandler(t, svc, true)

	body := `{"name":"new-name","host":"10.0.0.1","port":3306}`
	req := httptest.NewRequest("PUT", "/v1/databases/1", strings.NewReader(body))
	req = authContext(req)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	var resp instanceResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Name != "new-name" {
		t.Fatalf("name = %q, want %q", resp.Name, "new-name")
	}
}

func TestUpdateDatabaseNotFound(t *testing.T) {
	svc := newFakeService()
	mux := setupHandler(t, svc, true)

	body := `{"name":"nobody","host":"10.0.0.9","port":3306}`
	req := httptest.NewRequest("PUT", "/v1/databases/999", strings.NewReader(body))
	req = authContext(req)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestUpdateDatabaseForbidden(t *testing.T) {
	now := time.Now().UTC()
	inst := &model.DatabaseInstance{
		ID: 1, EdgeID: 1, Name: "test", DBType: model.DBTypeMySQL,
		Host: "10.0.0.1", Port: 3306, Status: model.StatusOnline,
		CreatedAt: now, UpdatedAt: now,
	}
	svc := newFakeService(inst)
	mux := setupHandler(t, svc, false) // authz denies write

	body := `{"name":"new-name","host":"10.0.0.1","port":3306}`
	req := httptest.NewRequest("PUT", "/v1/databases/1", strings.NewReader(body))
	req = authContext(req)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestDeleteDatabase(t *testing.T) {
	now := time.Now().UTC()
	inst := &model.DatabaseInstance{
		ID: 1, EdgeID: 1, Name: "to-delete", DBType: model.DBTypeMySQL,
		Host: "10.0.0.1", Port: 3306, Status: model.StatusOnline,
		CreatedAt: now, UpdatedAt: now,
	}
	svc := newFakeService(inst)
	mux := setupHandler(t, svc, true)

	req := httptest.NewRequest("DELETE", "/v1/databases/1", nil)
	req = authContext(req)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNoContent)
	}

	// Verify deletion: Get should now return 404
	req2 := httptest.NewRequest("GET", "/v1/databases/1", nil)
	req2 = authContext(req2)
	w2 := httptest.NewRecorder()
	mux.ServeHTTP(w2, req2)
	if w2.Code != http.StatusNotFound {
		t.Fatalf("after delete, status = %d, want %d", w2.Code, http.StatusNotFound)
	}
}

func TestDeleteDatabaseNotFound(t *testing.T) {
	svc := newFakeService()
	mux := setupHandler(t, svc, true)

	req := httptest.NewRequest("DELETE", "/v1/databases/999", nil)
	req = authContext(req)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestDeleteDatabaseForbidden(t *testing.T) {
	now := time.Now().UTC()
	inst := &model.DatabaseInstance{
		ID: 1, EdgeID: 1, Name: "cant-delete", DBType: model.DBTypeMySQL,
		Host: "10.0.0.1", Port: 3306, Status: model.StatusOnline,
		CreatedAt: now, UpdatedAt: now,
	}
	svc := newFakeService(inst)
	mux := setupHandler(t, svc, false) // authz denies delete

	req := httptest.NewRequest("DELETE", "/v1/databases/1", nil)
	req = authContext(req)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestGetDatabaseInvalidID(t *testing.T) {
	svc := newFakeService()
	mux := setupHandler(t, svc, false)

	req := httptest.NewRequest("GET", "/v1/databases/abc", nil)
	req = authContext(req)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// --- helpers ---

