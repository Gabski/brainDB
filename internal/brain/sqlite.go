// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

package brain

// The SQLite storage backend (Addendum F2). One file per database, WAL mode, the structured schema
// the query pushdown in sqlitequery.go reads. brain.xml stays available through `braindb export`.

import (
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	_ "modernc.org/sqlite" // pure Go: the release targets cross-compile with CGO_ENABLED=0
)

// SchemaVersion is bumped when the SQLite layout changes in a way a reader must notice.
const SchemaVersion = 2

const schemaSQL = `
CREATE TABLE IF NOT EXISTS meta (
  key   TEXT PRIMARY KEY,
  value TEXT
);
CREATE TABLE IF NOT EXISTS kind (
  name      TEXT PRIMARY KEY,
  label     TEXT,
  color     TEXT,
  base      REAL,
  half_life REAL,
  floor     REAL,
  prefix    TEXT,
  section   TEXT
);
CREATE TABLE IF NOT EXISTS section (
  name TEXT PRIMARY KEY,
  ord  INTEGER
);
CREATE TABLE IF NOT EXISTS node (
  id        TEXT PRIMARY KEY,
  ord       INTEGER,
  kind      TEXT,
  section   TEXT,
  text      TEXT,
  base      REAL,
  age       REAL,
  touched   INTEGER,
  pinned    INTEGER,
  sup       TEXT,
  parent    TEXT,
  idx       INTEGER,
  src       TEXT,
  hash      TEXT,
  labels    TEXT,
  meta      TEXT,
  purpose   TEXT,
  conn      TEXT,
  ext       TEXT
);
CREATE TABLE IF NOT EXISTS field (
  node_id TEXT,
  ord     INTEGER,
  name    TEXT,
  value   TEXT,
  num     REAL
);
CREATE TABLE IF NOT EXISTS edge (
  ord  INTEGER,
  a    TEXT,
  b    TEXT,
  type TEXT,
  w    REAL
);
CREATE TABLE IF NOT EXISTS vec (
  node_id TEXT,
  kind    TEXT,
  ord     INTEGER,
  dim     INTEGER,
  data    BLOB
);
CREATE TABLE IF NOT EXISTS stem (
  stem    TEXT,
  node_id TEXT
);
CREATE TABLE IF NOT EXISTS source (
  name TEXT PRIMARY KEY,
  ord  INTEGER,
  def  TEXT
);
-- The collation has to match the one the queries use, or SQLite ignores the index and scans.
CREATE INDEX IF NOT EXISTS node_kind    ON node(kind COLLATE NOCASE);
CREATE INDEX IF NOT EXISTS node_section ON node(section COLLATE NOCASE);
CREATE INDEX IF NOT EXISTS node_ord     ON node(ord);
CREATE INDEX IF NOT EXISTS edge_a       ON edge(a, type);
CREATE INDEX IF NOT EXISTS edge_b       ON edge(b, type);
CREATE INDEX IF NOT EXISTS field_node   ON field(node_id);
CREATE INDEX IF NOT EXISTS field_nv     ON field(name COLLATE NOCASE, value COLLATE NOCASE);
CREATE INDEX IF NOT EXISTS field_nn     ON field(name COLLATE NOCASE, num);
CREATE INDEX IF NOT EXISTS stem_stem    ON stem(stem);
CREATE INDEX IF NOT EXISTS vec_node     ON vec(node_id, kind);
`

// OpenSQLiteDB opens (creating if needed) a brain database and applies the pragmas and schema.
func OpenSQLiteDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	for _, p := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA foreign_keys=ON",
		"PRAGMA busy_timeout=5000",
	} {
		if _, err := db.Exec(p); err != nil {
			db.Close()
			return nil, fmt.Errorf("%s: %w", p, err)
		}
	}
	// Schema 1 created these with the default collation, which the queries cannot use. Dropping them
	// is safe: they are indexes, and the next statement rebuilds them.
	for _, ix := range []string{"node_kind", "node_section", "field_nv", "field_nn"} {
		var sqlText string
		err := db.QueryRow("SELECT sql FROM sqlite_master WHERE type='index' AND name=?", ix).Scan(&sqlText)
		if err == nil && !strings.Contains(strings.ToUpper(sqlText), "NOCASE") {
			if _, err := db.Exec("DROP INDEX " + ix); err != nil {
				db.Close()
				return nil, err
			}
		}
	}
	if _, err := db.Exec(schemaSQL); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func f32bytes(v []float32) []byte {
	b := make([]byte, 4*len(v))
	for i, x := range v {
		binary.LittleEndian.PutUint32(b[4*i:], math.Float32bits(x))
	}
	return b
}

