// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

package bdql

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/Gabski/brainDB/internal/brain"
)

var errAdmin = errors.New("access denied: this statement needs admin")

// rb returns the brain as the current principal sees it.
func (x *execState) rb() *brain.Brain {
	if x.p == nil || x.p.Admin {
		return x.real
	}
	return x.real.ViewFor(x.p, x.r.dbName())
}

func (r *Runner) dbName() string {
	if r.DB != "" {
		return r.DB
	}
	return r.Store.Name
}

func (x *execState) admin() bool { return x.p == nil || x.p.Admin }

func (x *execState) access() (*brain.AccessConfig, error) {
	a := x.real.Access()
	if a == nil {
		return nil, errors.New("access control is not enabled for this database (set access.enabled in braindb.yaml)")
	}
	return a, nil
}

func cloneAccess(a *brain.AccessConfig) *brain.AccessConfig {
	d, _ := json.Marshal(a)
	c := &brain.AccessConfig{}
	_ = json.Unmarshal(d, c)
	return c
}

func (x *execState) saveAccess(a *brain.AccessConfig) error {
	if x.r.SaveAccess != nil {
		return x.r.SaveAccess(x.st, a)
	}
	return x.st.SaveAccess(a)
}

func (x *execState) accessStmt(s *Stmt) (*StmtResult, error) {
	switch s.Kind {
	case "WHOAMI":
		p := x.p
		if p == nil {
			p = brain.AdminPrincipal
		}
		m := brain.NewOMap().Set("user", p.Name).Set("admin", p.Admin).Set("roles", strings.Join(p.Roles, ",")).
			Set("labels", strings.Join(p.Labels, ",")).Set("dbs", strings.Join(p.DBs, ",")).Set("access", x.real.AccessEnabled())
		return rowsResult([]string{"user", "admin", "roles", "labels", "dbs", "access"}, []*brain.OMap{m}, nil, ""), nil
	case "AS":
		if !x.origAdmin {
			return nil, errAdmin
		}
		if x.real.Access() == nil {
			if s.Name == "admin" {
				return &StmtResult{Kind: "text", Text: "as user admin"}, nil
			}
			return nil, errors.New("access control is not enabled for this database")
		}
		p, err := x.real.PrincipalFor(s.Name)
		if err != nil {
			if s.Name == "admin" {
				p = brain.AdminPrincipal
			} else {
				return nil, err
			}
		}
		x.p = p
		x.changes = append(x.changes, "as-user:"+s.Name)
		return &StmtResult{Kind: "text", Text: "as user " + s.Name}, nil
	case "EXPLAIN":
		if !x.admin() {
			return nil, errAdmin
		}
		if _, err := x.access(); err != nil {
			return nil, err
		}
		id, err := x.id(s.Refs[0])
		if err != nil {
			return nil, err
		}
		p, err := x.real.PrincipalFor(s.Name)
		if err != nil {
			return nil, err
		}
		t, err := x.real.ExplainAccess(id, p, x.r.dbName())
		if err != nil {
			return nil, err
		}
		return &StmtResult{Kind: "text", Text: t}, nil
	case "CREATE", "ALTER", "DROP", "GRANT", "REVOKE", "DENY":
		if !x.admin() {
			return nil, errAdmin
		}
		a0, err := x.access()
		if err != nil {
			return nil, err
		}
		a := cloneAccess(a0)
		msg, err := x.modifyAccess(a, s)
		if err != nil {
			return nil, err
		}
		if err := x.saveAccess(a); err != nil {
			return nil, err
		}
		x.changes = append(x.changes, msg)
		return &StmtResult{Kind: "changes", Changes: []string{msg}}, nil
	}
	return nil, fmt.Errorf("unsupported statement %s", s.Kind)
}

func findRole(a *brain.AccessConfig, name string) *brain.Role {
	for i := range a.Roles {
		if a.Roles[i].Name == name {
			return &a.Roles[i]
		}
	}
	return nil
}

