// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"math"
)

// Vectors are float32 like the reference Float32Array; arithmetic is done in float64 and stored as float32
// so results match JavaScript bit for bit.

const embDim = 256

func vnorm(v []float32) []float32 {
	s := 0.0
	for _, x := range v {
		s += float64(x) * float64(x)
	}
	s = math.Sqrt(s)
	if s == 0 {
		s = 1
	}
	for i := range v {
		v[i] = float32(float64(v[i]) / s)
	}
	return v
}

func vcos(a, b []float32) float64 {
	if a == nil || b == nil {
		return 0
	}
	s := 0.0
	for i := range a {
		s += float64(a[i]) * float64(b[i])
	}
	return s
}

// HashEmbed is the reference local embedding: 5-letter stems + bigrams + char trigrams, signed FNV-1a hashing.
// It is deterministic, offline and free, but it is lexical, not semantic: it matches shared word shapes, not meaning.
// For synonym-level recall configure a remote embedder (see EmbedderOptions and embedding.provider in braindb.yaml).
func HashEmbed(text string) []float32 { return hashEmbedDim(text, embDim) }

func hashEmbedDim(text string, dim int) []float32 {
	if dim <= 0 {
		dim = embDim
	}
	v := make([]float32, dim)
	words := []string{}
	for _, w := range asciiWords(text) {
		if len(w) >= 3 {
			words = append(words, w)
		}
	}
	add := func(f string, wt float64) {
		h := hash(f)
		sign := 1.0
		if h&1 == 1 {
			sign = -1
		}
		i := (h >> 1) % uint32(dim)
		v[i] = float32(float64(v[i]) + sign*wt)
	}
	p5 := func(w string) string {
		if len(w) > 5 {
			return w[:5]
		}
		return w
	}
	for _, w := range words {
		add("w:"+p5(w), 1)
	}
	for i := 0; i < len(words)-1; i++ {
		add("b:"+p5(words[i])+"_"+p5(words[i+1]), .5)
	}
	s := " "
	for i, w := range words {
		if i > 0 {
			s += " "
		}
		s += w
	}
	s += " "
	for i := 0; i < len(s)-2; i++ {
		add("c:"+s[i:i+3], .2)
	}
	return vnorm(v)
}

// emb returns the configured embedder, defaulting to the built-in hash embedder.
func (b *Brain) emb() Embedder {
	if b.embedder == nil {
		return hashEmbedder{dim: embDim}
	}
	return b.embedder
}

// embed vectorises one text (query side: search, fill, purpose).
func (b *Brain) embed(text string) []float32 {
	v, err := b.emb().Embed([]string{text})
	if err != nil || len(v) == 0 || v[0] == nil {
		return hashEmbedDim(text, b.emb().Dim())
	}
	return v[0]
}

// embedBatch vectorises many texts in one round trip.
func (b *Brain) embedBatch(texts []string) [][]float32 {
	v, err := b.emb().Embed(texts)
	if err != nil {
		v = make([][]float32, len(texts))
	}
	for i := range v {
		if v[i] == nil {
			v[i] = hashEmbedDim(texts[i], b.emb().Dim())
		}
	}
	return v
}

// embedAll recomputes raw vectors (memories from text, concepts = label + weighted member mean) and enriches them.
// Every text goes through the embedder in one batch, so a remote provider costs one round trip per import, not one
// per node, and the on-disk cache means unchanged nodes are never re-embedded.
func (b *Brain) embedAll() {
	b.absC = nil
	raw := map[string][]float32{}
	order := []string{}
	ids, texts := []string{}, []string{}
	for _, id := range b.order {
		if n := b.nodes[id]; n.Kind != Concept && n.Conn == nil {
			ids, texts = append(ids, id), append(texts, n.Text)
		}
	}
	cids := b.concepts()
	for _, id := range cids {
		texts = append(texts, b.nodes[id].Text)
	}
	vecs := b.embedBatch(texts)
	for i, id := range ids {
		raw[id], order = vecs[i], append(order, id)
	}
	for ci, id := range cids {
		// copy: the concept vector is mutated below and vecs may alias entries held in the embedder cache
		v := append([]float32(nil), vecs[len(ids)+ci]...)
		type mw struct {
			m []float32
			w float64
		}
		ms := []mw{}
		for _, e := range b.adj[id] {
			if e.Type != EdgeConcept {
				continue
			}
			if m := raw[other(e, id)]; m != nil {
				ms = append(ms, mw{m, e.W})
			}
		}
		sw := 0.0
		for _, x := range ms {
			sw += x.w
		}
		if sw == 0 {
			sw = 1
		}
		for _, x := range ms {
			for k := range v {
				v[k] = float32(float64(v[k]) + float64(x.m[k])*x.w/sw)
			}
		}
		raw[id] = vnorm(v)
		order = append(order, id)
	}
	b.raw, b.rawOr = raw, order
	b.Emb.Dim, b.Emb.Model = b.emb().Dim(), b.emb().Name()
	b.enrich()
}

