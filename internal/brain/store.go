// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Gabski/brainDB/internal/patterns"
)

// Event is broadcast over SSE and appended to the journal.
type Event struct {
	Type    string   `json:"type"`
	TS      int64    `json:"ts"`
	By      string   `json:"by"`
	Tool    string   `json:"tool"`
	Summary string   `json:"summary"`
	IDs     []string `json:"ids"`
	DB      string   `json:"db,omitempty"`
	Done    int      `json:"done,omitempty"`
	Total   int      `json:"total,omitempty"`
	File    string   `json:"file,omitempty"`
}

// Store binds a Brain to its brain.xml file: atomic saves, a JSONL journal, event fan-out and reload when
// another process changes the file.
type Store struct {
	mu       sync.Mutex
	B        *Brain
	Path     string // "" = in memory only
	Name     string // database name
	ReadOnly bool
	Log      func(format string, a ...any)

	Conf     *Config
	ConfText string

	// Backend is "file" (brain.xml) or "sqlite" (brain.db, Addendum F). It follows the extension of
	// Path, so a store is never ambiguous about where its durable copy lives.
	Backend string
	db      *sql.DB

	lastMod  time.Time
	lastSize int64
	subMu    sync.Mutex
	subs     map[chan Event]struct{}
	stop     chan struct{}
}

// JournalPath returns the journal file next to the brain file.
func JournalPath(brainPath string) string {
	return strings.TrimSuffix(brainPath, filepath.Ext(brainPath)) + ".journal.jsonl"
}

// OpenStore opens (or creates from seedPath, or empty) the brain at path.
// BackendName reports the backend this store persists through, deriving it from the path when it was
// not set explicitly (Path can be assigned after the store is constructed).
func (s *Store) BackendName() string {
	if s.Backend != "" {
		return s.Backend
	}
	if s.Path == "" {
		return "file"
	}
	return BackendFor(s.Path)
}

// BackendFor reports the storage backend a path uses: ".db" / ".sqlite" mean SQLite, anything else
// is the readable brain.xml file backend.
func BackendFor(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".db", ".sqlite", ".sqlite3", ".bdb":
		return "sqlite"
	}
	return "file"
}

func OpenStore(path, seedPath string) (*Store, error) {
	s := &Store{B: New(), Path: path, subs: map[chan Event]struct{}{}, Log: func(string, ...any) {}}
	s.Backend = BackendFor(path)
	if path != "" {
		s.B.Dir = filepath.Dir(path)
	}
	s.Name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if path == "" {
		s.Backend = "file"
		return s, nil
	}
	if s.Backend == "sqlite" {
		db, err := OpenSQLiteDB(path)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		s.db = db
		fresh, err := sqliteEmpty(db)
		if err != nil {
			return nil, err
		}
		if !fresh {
			if err := s.B.ReadSQLite(db); err != nil {
				return nil, fmt.Errorf("%s: %w", path, err)
			}
			s.stamp()
			if err := s.LoadConfig(false); err != nil {
				return nil, err
			}
			return s, nil
		}
		if seedPath != "" {
			if err := s.B.LoadSeedFile(seedPath); err != nil {
				return nil, err
			}
		}
		if err := s.LoadConfig(seedPath == ""); err != nil {
			return nil, err
		}
		if err := s.save(); err != nil {
			return nil, err
		}
		return s, nil
	}
	if d, err := os.ReadFile(path); err == nil {
		if err := s.B.LoadXML(string(d)); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		s.stamp()
		if err := s.LoadConfig(false); err != nil {
			return nil, err
		}
		return s, nil
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	if seedPath != "" {
		if err := s.B.LoadSeedFile(seedPath); err != nil {
			return nil, err
		}
	}
	if err := s.LoadConfig(seedPath == ""); err != nil {
		return nil, err
	}
	if err := s.save(); err != nil {
		return nil, err
	}
	return s, nil
}

// NewMemStore wraps an existing brain without a file.
func NewMemStore(b *Brain, name string) *Store {
	return &Store{B: b, Name: name, subs: map[chan Event]struct{}{}, Log: func(string, ...any) {}}
}

func (s *Store) stamp() {
	if st, err := os.Stat(s.Path); err == nil {
		s.lastMod, s.lastSize = st.ModTime(), st.Size()
	}
}

func (s *Store) save() error {
	if s.Path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o755); err != nil {
		return err
	}
	if s.BackendName() == "sqlite" {
		if s.db == nil {
			db, err := OpenSQLiteDB(s.Path)
			if err != nil {
				return err
			}
			s.db = db
		}
		if err := s.B.WriteSQLite(s.db); err != nil {
			return err
		}
		s.stamp()
		return nil
	}
	tmp := s.Path + ".tmp"
	if err := os.WriteFile(tmp, []byte(s.B.XML()), 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.Path); err != nil {
		return err
	}
	s.stamp()
	return nil
}

// Save writes brain.xml atomically.
func (s *Store) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.save()
}