func (x *execState) modifyAccess(a *brain.AccessConfig, s *Stmt) (string, error) {
	switch s.Kind {
	case "CREATE":
		if s.Sub == "ROLE" {
			if findRole(a, s.Name) != nil {
				return "", fmt.Errorf("role %s exists", s.Name)
			}
			a.Roles = append(a.Roles, brain.Role{Name: s.Name})
			return "+role:" + s.Name, nil
		}
		if a.User(s.Name) != nil {
			return "", fmt.Errorf("user %s exists", s.Name)
		}
		for _, r := range s.Strs {
			if findRole(a, r) == nil && r != "admin" {
				return "", fmt.Errorf("no such role %s", r)
			}
		}
		u := brain.User{Name: s.Name, Roles: s.Strs, Labels: s.Words, TokenEnv: s.Opts["tokenEnv"]}
		if d := s.Opts["dbs"]; d != "" {
			u.DBs = splitList(d)
		}
		a.Users = append(a.Users, u)
		return "+user:" + s.Name, nil
	case "ALTER":
		u := a.User(s.Name)
		if u == nil {
			return "", fmt.Errorf("no such user %s", s.Name)
		}
		for _, w := range s.Words {
			f := strings.Fields(w)
			switch {
			case w == "DISABLE":
				u.Disabled = true
			case w == "ENABLE":
				u.Disabled = false
			case f[0] == "TOKEN":
				u.TokenEnv = f[1]
			case f[1] == "ROLE":
				if f[0] == "ADD" {
					if findRole(a, f[2]) == nil && f[2] != "admin" {
						return "", fmt.Errorf("no such role %s", f[2])
					}
					u.Roles = uniqAppend(u.Roles, f[2])
				} else {
					u.Roles = remove(u.Roles, f[2])
				}
			case f[1] == "LABEL":
				if f[0] == "ADD" {
					u.Labels = uniqAppend(u.Labels, f[2])
				} else {
					u.Labels = remove(u.Labels, f[2])
				}
			default:
				return "", fmt.Errorf("ALTER USER: expected ADD|REMOVE ROLE|LABEL")
			}
		}
		return "~user:" + s.Name, nil
	case "DROP":
		if s.Sub == "ROLE" {
			out := []brain.Role{}
			found := false
			for _, r := range a.Roles {
				if r.Name == s.Name {
					found = true
					continue
				}
				out = append(out, r)
			}
			if !found {
				return "", fmt.Errorf("no such role %s", s.Name)
			}
			a.Roles = out
			return "-role:" + s.Name, nil
		}
		out := []brain.User{}
		found := false
		for _, u := range a.Users {
			if u.Name == s.Name {
				found = true
				continue
			}
			out = append(out, u)
		}
		if !found {
			return "", fmt.Errorf("no such user %s", s.Name)
		}
		a.Users = out
		return "-user:" + s.Name, nil
	case "GRANT", "DENY", "REVOKE":
		g := brain.Grant{Actions: s.Words, Labels: toAny(s.Strs), Deny: s.Kind == "DENY"}
		if v := s.Opts["sections"]; v != "" {
			g.Sections = splitList(v)
		}
		if v := s.Opts["kinds"]; v != "" {
			g.Kinds = splitList(v)
		}
		if v := s.Opts["dbs"]; v != "" {
			g.DBs = toAny(splitList(v))
		}
		var role *brain.Role
		if s.Sub == "ROLE" {
			role = findRole(a, s.Name)
			if role == nil {
				return "", fmt.Errorf("no such role %s", s.Name)
			}
		} else {
			u := a.User(s.Name)
			if u == nil {
				return "", fmt.Errorf("no such user %s", s.Name)
			}
			// per-user grants live in a personal role
			rn := "user:" + s.Name
			if role = findRole(a, rn); role == nil {
				a.Roles = append(a.Roles, brain.Role{Name: rn})
				role = &a.Roles[len(a.Roles)-1]
				u = a.User(s.Name)
				u.Roles = uniqAppend(u.Roles, rn)
			}
		}
		key := func(x brain.Grant) string { d, _ := json.Marshal(x); return string(d) }
		if s.Kind == "REVOKE" {
			out := []brain.Grant{}
			n := 0
			for _, x := range role.Grants {
				if !x.Deny && sameSet(x.Actions, g.Actions) && sameSet(anyStrs(x.Labels), s.Strs) {
					n++
					continue
				}
				out = append(out, x)
			}
			if n == 0 {
				return "", fmt.Errorf("no matching grant on role %s", role.Name)
			}
			role.Grants = out
			return fmt.Sprintf("-grant:%s:%s:%s", role.Name, strings.Join(g.Actions, ","), strings.Join(s.Strs, ",")), nil
		}
		for _, x := range role.Grants {
			if key(x) == key(g) {
				return "=grant (already present)", nil
			}
		}
		role.Grants = append(role.Grants, g)
		mark := "+grant"
		if g.Deny {
			mark = "+deny"
		}
		return fmt.Sprintf("%s:%s:%s:%s", mark, role.Name, strings.Join(g.Actions, ","), strings.Join(s.Strs, ",")), nil
	}
	return "", fmt.Errorf("unsupported")
}

func toAny(xs []string) []any {
	out := make([]any, len(xs))
	for i, x := range xs {
		out[i] = x
	}
	return out
}

func anyStrs(v any) []string {
	switch t := v.(type) {
	case string:
		return []string{t}
	case []any:
		out := []string{}
		for _, e := range t {
			out = append(out, fmt.Sprint(e))
		}
		return out
	case []string:
		return t
	}
	return nil
}

func sameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	m := map[string]bool{}
	for _, x := range a {
		m[x] = true
	}
	for _, x := range b {
		if !m[x] {
			return false
		}
	}
	return true
}

func uniqAppend(xs []string, x string) []string {
	for _, y := range xs {
		if y == x {
			return xs
		}
	}
	return append(xs, x)
}

func remove(xs []string, x string) []string {
	out := []string{}
	for _, y := range xs {
		if y != x {
			out = append(out, y)
		}
	}
	return out
}

// showAccess implements SHOW users | roles | labels | grants [FOR name].
func (x *execState) showAccess(s *Stmt) (*StmtResult, error) {
	a := x.real.Access()
	who := s.Opts["for"]
	switch s.Name {
	case "labels":
		used := x.real.LabelsInUse()
		rows := []*brain.OMap{}
		desc := map[string]string{}
		names := map[string]bool{}
		if a != nil {
			for _, l := range a.Labels {
				desc[l.Name] = l.Description
				names[l.Name] = true
			}
		}
		for l := range used {
			names[l] = true
		}
		ks := []string{}
		for k := range names {
			if x.admin() || containsStr(x.p.Labels, k) || containsStr(x.p.Labels, "*") {
				ks = append(ks, k)
			}
		}
		sort.Strings(ks)
		for _, k := range ks {
			n := used[k]
			if !x.admin() {
				n = 0
				for _, id := range x.rb().IDs() {
					if ls, _ := x.rb().LabelsOf(id); containsStr(ls, k) {
						n++
					}
				}
			}
			rows = append(rows, brain.NewOMap().Set("label", k).Set("description", desc[k]).Set("nodes", n))
		}
		return rowsResult([]string{"label", "description", "nodes"}, rows, nil, ""), nil
	case "users":
		if !x.admin() {
			return nil, errAdmin
		}
		rows := []*brain.OMap{}
		if a != nil {
			for _, u := range a.Users {
				rows = append(rows, brain.NewOMap().Set("name", u.Name).Set("display", u.Display).Set("roles", strings.Join(u.Roles, ",")).
					Set("labels", strings.Join(u.Labels, ",")).Set("dbs", strings.Join(u.DBs, ",")).Set("tokenEnv", u.TokenEnv).Set("disabled", u.Disabled))
			}
		}
		return rowsResult([]string{"name", "display", "roles", "labels", "dbs", "tokenEnv", "disabled"}, rows, nil, ""), nil
	case "roles", "grants":
		if a == nil {
			return rowsResult([]string{"role"}, nil, nil, ""), nil
		}
		roles := []brain.Role{}
		if who != "" {
			if !x.admin() && (x.p == nil || x.p.Name != who) {
				return nil, errAdmin
			}
			u := a.User(who)
			if u == nil {
				return nil, fmt.Errorf("no such user %s", who)
			}
			for _, rn := range u.Roles {
				if r := findRole(a, rn); r != nil {
					roles = append(roles, *r)
				} else if rn == "admin" {
					roles = append(roles, brain.Role{Name: "admin", Grants: []brain.Grant{{Actions: []string{"read", "write", "admin"}, Labels: "*"}}})
				}
			}
			if len(u.Labels) > 0 {
				roles = append(roles, brain.Role{Name: "(user labels)", Grants: []brain.Grant{{Actions: []string{"read"}, Labels: toAny(u.Labels)}}})
			}
		} else {
			if !x.admin() {
				return nil, errAdmin
			}
			roles = a.Roles
		}
		rows := []*brain.OMap{}
		for _, r := range roles {
			if s.Name == "roles" && who == "" {
				rows = append(rows, brain.NewOMap().Set("role", r.Name).Set("grants", len(r.Grants)))
				continue
			}
			for _, g := range r.Grants {
				eff := "allow"
				if g.Deny {
					eff = "deny"
				}
				rows = append(rows, brain.NewOMap().Set("role", r.Name).Set("effect", eff).Set("actions", strings.Join(g.Actions, ",")).
					Set("labels", strings.Join(anyStrs(g.Labels), ",")).Set("sections", strings.Join(g.Sections, ",")).Set("kinds", strings.Join(g.Kinds, ",")).
					Set("dbs", strings.Join(anyStrs(g.DBs), ",")))
			}
		}
		if s.Name == "roles" && who == "" {
			return rowsResult([]string{"role", "grants"}, rows, nil, ""), nil
		}
		return rowsResult([]string{"role", "effect", "actions", "labels", "sections", "kinds", "dbs"}, rows, nil, ""), nil
	}
	return nil, fmt.Errorf("unknown SHOW %s", s.Name)
}

func containsStr(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}
