package partneraccount

import (
	"context"
	"strconv"
	"sync"
	"time"
)

// --- fakeAccountRepo ---------------------------------------------------------

type fakeAccountRepo struct {
	mu        sync.Mutex
	byID      map[string]*Account
	pass      map[string]string // id -> bcrypt password hash
	emailToID map[string]string
	seq       int
}

func newFakeAccountRepo() *fakeAccountRepo {
	return &fakeAccountRepo{byID: map[string]*Account{}, pass: map[string]string{}, emailToID: map[string]string{}}
}

func (r *fakeAccountRepo) CreateAccount(_ context.Context, p CreateAccountParams) (Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.emailToID[p.BusinessEmail]; ok {
		return Account{}, ErrEmailTaken
	}
	r.seq++
	id := "acct-" + strconv.Itoa(r.seq)
	a := Account{ID: id, BusinessName: p.BusinessName, BusinessEmail: p.BusinessEmail, Status: p.Status, Plan: p.Plan, CreatedAt: time.Now()}
	r.byID[id] = &a
	r.pass[id] = p.PasswordHash
	r.emailToID[p.BusinessEmail] = id
	return a, nil
}

func (r *fakeAccountRepo) FindByEmail(_ context.Context, email string) (*Account, string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	id, ok := r.emailToID[email]
	if !ok {
		return nil, "", nil
	}
	cp := *r.byID[id]
	return &cp, r.pass[id], nil
}

func (r *fakeAccountRepo) FindByID(_ context.Context, id string) (*Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.byID[id]
	if !ok {
		return nil, nil
	}
	cp := *a
	return &cp, nil
}

func (r *fakeAccountRepo) UpdatePlan(_ context.Context, id, plan string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.byID[id]
	if !ok {
		return false, nil
	}
	a.Plan = plan
	return true, nil
}

func (r *fakeAccountRepo) UpdateStatus(_ context.Context, id, status string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.byID[id]
	if !ok {
		return false, nil
	}
	a.Status = status
	return true, nil
}

func (r *fakeAccountRepo) passwordHash(id string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.pass[id]
}

// --- fakeSessionRepo ---------------------------------------------------------

type fakeSessionRepo struct {
	mu     sync.Mutex
	byHash map[string]*Session
	seq    int
}

func newFakeSessionRepo() *fakeSessionRepo { return &fakeSessionRepo{byHash: map[string]*Session{}} }

func (r *fakeSessionRepo) CreateSession(_ context.Context, p CreateSessionParams) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seq++
	r.byHash[p.TokenHash] = &Session{
		ID: "sess-" + strconv.Itoa(r.seq), PartnerID: p.PartnerID, TokenHash: p.TokenHash,
		ExpiresAt: p.ExpiresAt, CreatedAt: time.Now(),
	}
	return nil
}

func (r *fakeSessionRepo) FindByTokenHash(_ context.Context, hash string) (*Session, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.byHash[hash]
	if !ok {
		return nil, nil
	}
	cp := *s
	return &cp, nil
}

func (r *fakeSessionRepo) Revoke(_ context.Context, hash string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if s, ok := r.byHash[hash]; ok && s.RevokedAt == nil {
		now := time.Now()
		s.RevokedAt = &now
	}
	return nil
}

// --- fakeKeyStore ------------------------------------------------------------

type storedKey struct {
	id, partnerID, keyHash, prefix, env, label, status string
}

type fakeKeyStore struct {
	mu   sync.Mutex
	keys []*storedKey
	seq  int
}

func newFakeKeyStore() *fakeKeyStore { return &fakeKeyStore{} }

func (s *fakeKeyStore) CreateKey(_ context.Context, p CreateKeyParams) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seq++
	id := "key-" + strconv.Itoa(s.seq)
	s.keys = append(s.keys, &storedKey{
		id: id, partnerID: p.PartnerID, keyHash: p.KeyHash, prefix: p.KeyPrefix,
		env: p.Environment, label: p.Label, status: p.Status,
	})
	return id, nil
}

func (s *fakeKeyStore) ListByPartner(_ context.Context, partnerID string) ([]KeyView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []KeyView
	for _, k := range s.keys {
		if k.partnerID == partnerID {
			out = append(out, KeyView{ID: k.id, Prefix: k.prefix, Environment: k.env, Label: k.label, Status: k.status})
		}
	}
	return out, nil
}

func (s *fakeKeyStore) RevokeByPartner(_ context.Context, partnerID, keyID string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, k := range s.keys {
		if k.id == keyID && k.partnerID == partnerID {
			k.status = "revoked"
			return true, nil
		}
	}
	return false, nil
}
