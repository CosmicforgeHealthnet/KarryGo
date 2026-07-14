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
	pass      map[string]*string // id -> nullable bcrypt password hash
	emailToID map[string]string
	seq       int
}

func newFakeAccountRepo() *fakeAccountRepo {
	return &fakeAccountRepo{byID: map[string]*Account{}, pass: map[string]*string{}, emailToID: map[string]string{}}
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

func (r *fakeAccountRepo) FindByEmail(_ context.Context, email string) (*Account, *string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	id, ok := r.emailToID[email]
	if !ok {
		return nil, nil, nil
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

func (r *fakeAccountRepo) UpdateProfile(_ context.Context, id string, params UpdateProfileParams) (*Account, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.byID[id]
	if !ok {
		return nil, false, nil
	}
	if params.BusinessName != nil {
		a.BusinessName = params.BusinessName
	}
	if params.BusinessPhone != nil {
		a.BusinessPhone = params.BusinessPhone
	}
	if params.State != nil {
		a.State = params.State
	}
	if params.City != nil {
		a.City = params.City
	}
	if params.BusinessLogoURL != nil {
		a.BusinessLogoURL = params.BusinessLogoURL
	}
	if params.ProfilePhotoURL != nil {
		a.ProfilePhotoURL = params.ProfilePhotoURL
	}
	cp := *a
	return &cp, true, nil
}
func (r *fakeAccountRepo) passwordHash(id string) *string {
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

// --- fakeOTPRepo -------------------------------------------------------------

type fakeOTPRepo struct {
	mu      sync.Mutex
	byID    map[string]*OTPChallenge
	byEmail map[string][]string
	seq     int
}

func newFakeOTPRepo() *fakeOTPRepo {
	return &fakeOTPRepo{byID: map[string]*OTPChallenge{}, byEmail: map[string][]string{}}
}

func (r *fakeOTPRepo) CreateOTPChallenge(_ context.Context, p CreateOTPChallengeParams) (OTPChallenge, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seq++
	id := "otp-" + strconv.Itoa(r.seq)
	c := OTPChallenge{ID: id, BusinessEmail: p.BusinessEmail, OTPCodeHash: p.OTPCodeHash, ExpiresAt: p.ExpiresAt, CreatedAt: time.Now()}
	r.byID[id] = &c
	r.byEmail[p.BusinessEmail] = append(r.byEmail[p.BusinessEmail], id)
	return c, nil
}

func (r *fakeOTPRepo) FindLatestOTPChallenge(_ context.Context, email string) (*OTPChallenge, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ids := r.byEmail[email]
	if len(ids) == 0 {
		return nil, nil
	}
	latest := r.byID[ids[len(ids)-1]]
	cp := *latest
	return &cp, nil
}

func (r *fakeOTPRepo) IncrementOTPAttempts(_ context.Context, id string) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c := r.byID[id]
	c.Attempts++
	return c.Attempts, nil
}

func (r *fakeOTPRepo) ConsumeOTPChallenge(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if c := r.byID[id]; c != nil && c.ConsumedAt == nil {
		now := time.Now()
		c.ConsumedAt = &now
	}
	return nil
}

func (r *fakeOTPRepo) DeleteOTPChallenge(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	c := r.byID[id]
	if c == nil {
		return nil
	}
	delete(r.byID, id)
	ids := r.byEmail[c.BusinessEmail]
	kept := ids[:0]
	for _, existing := range ids {
		if existing != id {
			kept = append(kept, existing)
		}
	}
	r.byEmail[c.BusinessEmail] = kept
	return nil
}

// --- fakePartnerEmailSender --------------------------------------------------

type sentOTP struct {
	Email string
	Code  string
}

type fakeOTPSender struct {
	mu   sync.Mutex
	sent []sentOTP
	fail error
}

func (s *fakeOTPSender) SendPartnerOTP(_ context.Context, email string, code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail != nil {
		return s.fail
	}
	s.sent = append(s.sent, sentOTP{Email: email, Code: code})
	return nil
}