// syncLocked reloads the brain when the file was changed by another process.
func (s *Store) syncLocked() bool {
	if s.Path == "" {
		return false
	}
	st, err := os.Stat(s.Path)
	if err != nil || (st.ModTime().Equal(s.lastMod) && st.Size() == s.lastSize) {
		return false
	}
	if s.BackendName() == "sqlite" {
		if s.db == nil {
			return false
		}
		if err := s.B.ReadSQLite(s.db); err != nil {
			s.Log("reload %s: %v", s.Path, err)
			s.stamp()
			return false
		}
		s.stamp()
		if s.Conf != nil {
			_ = s.B.ApplyConfig(s.Conf, false)
		}
		return true
	}
	d, err := os.ReadFile(s.Path)
	if err != nil {
		return false
	}
	if err := s.B.LoadXML(string(d)); err != nil {
		s.Log("reload %s: %v", s.Path, err)
		s.stamp()
		return false
	}
	s.stamp()
	if s.Conf != nil {
		_ = s.B.ApplyConfig(s.Conf, false)
	}
	return true
}

// Sync reloads the brain if brain.xml changed on disk. Returns true when reloaded.
func (s *Store) Sync() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.syncLocked()
}

// Read runs fn with the store locked (after syncing from disk).
func (s *Store) Read(fn func(b *Brain) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.syncLocked()
	return fn(s.B)
}

// ErrReadOnly is returned for writes on a read-only store.
var ErrReadOnly = fmt.Errorf("this brain is read-only")

// Mutate runs fn, saves, journals and broadcasts an event. fn returns the event summary and ids.
func (s *Store) Mutate(by, tool string, fn func(b *Brain) (string, []string, error)) error {
	s.mu.Lock()
	if s.ReadOnly {
		s.mu.Unlock()
		return ErrReadOnly
	}
	s.syncLocked()
	summary, ids, err := fn(s.B)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	err = s.save()
	s.mu.Unlock()
	if err != nil {
		return err
	}
	s.Emit(by, tool, summary, ids)
	return nil
}

// Emit journals and broadcasts a change event.
func (s *Store) Emit(by, tool, summary string, ids []string) {
	if ids == nil {
		ids = []string{}
	}
	ev := Event{Type: "changed", TS: time.Now().UnixMilli(), By: by, Tool: tool, Summary: summary, IDs: ids, DB: s.Name}
	if s.Path != "" {
		if f, err := os.OpenFile(JournalPath(s.Path), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
			d, _ := json.Marshal(ev)
			_, _ = f.Write(append(d, '\n'))
			f.Close()
		}
	}
	s.Broadcast(ev)
}

// Broadcast sends an event to all subscribers without blocking.
func (s *Store) Broadcast(ev Event) {
	s.subMu.Lock()
	defer s.subMu.Unlock()
	for c := range s.subs {
		select {
		case c <- ev:
		default:
		}
	}
}

// Subscribe returns a channel of events and a cancel function.
func (s *Store) Subscribe() (chan Event, func()) {
	c := make(chan Event, 64)
	s.subMu.Lock()
	s.subs[c] = struct{}{}
	s.subMu.Unlock()
	return c, func() {
		s.subMu.Lock()
		if _, ok := s.subs[c]; ok {
			delete(s.subs, c)
			close(c)
		}
		s.subMu.Unlock()
	}
}

// CloseSubscribers closes every subscriber channel (shutdown).
func (s *Store) CloseSubscribers() {
	s.subMu.Lock()
	defer s.subMu.Unlock()
	for c := range s.subs {
		delete(s.subs, c)
		close(c)
	}
}

// Journal returns the last n journal events.
func (s *Store) Journal(n int) []Event {
	out := []Event{}
	if s.Path == "" {
		return out
	}
	f, err := os.Open(JournalPath(s.Path))
	if err != nil {
		return out
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		var ev Event
		if json.Unmarshal(sc.Bytes(), &ev) == nil {
			out = append(out, ev)
		}
	}
	if len(out) > n {
		out = out[len(out)-n:]
	}
	return out
}

// Watch polls brain.xml for changes by other processes and broadcasts a "file" event on reload.
func (s *Store) Watch(interval time.Duration) {
	if s.Path == "" || s.stop != nil {
		return
	}
	s.stop = make(chan struct{})
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-s.stop:
				return
			case <-t.C:
				if s.Sync() {
					s.Broadcast(Event{Type: "changed", TS: time.Now().UnixMilli(), By: "file", Tool: "file", Summary: "brain changed by another process", IDs: []string{}, DB: s.Name})
				}
			}
		}
	}()
}

// Close stops the watcher and closes subscribers.
func (s *Store) Close() {
	if s.stop != nil {
		close(s.stop)
		s.stop = nil
	}
	s.CloseSubscribers()
	s.mu.Lock()
	if s.db != nil {
		_ = s.db.Close()
		s.db = nil
	}
	s.mu.Unlock()
}

var _ = io.EOF