func bytesF32(b []byte) []float32 {
	v := make([]float32, len(b)/4)
	for i := range v {
		v[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[4*i:]))
	}
	return v
}

func jsonStr(v any) string {
	if v == nil {
		return ""
	}
	d, err := json.Marshal(v)
	if err != nil || string(d) == "null" {
		return "" // a typed nil pointer marshals to "null"; keep the column genuinely empty
	}
	return string(d)
}

// WriteSQLite replaces the contents of db with this brain, in one transaction.
func (b *Brain) WriteSQLite(db *sql.DB) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.writeSQLite(db)
}

func (b *Brain) writeSQLite(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, t := range []string{"meta", "kind", "section", "node", "field", "edge", "vec", "stem", "source"} {
		if _, err := tx.Exec("DELETE FROM " + t); err != nil {
			return err
		}
	}
	meta := map[string]string{
		"schemaVersion": strconv.Itoa(SchemaVersion),
		"name":          b.Name,
		"goal":          b.Goal,
		"step":          strconv.Itoa(b.Step),
		"nextID":        strconv.Itoa(b.nextID),
		"params":        jsonStr(b.P),
		"embedding":     jsonStr(b.Emb),
		"focus":         strings.Join(b.seeds, ","),
		"goalFocus":     strings.Join(b.goal, ","),
	}
	for k, v := range meta {
		if _, err := tx.Exec("INSERT INTO meta (key, value) VALUES (?, ?)", k, v); err != nil {
			return err
		}
	}
	for _, k := range b.kinds {
		if k.Builtin {
			continue
		}
		hl := math.NaN()
		if k.HalfLife != nil {
			hl = *k.HalfLife
		}
		var hlv any
		if !math.IsNaN(hl) {
			hlv = hl
		}
		if _, err := tx.Exec("INSERT INTO kind (name, label, color, base, half_life, floor, prefix, section) VALUES (?,?,?,?,?,?,?,?)",
			k.Name, k.Label, k.Color, k.Base, hlv, k.Floor, k.Prefix, k.Section); err != nil {
			return err
		}
	}
	for i, s := range b.secs {
		if _, err := tx.Exec("INSERT INTO section (name, ord) VALUES (?, ?)", s, i); err != nil {
			return err
		}
	}
	nodeStmt, err := tx.Prepare(`INSERT INTO node
		(id, ord, kind, section, text, base, age, touched, pinned, sup, parent, idx, src, hash, labels, meta, purpose, conn, ext)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer nodeStmt.Close()
	fieldStmt, err := tx.Prepare("INSERT INTO field (node_id, ord, name, value, num) VALUES (?,?,?,?,?)")
	if err != nil {
		return err
	}
	defer fieldStmt.Close()
	stemStmt, err := tx.Prepare("INSERT INTO stem (stem, node_id) VALUES (?, ?)")
	if err != nil {
		return err
	}
	defer stemStmt.Close()
	for i, id := range b.order {
		n := b.nodes[id]
		if n == nil {
			continue
		}
		pin := 0
		if n.Pin {
			pin = 1
		}
		if _, err := nodeStmt.Exec(n.ID, i, n.Kind, n.Section, n.Text, n.Base, b.ageNow(n), b.touchOf(n), pin,
			n.Sup, n.Parent, n.Idx, n.Src, n.Hash, strings.Join(n.Labels, ","),
			jsonStr(n.Meta), jsonStr(n.Purpose), jsonStr(n.Conn), jsonStr(n.Ext)); err != nil {
			return err
		}
		for j, f := range n.Fields {
			var num any
			if v, ok := parseNum(f.Value); ok {
				num = v
			}
			if _, err := fieldStmt.Exec(n.ID, j, f.Name, f.Value, num); err != nil {
				return err
			}
		}
		// Inverted stem index (F4): text search and graph seeding without a full scan.
		if n.Conn == nil {
			for s := range stems(n.Text) {
				if _, err := stemStmt.Exec(s, n.ID); err != nil {
					return err
				}
			}
		}
	}
	edgeStmt, err := tx.Prepare("INSERT INTO edge (ord, a, b, type, w) VALUES (?,?,?,?,?)")
	if err != nil {
		return err
	}
	defer edgeStmt.Close()
	for i, e := range b.edges {
		if _, err := edgeStmt.Exec(i, e.A, e.B, e.Type, e.W); err != nil {
			return err
		}
	}
	vecStmt, err := tx.Prepare("INSERT INTO vec (node_id, kind, ord, dim, data) VALUES (?,?,?,?,?)")
	if err != nil {
		return err
	}
	defer vecStmt.Close()
	for i, id := range b.rawOr {
		if v, ok := b.raw[id]; ok {
			if _, err := vecStmt.Exec(id, "raw", i, len(v), f32bytes(v)); err != nil {
				return err
			}
		}
	}
	enrIDs := make([]string, 0, len(b.enr))
	for id := range b.enr {
		enrIDs = append(enrIDs, id)
	}
	sort.Strings(enrIDs)
	for i, id := range enrIDs {
		v := b.enr[id]
		if _, err := vecStmt.Exec(id, "enr", i, len(v), f32bytes(v)); err != nil {
			return err
		}
	}
	for i, s := range b.sources {
		if _, err := tx.Exec("INSERT INTO source (name, ord, def) VALUES (?,?,?)", s.Name, i, jsonStr(s.Config())); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	// Statistics are refreshed with the data, not on every open: ANALYZE costs a full index pass and
	// a large brain is opened far more often than it is written.
	if _, err := db.Exec("ANALYZE"); err != nil {
		return err
	}
	// From here the database matches memory, so the planner may narrow candidates with SQL.
	b.sq, b.sqGen = db, b.gen
	return nil
}

// ReadSQLite replaces this brain's contents with the database's.
func (b *Brain) ReadSQLite(db *sql.DB) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.readSQLite(db)
}

func (b *Brain) readSQLite(db *sql.DB) error {
	meta := map[string]string{}
	rows, err := db.Query("SELECT key, value FROM meta")
	if err != nil {
		return err
	}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			rows.Close()
			return err
		}
		meta[k] = v
	}
	rows.Close()
	if v := meta["schemaVersion"]; v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > SchemaVersion {
			return errf("this database was written by a newer brainDB (schema %d, this build reads %d)", n, SchemaVersion)
		}
	}
	b.reset()
	b.Name, b.Goal = meta["name"], meta["goal"]
	b.Step, _ = strconv.Atoi(meta["step"])
	b.nextID, _ = strconv.Atoi(meta["nextID"])
	if s := meta["params"]; s != "" {
		_ = json.Unmarshal([]byte(s), &b.P)
	}
	if s := meta["embedding"]; s != "" {
		_ = json.Unmarshal([]byte(s), &b.Emb)
	}
	b.seeds = splitIDs(meta["focus"])
	b.goal = splitIDs(meta["goalFocus"])

	rows, err = db.Query("SELECT name, label, color, base, half_life, floor, prefix, section FROM kind")
	if err != nil {
		return err
	}
	for rows.Next() {
		k := &Kind{}
		var hl sql.NullFloat64
		if err := rows.Scan(&k.Name, &k.Label, &k.Color, &k.Base, &hl, &k.Floor, &k.Prefix, &k.Section); err != nil {
			rows.Close()
			return err
		}
		if hl.Valid {
			v := hl.Float64
			k.HalfLife = &v
		}
		b.addKind(*k)
	}
	rows.Close()

	rows, err = db.Query("SELECT name FROM section ORDER BY ord")
	if err != nil {
		return err
	}
	secs := []string{}
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			rows.Close()
			return err
		}
		secs = append(secs, s)
	}
	rows.Close()
	if len(secs) > 0 {
		b.secs = secs
	}

	fields := map[string]Fields{}
	rows, err = db.Query("SELECT node_id, name, value FROM field ORDER BY node_id, ord")
	if err != nil {
		return err
	}
	for rows.Next() {
		var id, name, value string
		if err := rows.Scan(&id, &name, &value); err != nil {
			rows.Close()
			return err
		}
		fields[id] = append(fields[id], Field{Name: name, Value: value})
	}
	rows.Close()

	rows, err = db.Query(`SELECT id, kind, section, text, base, age, touched, pinned, sup, parent, idx, src, hash,
		labels, meta, purpose, conn, ext FROM node ORDER BY ord`)
	if err != nil {
		return err
	}
	for rows.Next() {
		n := &Node{}
		var pin int
		var labels, metaJSON, purposeJSON, connJSON, extJSON string
		if err := rows.Scan(&n.ID, &n.Kind, &n.Section, &n.Text, &n.Base, &n.Age, &n.Touched, &pin, &n.Sup,
			&n.Parent, &n.Idx, &n.Src, &n.Hash, &labels, &metaJSON, &purposeJSON, &connJSON, &extJSON); err != nil {
			rows.Close()
			return err
		}
		n.Pin = pin != 0
		n.Labels = splitIDs(labels)
		n.Fields = fields[n.ID]
		unmarshalInto(metaJSON, &n.Meta)
		unmarshalInto(purposeJSON, &n.Purpose)
		unmarshalInto(connJSON, &n.Conn)
		unmarshalInto(extJSON, &n.Ext)
		b.addNode(n)
	}
	rows.Close()

	rows, err = db.Query("SELECT a, b, type, w FROM edge ORDER BY ord")
	if err != nil {
		return err
	}
	for rows.Next() {
		e := &Edge{}
		if err := rows.Scan(&e.A, &e.B, &e.Type, &e.W); err != nil {
			rows.Close()
			return err
		}
		b.edges = append(b.edges, e)
		b.adj[e.A] = append(b.adj[e.A], e)
		b.adj[e.B] = append(b.adj[e.B], e)
	}
	rows.Close()

	rows, err = db.Query("SELECT node_id, kind, data FROM vec ORDER BY kind, ord")
	if err != nil {
		return err
	}
	for rows.Next() {
		var id, kind string
		var data []byte
		if err := rows.Scan(&id, &kind, &data); err != nil {
			rows.Close()
			return err
		}
		if kind == "raw" {
			b.raw[id] = bytesF32(data)
			b.rawOr = append(b.rawOr, id)
		} else {
			b.enr[id] = bytesF32(data)
		}
	}
	rows.Close()

	rows, err = db.Query("SELECT def FROM source ORDER BY ord")
	if err != nil {
		return err
	}
	for rows.Next() {
		var def string
		if err := rows.Scan(&def); err != nil {
			rows.Close()
			return err
		}
		var c SourceConfig
		if err := json.Unmarshal([]byte(def), &c); err == nil {
			b.sources = append(b.sources, c.toSource())
		}
	}
	rows.Close()
	b.absC = nil
	b.sq, b.sqGen = db, b.gen
	return nil
}

func unmarshalInto(s string, v any) {
	if s == "" || s == "null" {
		return
	}
	_ = json.Unmarshal([]byte(s), v)
}

// sqliteEmpty reports whether a freshly opened database has no brain in it yet.
func sqliteEmpty(db *sql.DB) (bool, error) {
	var n int
	if err := db.QueryRow("SELECT count(*) FROM meta").Scan(&n); err != nil {
		return false, err
	}
	return n == 0, nil
}

// MigrateTo writes this store's brain into the other backend next to it and returns the new path.
// Both directions are lossless: the XML of the migrated brain equals the XML of this one.
func (s *Store) MigrateTo(backend string) (string, error) {
	if s.Path == "" {
		return "", errf("this brain has no file to migrate")
	}
	if backend != "file" && backend != "sqlite" {
		return "", errf("unknown backend %q (file or sqlite)", backend)
	}
	if backend == s.BackendName() {
		return s.Path, nil
	}
	base := strings.TrimSuffix(s.Path, filepath.Ext(s.Path))
	s.mu.Lock()
	defer s.mu.Unlock()
	if backend == "sqlite" {
		out := base + ".db"
		db, err := OpenSQLiteDB(out)
		if err != nil {
			return "", err
		}
		defer db.Close()
		if err := s.B.WriteSQLite(db); err != nil {
			return "", err
		}
		return out, nil
	}
	out := base + ".xml"
	if err := os.WriteFile(out+".tmp", []byte(s.B.XML()), 0o644); err != nil {
		return "", err
	}
	if err := os.Rename(out+".tmp", out); err != nil {
		return "", err
	}
	return out, nil
}
