// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

package brain

// Access control (labels, users, roles, grants). A node is visible to a principal iff the principal holds every label
// on it. For non-admin principals every read runs on a filtered copy of the brain in which invisible nodes (and their
// edges) are deleted and vectors and activation are recomputed, so nothing observable depends on hidden knowledge:
// "view as P" equals "admin view of a copy with P-invisible nodes deleted" by construction.

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// AccessConfig is the access section of braindb.yaml (or access.yaml).
type AccessConfig struct {
	Enabled bool   `json:"enabled,omitempty"`
	File    string `json:"file,omitempty"`
	Labels  []struct {
		Name        string `json:"name"`
		Description string `json:"description,omitempty"`
	} `json:"labels,omitempty"`
	Defaults struct {
		Sections map[string][]string `json:"sections,omitempty"`
		Kinds    map[string][]string `json:"kinds,omitempty"`
		Import   map[string][]string `json:"import,omitempty"`
	} `json:"defaults"`
	Roles []Role         `json:"roles,omitempty"`
	Users []User         `json:"users,omitempty"`
	JWT   map[string]any `json:"jwt,omitempty"`
}

// Role is a named set of grants.
type Role struct {
	Name   string  `json:"name"`
	Grants []Grant `json:"grants,omitempty"`
}

// Grant allows (or denies) actions on labels, optionally narrowed to databases, sections and kinds.
type Grant struct {
	Actions  []string `json:"actions,omitempty"`
	Labels   any      `json:"labels,omitempty"` // "*" or list
	DBs      any      `json:"dbs,omitempty"`    // "*" or list
	Sections []string `json:"sections,omitempty"`
	Kinds    []string `json:"kinds,omitempty"`
	Sources  []string `json:"sources,omitempty"`
	Deny     bool     `json:"deny,omitempty"`
}

// User is a principal definition.
type User struct {
	Name               string   `json:"name"`
	Display            string   `json:"display,omitempty"`
	Roles              []string `json:"roles,omitempty"`
	Labels             []string `json:"labels,omitempty"`
	DefaultWriteLabels []string `json:"defaultWriteLabels,omitempty"`
	DBs                []string `json:"dbs,omitempty"`
	Disabled           bool     `json:"disabled,omitempty"`
	TokenEnv           string   `json:"tokenEnv,omitempty"`
}

func anyList(v any) []string {
	switch x := v.(type) {
	case nil:
		return nil
	case string:
		return []string{x}
	case []any:
		out := []string{}
		for _, e := range x {
			out = append(out, scalarString(e))
		}
		return out
	case []string:
		return x
	}
	return nil
}

func (a *AccessConfig) check() []ConfigError {
	errs := []ConfigError{}
	roles := map[string]bool{"admin": true}
	for _, r := range a.Roles {
		roles[r.Name] = true
		for i, g := range r.Grants {
			for _, act := range g.Actions {
				if act != "read" && act != "write" && act != "admin" && act != "review" && act != "purpose-override" {
					errs = append(errs, ConfigError{Path: fmt.Sprintf("access.roles.%s.grants[%d]", r.Name, i), Msg: "action " + act + " (use read, write, admin)"})
				}
			}
		}
	}
	known := map[string]bool{}
	for _, l := range a.Labels {
		known[l.Name] = true
	}
	for _, u := range a.Users {
		for _, r := range u.Roles {
			if !roles[r] {
				names := []string{}
				for k := range roles {
					names = append(names, k)
				}
				msg := fmt.Sprintf("unknown role %q", r)
				if c := Closest(r, names, 2); c != "" {
					msg += fmt.Sprintf(" (did you mean %q?)", c)
				}
				errs = append(errs, ConfigError{Path: "access.users." + u.Name, Msg: msg})
			}
		}
	}
	return errs
}

// Principal is the effective identity of a request.
type Principal struct {
	Name     string   `json:"name"`
	Display  string   `json:"display,omitempty"`
	Admin    bool     `json:"admin"`
	Roles    []string `json:"roles"`
	Labels   []string `json:"labels"` // labels readable everywhere (for display)
	DBs      []string `json:"dbs,omitempty"`
	Default  []string `json:"defaultWriteLabels,omitempty"`
	grants   []Grant
	Disabled bool `json:"disabled,omitempty"`
}

// AdminPrincipal sees everything.
var AdminPrincipal = &Principal{Name: "admin", Admin: true, Roles: []string{"admin"}, Labels: []string{"*"}}

func (a *AccessConfig) role(name string) *Role {
	for i := range a.Roles {
		if a.Roles[i].Name == name {
			return &a.Roles[i]
		}
	}
	if name == "admin" {
		return &Role{Name: "admin", Grants: []Grant{{Actions: []string{"read", "write", "admin"}, Labels: "*", DBs: "*"}}}
	}
	return nil
}