// LoadConfig reads braindb.yaml next to brain.xml (if any) and applies it.
func (s *Store) LoadConfig(newDB bool) error {
	if s.Path == "" {
		return nil
	}
	cfg, text, err := LoadConfigFile(ConfigPath(s.Path))
	if err != nil {
		return err
	}
	s.ConfText = text
	if cfg == nil {
		// no braindb.yaml: the environment can still select the embedding engine
		return s.B.ApplyEmbedder(EmbedderFromEnv(EmbedderOptions{}))
	}
	s.Conf = cfg
	return s.B.ApplyConfig(cfg, newDB)
}

// SaveConfigText validates and writes braindb.yaml (atomic, with a .bak backup) and applies it.
func (s *Store) SaveConfigText(text string) (*Config, []ConfigError, error) {
	cfg, errs := ParseConfig(text)
	if len(errs) > 0 {
		return nil, errs, nil
	}
	if s.Path == "" {
		return cfg, nil, fmt.Errorf("this brain has no directory to store braindb.yaml")
	}
	p := ConfigPath(s.Path)
	if old, err := os.ReadFile(p); err == nil {
		_ = os.WriteFile(p+".bak", old, 0o600)
	}
	if err := os.WriteFile(p+".tmp", []byte(text), 0o600); err != nil {
		return nil, nil, err
	}
	if err := os.Rename(p+".tmp", p); err != nil {
		return nil, nil, err
	}
	s.mu.Lock()
	s.Conf, s.ConfText = cfg, text
	s.mu.Unlock()
	return cfg, nil, s.B.ApplyConfig(cfg, false)
}

// SaveAccess persists the access configuration (to access.file when set, otherwise into braindb.yaml) and applies it.
func (s *Store) SaveAccess(a *AccessConfig) error {
	s.B.SetAccess(a)
	if s.Path == "" {
		return nil
	}
	cp := ConfigPath(s.Path)
	write := func(p, text string) error {
		if old, err := os.ReadFile(p); err == nil {
			_ = os.WriteFile(p+".bak", old, 0o600)
		}
		if err := os.WriteFile(p+".tmp", []byte(text), 0o600); err != nil {
			return err
		}
		return os.Rename(p+".tmp", p)
	}
	if a.File != "" {
		p := a.File
		if !filepath.IsAbs(p) {
			p = filepath.Join(filepath.Dir(cp), p)
		}
		c := *a
		c.File = ""
		return write(p, EmitYAML(toOrdered(c)))
	}
	root := NewOMap()
	if s.ConfText != "" {
		if v, err := ParseYAML(s.ConfText); err == nil {
			if m, ok := v.(*OMap); ok {
				root = m
			}
		}
	}
	root.Set("access", toOrdered(a))
	text := "# braindb.yaml (rewritten by an access statement; the previous version is in braindb.yaml.bak)\n" + EmitYAML(root)
	if err := write(cp, text); err != nil {
		return err
	}
	s.ConfText = text
	if s.Conf != nil {
		s.Conf.Access = *a
	}
	return nil
}

// SetPatternEnabled enables or disables a pattern and persists the change in braindb.yaml.
func (s *Store) SetPatternEnabled(name string, enabled bool) error {
	o := s.B.PatternOptions()
	dis := filterStr(o.Disabled, func(x string) bool { return x != name })
	if !enabled {
		dis = append(dis, name)
	}
	s.B.SetDisabledPatterns(dis)
	return s.SavePatterns()
}

// customPatternList renders the live custom patterns as YAML nodes.
func customPatternList(cs []*patterns.Custom) []any {
	out := []any{}
	for _, c := range cs {
		m := NewOMap()
		m.Set("name", c.Name)
		m.Set("type", c.Type)
		m.Set("regex", c.Regex)
		if c.Normalize != "" {
			m.Set("normalize", c.Normalize)
		}
		if c.PII {
			m.Set("pii", true)
		}
		out = append(out, m)
	}
	return out
}

// SavePatterns writes the live pattern options (disabled list and custom patterns) into braindb.yaml.
// Saving reapplies the config, which rebuilds the options from the file, so both halves have to go in
// together: otherwise enabling one pattern would delete every custom one added at runtime.
func (s *Store) SavePatterns() error {
	if s.Path == "" {
		return nil
	}
	o := s.B.PatternOptions()
	dis := []any{}
	for _, d := range o.Disabled {
		dis = append(dis, d)
	}
	text := SpliceYAML(s.ConfText, []string{"patterns", "disabled"}, dis)
	// Written even when empty: removing the last custom pattern has to clear the block, or the next
	// load brings it back.
	text = SpliceYAML(text, []string{"patterns", "custom"}, customPatternList(o.Custom))
	if _, errs, err := s.SaveConfigText(text); len(errs) > 0 || err != nil {
		if err == nil {
			err = errs[0]
		}
		return err
	}
	return nil
}

// SetConfigKey replaces one top-level key of braindb.yaml (validated, atomic, with a backup) and applies the result.
func (s *Store) SetConfigKey(key string, value any) error {
	_, errs, err := s.SaveConfigText(SpliceYAML(s.ConfText, []string{key}, toOrdered(value)))
	if len(errs) > 0 {
		return errs[0]
	}
	return err
}
