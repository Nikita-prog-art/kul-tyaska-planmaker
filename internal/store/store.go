// Package store — простое надёжное хранилище в одном JSON-файле.
// Пользователей, сессии и планы немного, зато такой файл легко
// забэкапить, посмотреть глазами и перенести.
package store

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"kultyaska/internal/plan"
)

// Ошибки хранилища.
var (
	ErrNotFound = errors.New("не найдено")
	ErrNickBusy = errors.New("ник уже занят")
)

// User — учётная запись.
type User struct {
	ID           string    `json:"id"`
	Nick         string    `json:"nick"`
	Discord      string    `json:"discord"`
	PassHash     string    `json:"pass_hash"`
	IsAdmin      bool      `json:"is_admin"`
	CreatedAt    time.Time `json:"created_at"`
	LastLogin    time.Time `json:"last_login,omitempty"`
	TempPassword string    `json:"temp_password,omitempty"`
}

// Session — сессия пользователя.
type Session struct {
	Token   string    `json:"token"`
	UserID  string    `json:"user_id"`
	Expires time.Time `json:"expires"`
}

// data — содержимое файла базы.
type data struct {
	Version  int                   `json:"version"`
	Users    []*User               `json:"users"`
	Sessions map[string]*Session   `json:"sessions"`
	Plans    map[string]*plan.Plan `json:"plans"`
}

// Store — хранилище с блокировкой и атомарной записью.
type Store struct {
	mu   sync.RWMutex
	path string
	d    data
}

// Open читает базу (или создаёт пустую).
func Open(path string) (*Store, error) {
	s := &Store{path: path, d: data{Version: 1, Sessions: map[string]*Session{}, Plans: map[string]*plan.Plan{}}}
	raw, err := os.ReadFile(path)
	switch {
	case err == nil:
		if err := json.Unmarshal(raw, &s.d); err != nil {
			return nil, fmt.Errorf("база %s повреждена: %w", path, err)
		}
		if s.d.Sessions == nil {
			s.d.Sessions = map[string]*Session{}
		}
		if s.d.Plans == nil {
			s.d.Plans = map[string]*plan.Plan{}
		}
	case errors.Is(err, os.ErrNotExist):
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, err
		}
		if err := s.save(); err != nil {
			return nil, err
		}
	default:
		return nil, err
	}
	s.dropExpired()
	return s, nil
}

// Close ничего не делает: данные пишутся при каждом изменении.
func (s *Store) Close() error { return nil }

// Path возвращает путь к файлу базы.
func (s *Store) Path() string { return s.path }

// save пишет базу атомарно. Вызывается под замком.
func (s *Store) save() error {
	raw, err := json.MarshalIndent(&s.d, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *Store) dropExpired() {
	now := time.Now().UTC()
	for tok, sess := range s.d.Sessions {
		if now.After(sess.Expires) {
			delete(s.d.Sessions, tok)
		}
	}
}

// NewID генерирует идентификатор.
func NewID() string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}

func findUser(users []*User, nick string) *User {
	for _, u := range users {
		if strings.EqualFold(u.Nick, nick) {
			return u
		}
	}
	return nil
}

// CreateUser создаёт пользователя.
func (s *Store) CreateUser(u *User) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if findUser(s.d.Users, u.Nick) != nil {
		return ErrNickBusy
	}
	if u.ID == "" {
		u.ID = NewID()
	}
	if u.CreatedAt.IsZero() {
		u.CreatedAt = time.Now().UTC()
	}
	s.d.Users = append(s.d.Users, u)
	return s.save()
}

// UpdateUser сохраняет изменения пользователя.
func (s *Store) UpdateUser(u *User) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, old := range s.d.Users {
		if old.ID == u.ID {
			s.d.Users[i] = u
			return s.save()
		}
	}
	return ErrNotFound
}

// UserByNick ищет пользователя по нику.
func (s *Store) UserByNick(nick string) (*User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if u := findUser(s.d.Users, nick); u != nil {
		return u, nil
	}
	return nil, ErrNotFound
}

// UserByID ищет пользователя по идентификатору.
func (s *Store) UserByID(id string) (*User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, u := range s.d.Users {
		if u.ID == id {
			return u, nil
		}
	}
	return nil, ErrNotFound
}

// ListUsers возвращает всех пользователей по алфавиту.
func (s *Store) ListUsers() ([]*User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*User, len(s.d.Users))
	copy(out, s.d.Users)
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Nick) < strings.ToLower(out[j].Nick) })
	return out, nil
}

// CountUsers считает пользователей.
func (s *Store) CountUsers() (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.d.Users), nil
}

// CreateSession создаёт сессию.
func (s *Store) CreateSession(userID string, ttl time.Duration) (*Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dropExpired()
	sess := &Session{Token: NewID() + NewID(), UserID: userID, Expires: time.Now().UTC().Add(ttl)}
	s.d.Sessions[sess.Token] = sess
	if err := s.save(); err != nil {
		return nil, err
	}
	return sess, nil
}

// Session ищет сессию по токену.
func (s *Store) Session(token string) (*Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.d.Sessions[token]
	if !ok {
		return nil, ErrNotFound
	}
	if time.Now().UTC().After(sess.Expires) {
		return nil, ErrNotFound
	}
	return sess, nil
}

// DeleteSession удаляет сессию.
func (s *Store) DeleteSession(token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.d.Sessions, token)
	return s.save()
}

// DropSessionsUser выкидывает все сессии пользователя.
func (s *Store) DropSessionsUser(userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for tok, sess := range s.d.Sessions {
		if sess.UserID == userID {
			delete(s.d.Sessions, tok)
		}
	}
	return s.save()
}

// PutPlan сохраняет план.
func (s *Store) PutPlan(p *plan.Plan) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p.Normalize()
	p.UpdatedAt = time.Now().UTC()
	if p.CreatedAt.IsZero() {
		p.CreatedAt = p.UpdatedAt
	}
	cp := *p
	s.d.Plans[p.ID] = &cp
	return s.save()
}

// Plan возвращает план по идентификатору.
// Normalize нужен и на чтении: в базе могли остаться планы со списками-null.
func (s *Store) Plan(id string) (*plan.Plan, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.d.Plans[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *p
	cp.Normalize()
	return &cp, nil
}

// PlansByUser возвращает планы пользователя, свежие сверху.
func (s *Store) PlansByUser(userID string) ([]*plan.Plan, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []*plan.Plan
	for _, p := range s.d.Plans {
		if p.UserID == userID {
			cp := *p
			cp.Normalize()
			out = append(out, &cp)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}

// DeletePlan удаляет план.
func (s *Store) DeletePlan(p *plan.Plan) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.d.Plans, p.ID)
	return s.save()
}

// PlansCountByUser считает планы пользователя.
func (s *Store) PlansCountByUser(userID string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n := 0
	for _, p := range s.d.Plans {
		if p.UserID == userID {
			n++
		}
	}
	return n
}

// Stats — сводка для админки.
func (s *Store) Stats() (users, plans int, err error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.d.Users), len(s.d.Plans), nil
}