// User returns a user definition.
func (a *AccessConfig) User(name string) *User {
	for i := range a.Users {
		if a.Users[i].Name == name {
			return &a.Users[i]
		}
	}
	return nil
}

// Principal resolves a user to its effective grants.
func (a *AccessConfig) Principal(name string) (*Principal, error) {
	if a == nil || !a.Enabled {
		return AdminPrincipal, nil
	}
	u := a.User(name)
	if u == nil {
		return nil, fmt.Errorf("no such user %s", name)
	}
	p := &Principal{Name: u.Name, Display: u.Display, Roles: append([]string{}, u.Roles...), DBs: u.DBs, Default: u.DefaultWriteLabels, Disabled: u.Disabled}
	for _, rn := range u.Roles {
		r := a.role(rn)
		if r == nil {
			continue
		}
		for _, g := range r.Grants {
			p.grants = append(p.grants, g)
			if !g.Deny && contains(g.Actions, "admin") {
				p.Admin = true
			}
		}
	}
	if len(u.Labels) > 0 {
		p.grants = append(p.grants, Grant{Actions: []string{"read"}, Labels: toAnySlice(u.Labels)})
	}
	set := map[string]bool{}
	for _, g := range p.grants {
		if !g.Deny && contains(g.Actions, "read") && len(g.Sections) == 0 && len(g.Kinds) == 0 {
			for _, l := range anyList(g.Labels) {
				set[l] = true
			}
		}
	}
	p.Labels = sortedKeys(set)
	return p, nil
}

func toAnySlice(xs []string) []any {
	out := make([]any, len(xs))
	for i, x := range xs {
		out[i] = x
	}
	return out
}

func grantMatches(g Grant, action, label, section, kind, db string) bool {
	if !contains(g.Actions, action) {
		return false
	}
	ls := anyList(g.Labels)
	if !(contains(ls, "*") || contains(ls, label)) {
		return false
	}
	if dbs := anyList(g.DBs); len(dbs) > 0 && db != "" && !contains(dbs, "*") && !contains(dbs, db) {
		return false
	}
	if len(g.Sections) > 0 && !contains(g.Sections, section) {
		return false
	}
	if len(g.Kinds) > 0 && !contains(g.Kinds, kind) {
		return false
	}
	return true
}

// Can reports whether the principal may perform action on a label for a node in section/kind of db.
func (p *Principal) Can(action, label, section, kind, db string) bool {
	if p == nil || p.Admin {
		return true
	}
	for _, g := range p.grants {
		if g.Deny && grantMatches(g, action, label, section, kind, db) {
			return false
		}
	}
	for _, g := range p.grants {
		if !g.Deny && grantMatches(g, action, label, section, kind, db) {
			return true
		}
	}
	return false
}

// CanDB reports whether the principal may use a database.
func (p *Principal) CanDB(db string) bool {
	if p == nil || p.Admin || len(p.DBs) == 0 || db == "" {
		return true
	}
	return contains(p.DBs, db) || contains(p.DBs, "*")
}

// Visible reports whether a node is visible.
func (p *Principal) Visible(n *Node, db string) bool {
	if p == nil || p.Admin {
		return true
	}
	for _, l := range n.Labels {
		if l == QuarantineLabel {
			if !p.Can("review", "*", "", "", db) && !p.Can("review", QuarantineLabel, "", "", db) {
				return false
			}
			continue
		}
		if !p.Can("read", l, n.Section, n.Kind, db) {
			return false
		}
	}
	return true
}

// Explain describes why a node is visible or hidden for the principal.
func (p *Principal) Explain(n *Node, db string) string {
	if p.Admin {
		return fmt.Sprintf("visible: %s is an admin", p.Name)
	}
	if len(n.Labels) == 0 {
		return fmt.Sprintf("visible: %s carries no labels", n.ID)
	}
	reasons := []string{}
	ok := true
	for _, l := range n.Labels {
		denied := false
		for _, g := range p.grants {
			if g.Deny && grantMatches(g, "read", l, n.Section, n.Kind, db) {
				denied = true
				reasons = append(reasons, fmt.Sprintf("label %s: denied by a deny grant%s", l, narrowing(g)))
				break
			}
		}
		if denied {
			ok = false
			continue
		}
		allowed := false
		for _, g := range p.grants {
			if !g.Deny && grantMatches(g, "read", l, n.Section, n.Kind, db) {
				allowed = true
				reasons = append(reasons, fmt.Sprintf("label %s: granted (labels %v%s)", l, anyList(g.Labels), narrowing(g)))
				break
			}
		}
		if !allowed {
			ok = false
			reasons = append(reasons, fmt.Sprintf("label %s: no read grant for section %s / kind %s", l, n.Section, n.Kind))
		}
	}
	v := "hidden"
	if ok {
		v = "visible"
	}
	return fmt.Sprintf("%s: %s for %s (labels %s; roles %s)\n  %s", v, n.ID, p.Name, strings.Join(n.Labels, ","), strings.Join(p.Roles, ","), strings.Join(reasons, "\n  "))
}