// enrich is the APPNP-style propagation: h = (1-α)·x + α·Σ w·h_n / Σ w (supersede edges ×0.3).
func (b *Brain) enrich() {
	H := b.raw
	for k := 0; k < b.Emb.Hops; k++ {
		N := make(map[string][]float32, len(b.raw))
		for _, id := range b.rawOr {
			x := b.raw[id]
			agg := make([]float32, len(x))
			sw := 0.0
			for _, e := range b.adj[id] {
				h := H[other(e, id)]
				if h == nil || isQuarantined(b.nodes[other(e, id)]) {
					continue
				}
				w := e.W
				if e.Type == EdgeSupersede {
					w *= .3
				}
				for i := range agg {
					agg[i] = float32(float64(agg[i]) + float64(h[i])*w)
				}
				sw += w
			}
			v := make([]float32, len(x))
			for i := range v {
				nb := float64(x[i])
				if sw != 0 {
					nb = float64(agg[i]) / sw
				}
				v[i] = float32((1-b.Emb.Alpha)*float64(x[i]) + b.Emb.Alpha*nb)
			}
			N[id] = vnorm(v)
		}
		H = N
	}
	b.enr = H
}

func (b *Brain) vecOf(id string) []float32 {
	if b.Emb.Use == "raw" {
		return b.raw[id]
	}
	return b.enr[id]
}

func (b *Brain) coherence(m map[string][]float32) float64 {
	s, n := 0.0, 0
	for _, e := range b.edges {
		if e.Type == EdgeSupersede {
			continue
		}
		x, y := m[e.A], m[e.B]
		if x != nil && y != nil {
			s += vcos(x, y)
			n++
		}
	}
	if n == 0 {
		return 0
	}
	return s / float64(n)
}

// pcaLayout is the "Vectors" view: 3D PCA by power iteration with deflation.
func (b *Brain) pcaLayout() map[string][3]float64 {
	out := map[string][3]float64{}
	ids := filterStr(b.order, func(id string) bool { return b.vecOf(id) != nil })
	if len(ids) < 4 {
		return out
	}
	n, d := len(ids), len(b.vecOf(ids[0]))
	mean := make([]float64, d)
	for _, id := range ids {
		for i, x := range b.vecOf(id) {
			mean[i] += float64(x) / float64(n)
		}
	}
	C := make([][]float64, n)
	for j, id := range ids {
		x := b.vecOf(id)
		C[j] = make([]float64, d)
		for i := range x {
			C[j][i] = float64(x[i]) - mean[i]
		}
	}
	dot := func(a, c []float64) float64 {
		s := 0.0
		for i := range a {
			s += a[i] * c[i]
		}
		return s
	}
	norm := func(v []float64) []float64 {
		s := math.Sqrt(dot(v, v))
		if s == 0 {
			s = 1
		}
		for i := range v {
			v[i] /= s
		}
		return v
	}
	comps := [][]float64{}
	for c := 0; c < 3; c++ {
		v := make([]float64, d)
		for i := range v {
			v[i] = math.Sin(float64(i)*float64(c+1)*1.7 + float64(c) + 1)
		}
		v = norm(v)
		for it := 0; it < 40; it++ {
			w := make([]float64, d)
			for _, x := range C {
				p := dot(x, v)
				for i := range w {
					w[i] += x[i] * p
				}
			}
			for _, u := range comps {
				dt := dot(w, u)
				for i := range w {
					w[i] -= dt * u[i]
				}
			}
			v = norm(w)
		}
		comps = append(comps, v)
	}
	P := make([][3]float64, n)
	mx := [3]float64{1e-6, 1e-6, 1e-6}
	for j, x := range C {
		for k := 0; k < 3; k++ {
			P[j][k] = dot(x, comps[k])
			mx[k] = math.Max(mx[k], math.Abs(P[j][k]))
		}
	}
	for j, id := range ids {
		out[id] = [3]float64{P[j][0] / mx[0] * 115, P[j][1] / mx[1] * 95, P[j][2] / mx[2] * 115}
	}
	return out
}
