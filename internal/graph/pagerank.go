// Package graph implements the dependency-free personalized PageRank used for
// both file/package importance and Aider-style symbol ranking. It is a direct
// port of agentmap's power iteration: deterministic (stable node order, no
// PRNG), with dangling mass + teleport routed through the personalization
// vector (Aider parity).
package graph

// Tuning constants — kept identical to the JS implementation.
const (
	Damping = 0.85
	Tol     = 1e-6
	MaxIter = 100
)

// Edge is a weighted directed edge. Rank flows from→to, so with
// importer→imported edges, heavily-imported hubs rank high.
type Edge struct {
	From   string
	To     string
	Weight float64
}

// PageRank returns node → score. nodes fixes the (stable) ordering. If
// personalization is non-nil it is normalized and used as the teleport vector;
// otherwise the teleport is uniform.
func PageRank(nodes []string, edges []Edge, personalization map[string]float64) map[string]float64 {
	n := len(nodes)
	out := make(map[string]float64, n)
	if n == 0 {
		return out
	}
	idx := make(map[string]int, n)
	for i, node := range nodes {
		idx[node] = i
	}
	outW := make([]float64, n)
	type adjEntry struct {
		to int
		w  float64
	}
	adj := make([][]adjEntry, n)
	for _, e := range edges {
		a, okA := idx[e.From]
		b, okB := idx[e.To]
		if !okA || !okB || a == b { // skip self-loops
			continue
		}
		w := e.Weight
		if w <= 0 {
			w = 1
		}
		adj[a] = append(adj[a], adjEntry{to: b, w: w})
		outW[a] += w
	}

	// teleport vector p
	p := make([]float64, n)
	if personalization != nil {
		var s float64
		for k, v := range personalization {
			if i, ok := idx[k]; ok && v > 0 {
				p[i] = v
				s += v
			}
		}
		if s == 0 {
			for i := range p {
				p[i] = 1.0 / float64(n)
			}
		} else {
			for i := range p {
				p[i] /= s
			}
		}
	} else {
		for i := range p {
			p[i] = 1.0 / float64(n)
		}
	}

	r := make([]float64, n)
	copy(r, p)
	for iter := 0; iter < MaxIter; iter++ {
		var dangling float64
		for i := 0; i < n; i++ {
			if outW[i] == 0 {
				dangling += r[i]
			}
		}
		next := make([]float64, n)
		for i := 0; i < n; i++ {
			next[i] = (1-Damping)*p[i] + Damping*dangling*p[i]
		}
		for i := 0; i < n; i++ {
			if outW[i] == 0 {
				continue
			}
			ri := Damping * r[i]
			for _, e := range adj[i] {
				next[e.to] += ri * (e.w / outW[i])
			}
		}
		var diff float64
		for i := 0; i < n; i++ {
			d := next[i] - r[i]
			if d < 0 {
				d = -d
			}
			diff += d
		}
		r = next
		if diff < Tol {
			break
		}
	}
	for i := 0; i < n; i++ {
		out[nodes[i]] = r[i]
	}
	return out
}