func narrowing(g Grant) string {
	s := ""
	if len(g.Sections) > 0 {
		s += " in sections " + strings.Join(g.Sections, ",")
	}
	if len(g.Kinds) > 0 {
		s += " for kinds " + strings.Join(g.Kinds, ",")
	}
	return s
}

// Grants returns the principal's grants.
func (p *Principal) Grants() []Grant { return p.grants }

// key identifies the principal's view for caching.
func (p *Principal) key() string {
	parts := []string{p.Name}
	for _, g := range p.grants {
		parts = append(parts, fmt.Sprint(g))
	}
	return strings.Join(parts, "|")
}

// ---------------------------------------------------------------- brain integration

// AccessEnabled reports whether access control is on.
func (b *Brain) AccessEnabled() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.access != nil && b.access.Enabled
}

// Access returns the access configuration (nil when disabled).
func (b *Brain) Access() *AccessConfig {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.access == nil || !b.access.Enabled {
		return nil
	}
	return b.access
}

// SetAccess replaces the access configuration.
func (b *Brain) SetAccess(a *AccessConfig) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.access = a
	b.views = nil
}

// PrincipalFor resolves a user name ("" or unknown with access disabled → admin).
func (b *Brain) PrincipalFor(name string) (*Principal, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.access == nil || !b.access.Enabled {
		return AdminPrincipal, nil
	}
	if name == "" {
		return nil, fmt.Errorf("access control is enabled: a user is required")
	}
	return b.access.Principal(name)
}

type viewEntry struct {
	sig string
	b   *Brain
}

func (b *Brain) signature() string {
	return fmt.Sprintf("%d|%d|%d|%s|%v|%v|%s", b.Step, len(b.order), len(b.edges), strings.Join(b.seeds, ","), b.P, b.Emb, b.Goal)
}

// ViewFor returns the brain as seen by a principal: b itself for admins, otherwise a filtered copy (cached until the
// brain changes). The copy is a read-only snapshot.
func (b *Brain) ViewFor(p *Principal, db string) *Brain {
	b.mu.Lock()
	defer b.mu.Unlock()
	if p == nil || p.Admin || b.access == nil || !b.access.Enabled {
		return b
	}
	sig := b.signature()
	k := p.key() + "@" + db
	if v, ok := b.views[k]; ok && v.sig == sig {
		return v.b
	}
	c := b.clone()
	c.access = nil
	c.views = nil
	c.entCache = nil
	for _, id := range append([]string(nil), c.order...) {
		n := c.nodes[id]
		if n != nil && !p.Visible(n, db) {
			c.removeNode(id, false)
		}
	}
	// a connector that only joined invisible nodes disappears with them
	for _, id := range append([]string(nil), c.order...) {
		if n := c.nodes[id]; n != nil && n.Conn != nil && len(c.adj[id]) == 0 && len(b.adj[id]) > 0 {
			c.removeNode(id, false)
		}
	}
	c.recomputeNextID()
	c.embedAll()
	c.refresh()
	if b.views == nil {
		b.views = map[string]viewEntry{}
	}
	b.views[k] = viewEntry{sig, c}
	return c
}

// defaultLabels returns labels new nodes get from access defaults (section, kind, import path).
func (b *Brain) defaultLabels(section, kind, importPath string) []string {
	a := b.access
	if a == nil {
		return nil
	}
	out := []string{}
	out = append(out, a.Defaults.Sections[section]...)
	out = append(out, a.Defaults.Kinds[kind]...)
	if importPath != "" {
		ipath, ifrag, _ := strings.Cut(importPath, "#")
		for pat, ls := range a.Defaults.Import {
			base, frag, _ := strings.Cut(pat, "#")
			if frag != ifrag {
				continue
			}
			if m, _ := filepath.Match(base, ipath); m || base == ipath || filepath.Base(ipath) == base || strings.HasSuffix(ipath, "/"+base) || globMatch(base, ipath) {
				out = append(out, ls...)
			}
		}
	}
	return uniqSorted(out)
}

func uniqSorted(xs []string) []string {
	m := map[string]bool{}
	for _, x := range xs {
		if x != "" {
			m[x] = true
		}
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// CheckOps verifies that a principal may run a batch of exec ops: referenced nodes must be visible (otherwise the
// error is the same "no such item" as for a missing id); modified nodes and new labels need write grants.
func (b *Brain) CheckOps(p *Principal, db string, ops []Op) error {
	if p == nil || p.Admin {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.access == nil || !b.access.Enabled {
		return nil
	}
	tmp := map[string]bool{}
	seeVis := func(where, x string) error {
		if x == "" || strings.HasPrefix(x, "@") {
			return nil
		}
		n := b.nodes[x]
		if n == nil || !p.Visible(n, db) {
			if b.isExternalID(x) && n == nil {
				return nil
			}
			return errf("%s: no such item %s", where, x)
		}
		return nil
	}
	canWrite := func(where, x string) error {
		if err := seeVis(where, x); err != nil || x == "" || strings.HasPrefix(x, "@") {
			return err
		}
		n := b.nodes[x]
		for _, l := range n.Labels {
			if !p.Can("write", l, n.Section, n.Kind, db) {
				return errf("%s: access denied: %s needs write on label %s", where, x, l)
			}
		}
		if len(n.Labels) == 0 && !p.hasAnyWrite(db) {
			return errf("%s: access denied: %s has no write grant", where, p.Name)
		}
		return nil
	}
	for i, o := range ops {
		where := fmt.Sprintf("op#%d %s", i+1, o.Op)
		if o.Tmp != "" {
			tmp[o.Tmp] = true
		}
		refs := []string{o.A, o.B, o.Old, o.New, o.Supersedes}
		for _, l := range o.Links {
			refs = append(refs, l.To)
		}
		refs = append(append(append(append(append(refs, o.Members...), o.Connect...), o.IDs...), o.Add...), o.Remove...)
		for _, r := range refs {
			if err := seeVis(where, r); err != nil {
				return err
			}
		}
		switch o.Op {
		case "remember":
			sec := o.Section
			if k, ok := b.kindM[o.Kind]; ok && sec == "" {
				sec = k.Section
			}
			labels := uniqSorted(append(append(append([]string{}, p.Default...), o.Labels...), b.defaultLabels(sec, o.Kind, "")...))
			for _, l := range labels {
				if !p.Can("write", l, sec, o.Kind, db) {
					return errf("%s: access denied: no write grant for label %s", where, l)
				}
			}
			if len(labels) == 0 && !p.hasAnyWrite(db) {
				return errf("%s: access denied: %s has no write grant", where, p.Name)
			}
		case "boost", "weaken", "pin", "unpin", "edit", "forget":
			if err := canWrite(where, o.ID); err != nil {
				return err
			}
		case "supersede":
			if err := canWrite(where, o.Old); err != nil {
				return err
			}
		case "label":
			for _, id := range o.IDs {
				if err := canWrite(where, id); err != nil {
					return err
				}
			}
			for _, l := range append(append([]string{}, o.Add...), o.Remove...) {
				if !p.Can("write", l, "", "", db) {
					return errf("%s: access denied: no write grant for label %s", where, l)
				}
			}
		default:
			if o.ID != "" {
				if err := canWrite(where, o.ID); err != nil {
					return err
				}
			}
			if !p.hasAnyWrite(db) {
				return errf("%s: access denied: %s has no write grant", where, p.Name)
			}
		}
	}
	return nil
}

func (p *Principal) hasAnyWrite(db string) bool {
	if p.Admin {
		return true
	}
	for _, g := range p.grants {
		if !g.Deny && contains(g.Actions, "write") {
			if dbs := anyList(g.DBs); len(dbs) > 0 && db != "" && !contains(dbs, "*") && !contains(dbs, db) {
				continue
			}
			return true
		}
	}
	return false
}

// LabelsInUse counts nodes per label.
func (b *Brain) LabelsInUse() map[string]int {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := map[string]int{}
	for _, id := range b.order {
		for _, l := range b.nodes[id].Labels {
			out[l]++
		}
	}
	return out
}

// LabelsOf returns the labels of a node.
func (b *Brain) LabelsOf(id string) ([]string, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := b.nodes[id]
	if n == nil {
		return nil, false
	}
	return append([]string{}, n.Labels...), true
}

// ExplainAccess explains visibility of a node for a principal.
func (b *Brain) ExplainAccess(id string, p *Principal, db string) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := b.nodes[id]
	if n == nil {
		return "", errf("no such item %s", id)
	}
	return p.Explain(n, db), nil
}

// VisibleIDs filters ids to those a principal can see.
func (b *Brain) VisibleIDs(p *Principal, db string, ids []string) ([]string, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := []string{}
	hidden := false
	for _, id := range ids {
		if n := b.nodes[id]; n != nil && p.Visible(n, db) {
			out = append(out, id)
		} else if n != nil {
			hidden = true
		}
	}
	return out, hidden
}

// CanWriteAny reports whether the principal holds any write grant.
func (b *Brain) CanWriteAny(p *Principal, db string) bool { return p == nil || p.hasAnyWrite(db) }
