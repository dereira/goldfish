package main

import (
	"math"
	"slices"
	"sort"
	"strings"

	gonumgraph "gonum.org/v1/gonum/graph"
	"gonum.org/v1/gonum/graph/iterator"
)

type graphNode struct{ id int64 }

func (n graphNode) ID() int64 { return n.id }

type edge struct {
	from, to int64
	weight   float64
}

func (e edge) From() gonumgraph.Node { return graphNode{e.from} }
func (e edge) To() gonumgraph.Node   { return graphNode{e.to} }
func (e edge) ReversedEdge() gonumgraph.Edge {
	return edge{from: e.to, to: e.from, weight: e.weight}
}
func (e edge) Weight() float64 { return e.weight }

// weightedDiGraph is a simple adjacency-list weighted directed graph.
type weightedDiGraph struct {
	nodes map[int64]struct{}
	out   map[int64]map[int64]float64 // from -> to -> accumulated weight
}

func newGraph() *weightedDiGraph {
	return &weightedDiGraph{
		nodes: map[int64]struct{}{},
		out:   map[int64]map[int64]float64{},
	}
}

func (g *weightedDiGraph) addEdge(from, to int64, weight float64) {
	g.nodes[from] = struct{}{}
	g.nodes[to] = struct{}{}
	if g.out[from] == nil {
		g.out[from] = map[int64]float64{}
	}
	g.out[from][to] += weight
}

func (g *weightedDiGraph) HasEdgeBetween(x, y int64) bool {
	_, ok := g.out[x][y]
	if ok {
		return true
	}
	_, ok = g.out[y][x]
	return ok
}

func (g *weightedDiGraph) HasEdgeFromTo(from, to int64) bool {
	_, ok := g.out[from][to]
	return ok
}

func (g *weightedDiGraph) Node(id int64) gonumgraph.Node {
	if _, ok := g.nodes[id]; ok {
		return graphNode{id}
	}
	return nil
}

func (g *weightedDiGraph) Nodes() gonumgraph.Nodes {
	ns := make([]gonumgraph.Node, 0, len(g.nodes))
	for id := range g.nodes {
		ns = append(ns, graphNode{id})
	}
	return iterator.NewOrderedNodes(ns)
}

func (g *weightedDiGraph) From(id int64) gonumgraph.Nodes {
	ns := make([]gonumgraph.Node, 0, len(g.out[id]))
	for to := range g.out[id] {
		ns = append(ns, graphNode{to})
	}
	return iterator.NewOrderedNodes(ns)
}

func (g *weightedDiGraph) To(id int64) gonumgraph.Nodes {
	var ns []gonumgraph.Node
	for from, tos := range g.out {
		if _, ok := tos[id]; ok {
			ns = append(ns, graphNode{from})
		}
	}
	return iterator.NewOrderedNodes(ns)
}

func (*weightedDiGraph) HasEdgeFromToLine(_, _, _ int64) bool { return false }

func (g *weightedDiGraph) Edge(from, to int64) gonumgraph.Edge {
	w, ok := g.out[from][to]
	if !ok {
		return nil
	}
	return edge{from: from, to: to, weight: w}
}

func (g *weightedDiGraph) WeightedEdge(from, to int64) gonumgraph.WeightedEdge {
	w, ok := g.out[from][to]
	if !ok {
		return nil
	}
	return edge{from: from, to: to, weight: w}
}

func (g *weightedDiGraph) Weight(xid, yid int64) (float64, bool) {
	w, ok := g.out[xid][yid]
	return w, ok
}
func (*weightedDiGraph) Edges() gonumgraph.Edges                 { return gonumgraph.Empty }
func (*weightedDiGraph) WeightedEdges() gonumgraph.WeightedEdges { return gonumgraph.Empty }
func (g *weightedDiGraph) WeightedEdgesFrom(id int64) gonumgraph.WeightedEdges {
	edges := make([]gonumgraph.WeightedEdge, 0, len(g.out[id]))
	for to, w := range g.out[id] {
		edges = append(edges, edge{from: id, to: to, weight: w})
	}
	return iterator.NewOrderedWeightedEdges(edges)
}

const (
	// isolatedDefWeight is the self-edge weight assigned to definitions that have
	// no cross-file references. It keeps them in the PageRank graph with a minimal
	// score rather than dropping them as dangling nodes.
	isolatedDefWeight = 0.1

	// chatFileMultiplier boosts edges from files currently being edited so that
	// symbols used by those files rank highest in the map.
	chatFileMultiplier = 50.0

	// noiseDefinerThreshold is the maximum number of files that can define a symbol
	// before it is considered too generic to rank (e.g. init, main, String).
	noiseDefinerThreshold = 5

	// meaningfulIdentMinLen is the minimum length for a domain-specific identifier.
	// Tokens shorter than this (i, err, ok, val) are common noise across all files.
	meaningfulIdentMinLen = 8

	// pageRankDamping is the probability the random surfer follows an edge rather
	// than jumping to a random node. Brin & Page chose 0.85 in their 1998 paper
	// and it remains the standard default.
	// https://snap.stanford.edu/class/cs224w-readings/Brin98Anatomy.pdf
	pageRankDamping = 0.85

	// pageRankTolerance is the convergence threshold for the power-iteration solver.
	// Tight enough for accurate results on typical repo sizes without excess iterations.
	pageRankTolerance = 1e-6
)

// symbolMaps holds the three index structures derived from a flat tag list.
type symbolMaps struct {
	defines      map[string]map[string]bool // ident → set of defining relFnames
	references   map[string][]string        // ident → []relFname (duplicates = frequency)
	definitions  map[[2]string][]Tag        // (relFname, ident) → tags
	allRelFnames map[string]bool
}

// buildSymbolMaps scans tags into lookup maps used by the ranking pipeline.
// When no references are found (some language queries emit only definitions),
// it falls back to treating each definition as a self-reference so the graph
// still has edges to rank.
func buildSymbolMaps(allTags []Tag) symbolMaps {
	sm := symbolMaps{
		defines:      map[string]map[string]bool{},
		references:   map[string][]string{},
		definitions:  map[[2]string][]Tag{},
		allRelFnames: map[string]bool{},
	}
	for _, t := range allTags {
		sm.allRelFnames[t.RelFname] = true
		switch t.Kind {
		case kindDef:
			if sm.defines[t.Name] == nil {
				sm.defines[t.Name] = map[string]bool{}
			}
			sm.defines[t.Name][t.RelFname] = true
			key := [2]string{t.RelFname, t.Name}
			sm.definitions[key] = append(sm.definitions[key], t)
		case kindRef:
			sm.references[t.Name] = append(sm.references[t.Name], t.RelFname)
		}
	}
	if len(sm.references) == 0 {
		for ident, definers := range sm.defines {
			for d := range definers {
				sm.references[ident] = append(sm.references[ident], d)
			}
		}
	}
	return sm
}

// identWeight returns the edge-weight multiplier for an identifier.
// Higher weight means its defining files rank higher in the output map.
func identWeight(ident string, definers, mentionedIdents map[string]bool) float64 {
	mul := 1.0
	if mentionedIdents[ident] {
		mul *= 10
	}
	// Boost meaningful domain-specific identifiers. Short generic tokens
	// (i, err, ok, val) are common across all files and carry little signal.
	// snake_case, kebab-case, and camelCase names of ≥8 characters are good
	// proxies for domain-specific symbols worth surfacing.
	hasUnderscore := strings.Contains(ident, "_")
	hasHyphen := strings.Contains(ident, "-")
	hasMixedCase := strings.IndexFunc(ident, func(r rune) bool { return r >= 'A' && r <= 'Z' }) >= 0 &&
		strings.IndexFunc(ident, func(r rune) bool { return r >= 'a' && r <= 'z' }) >= 0
	if len(ident) >= meaningfulIdentMinLen && (hasUnderscore || hasHyphen || hasMixedCase) {
		mul *= 10
	}
	if strings.HasPrefix(ident, "_") {
		mul *= 0.1
	}
	// Symbols defined in more than noiseDefinerThreshold files are ubiquitous
	// infrastructure tokens (init, main, String, new) - too generic to signal importance.
	if len(definers) > noiseDefinerThreshold {
		mul *= 0.1
	}
	return mul
}

type symbolGraph struct {
	g       *weightedDiGraph
	nodeIDs map[string]int64
	idNodes map[int64]string
	idents  map[string]bool // cross-referenced ident set
}

// buildSymbolGraph constructs the weighted directed graph used for PageRank.
// Nodes are files; edges run from the referencing file to the defining file,
// weighted by reference frequency and identifier specificity.
// Returns the graph, stable file↔node ID maps, and the set of cross-referenced idents.
func buildSymbolGraph(
	sm symbolMaps,
	chatRelFnames map[string]bool,
	mentionedIdents map[string]bool,
) symbolGraph {
	nodeIDs := map[string]int64{}
	idNodes := map[int64]string{}
	var nextID int64
	for relFname := range sm.allRelFnames {
		nodeIDs[relFname] = nextID
		idNodes[nextID] = relFname
		nextID++
	}

	g := newGraph()
	idents := map[string]bool{}

	for ident, definers := range sm.defines {
		if _, hasRef := sm.references[ident]; !hasRef {
			// Definitions with no cross-file references would be dangling nodes in
			// PageRank (score zero). A small self-edge gives them a minimal floor.
			for definer := range definers {
				g.addEdge(nodeIDs[definer], nodeIDs[definer], isolatedDefWeight)
			}
			continue
		}
		idents[ident] = true
	}

	for ident := range idents {
		definers := sm.defines[ident]
		mul := identWeight(ident, definers, mentionedIdents)

		refCounts := map[string]int{}
		for _, refFname := range sm.references[ident] {
			refCounts[refFname]++
		}
		for referencer, count := range refCounts {
			useMul := mul
			// References from chat files get a large boost so that symbols used by
			// the files currently being edited rank highest in the map.
			if chatRelFnames[referencer] {
				useMul *= chatFileMultiplier
			}
			// sqrt dampens high-frequency references: a utility referenced 100×
			// doesn't dominate 10 files each referencing it 10×.
			weight := useMul * math.Sqrt(float64(count))
			for definer := range definers {
				g.addEdge(nodeIDs[referencer], nodeIDs[definer], weight)
			}
		}
	}

	return symbolGraph{g: g, nodeIDs: nodeIDs, idNodes: idNodes, idents: idents}
}

// scoreDefinitions distributes each file's PageRank score across its symbol
// definitions, proportional to each edge's contribution to the file's total
// outgoing weight.
//
// gonum accumulates multi-edges into a single edge, so we re-derive per-ident
// scores by iterating over reference counts directly rather than reading graph edges.
func scoreDefinitions(
	sg symbolGraph,
	sm symbolMaps,
	ranked map[int64]float64,
	chatRelFnames map[string]bool,
	mentionedIdents map[string]bool,
) map[[2]string]float64 {
	defScores := map[[2]string]float64{}
	for ident := range sg.idents {
		refCounts := map[string]int{}
		for _, refFname := range sm.references[ident] {
			refCounts[refFname]++
		}
		for referencer, count := range refCounts {
			srcID := sg.nodeIDs[referencer]
			totalWeight := 0.0
			for _, w := range sg.g.out[srcID] {
				totalWeight += w
			}
			if totalWeight == 0 {
				continue
			}
			mul := identWeight(ident, sm.defines[ident], mentionedIdents)
			if chatRelFnames[referencer] {
				mul *= chatFileMultiplier
			}
			w := mul * math.Sqrt(float64(count))
			srcRank := ranked[srcID]
			for definer := range sm.defines[ident] {
				dstID := sg.nodeIDs[definer]
				if edgeWeight, _ := sg.g.Weight(srcID, dstID); edgeWeight == 0 {
					continue
				}
				key := [2]string{definer, ident}
				defScores[key] += srcRank * w / totalWeight
			}
		}
	}
	return defScores
}

// collectRankedTags sorts definition scores, collects the corresponding tags
// (skipping chat files), then appends any ranked files that produced no
// symbol definitions - ensuring graph-important files always appear in output.
func collectRankedTags(
	defScores map[[2]string]float64,
	definitions map[[2]string][]Tag,
	ranked map[int64]float64,
	idNodes map[int64]string,
	chatRelFnames map[string]bool,
) []Tag {
	type scored struct {
		key  [2]string
		rank float64
	}
	scoredList := make([]scored, 0, len(defScores))
	for k, v := range defScores {
		scoredList = append(scoredList, scored{k, v})
	}
	sort.Slice(scoredList, func(i, j int) bool {
		if scoredList[i].rank != scoredList[j].rank {
			return scoredList[i].rank > scoredList[j].rank
		}
		// Stable tie-break by key so output is deterministic.
		if scoredList[i].key[0] != scoredList[j].key[0] {
			return scoredList[i].key[0] < scoredList[j].key[0]
		}
		return scoredList[i].key[1] < scoredList[j].key[1]
	})

	seen := map[[2]string]bool{}
	var rankedTags []Tag
	for _, s := range scoredList {
		relFname, ident := s.key[0], s.key[1]
		if chatRelFnames[relFname] {
			continue
		}
		key := [2]string{relFname, ident}
		if seen[key] {
			continue
		}
		seen[key] = true
		rankedTags = append(rankedTags, definitions[key]...)
	}

	// Files that participated in PageRank but produced no ranked symbol definitions
	// are appended in rank order. This ensures every file the graph considers
	// important is represented in the output, even if its symbols didn't make the cut.
	type nodeRank struct {
		relFname string
		rank     float64
	}
	nodeRanks := make([]nodeRank, 0, len(ranked))
	for id, r := range ranked {
		nodeRanks = append(nodeRanks, nodeRank{idNodes[id], r})
	}
	sort.Slice(nodeRanks, func(i, j int) bool {
		return nodeRanks[i].rank > nodeRanks[j].rank
	})

	includedFnames := map[string]bool{}
	for _, t := range rankedTags {
		includedFnames[t.RelFname] = true
	}
	for _, nr := range nodeRanks {
		if !includedFnames[nr.relFname] && !chatRelFnames[nr.relFname] {
			rankedTags = append(rankedTags, Tag{RelFname: nr.relFname, Kind: kindFile})
			includedFnames[nr.relFname] = true
		}
	}

	return rankedTags
}

// matVecAdd computes dst = M·v + teleportWeight·p in place.
// M is a row-major n×n dense matrix stored as mat[i*n+j].
func matVecAdd(mat, v, p []float64, teleportWeight float64, dst []float64, n int) {
	for i := range dst {
		dst[i] = teleportWeight * p[i]
		for j := range n {
			dst[i] += mat[i*n+j] * v[j]
		}
	}
}

// buildTransitionMatrix constructs a column-stochastic n×n transition matrix
// for the given graph. Column j holds the probability of moving to each node
// from node j. Dangling nodes (no outgoing edges) spread weight uniformly.
func buildTransitionMatrix(g *weightedDiGraph, nodes []int64, indexOf map[int64]int, damp float64) []float64 {
	n := len(nodes)
	mat := make([]float64, n*n)
	dangleWeight := damp / float64(n)
	for j, from := range nodes {
		totalOut := 0.0
		for _, w := range g.out[from] {
			totalOut += w
		}
		if totalOut == 0 {
			for i := range nodes {
				mat[i*n+j] = dangleWeight
			}
		} else {
			for to, w := range g.out[from] {
				mat[indexOf[to]*n+j] += (w / totalOut) * damp
			}
		}
	}
	return mat
}

// buildTeleportVector returns a normalised teleportation distribution.
// If personalization is nil or sums to zero, a uniform distribution is used.
func buildTeleportVector(nodes []int64, personalization map[int64]float64) []float64 {
	p := make([]float64, len(nodes))
	var pSum float64
	for i, id := range nodes {
		p[i] = personalization[id]
		pSum += p[i]
	}
	if pSum == 0 {
		for i := range p {
			p[i] = 1.0 / float64(len(nodes))
		}
	} else {
		for i := range p {
			p[i] /= pSum
		}
	}
	return p
}

// personalizedPageRank runs a weighted power-iteration PageRank on g.
// personalization is an optional map from node ID to non-negative weight; when
// non-nil and non-empty it replaces the uniform teleportation distribution so
// the random surfer jumps to preferred nodes more often. The weights are
// normalised internally, so callers can use any positive scale.
// When personalization is nil or empty, a standard uniform teleportation is used.
//
// Algorithm: v_new = α·M·v_old + (1-α)·p
// where M is the column-stochastic transition matrix and p is the teleportation
// vector. Iterates until the L2 norm of the change is below tol.
//
//nolint:unparam // damp and tol are intentionally parameterised for testability
func personalizedPageRank(g *weightedDiGraph, personalization map[int64]float64, damp, tol float64) map[int64]float64 {
	nodes := make([]int64, 0, len(g.nodes))
	for id := range g.nodes {
		nodes = append(nodes, id)
	}
	slices.Sort(nodes)

	n := len(nodes)
	if n == 0 {
		return map[int64]float64{}
	}

	indexOf := make(map[int64]int, n)
	for i, id := range nodes {
		indexOf[id] = i
	}

	mat := buildTransitionMatrix(g, nodes, indexOf, damp)
	p := buildTeleportVector(nodes, personalization)
	teleportWeight := 1 - damp

	v := make([]float64, n)
	for i := range v {
		v[i] = 1.0 / float64(n)
	}
	next := make([]float64, n)

	for {
		matVecAdd(mat, v, p, teleportWeight, next, n)

		var diff float64
		for i := range next {
			d := next[i] - v[i]
			diff += d * d
		}
		next, v = v, next
		if math.Sqrt(diff) < tol {
			break
		}
	}

	ranks := make(map[int64]float64, n)
	for i, id := range nodes {
		ranks[id] = v[i]
	}
	return ranks
}

// buildPersonalizationVector builds the teleportation bias for PageRank.
// Chat files and mentioned files get equal weight here; the additional directional
// pull toward chat-adjacent symbols comes from the edge boost in buildSymbolGraph.
func buildPersonalizationVector(
	allRelFnames map[string]bool,
	chatRelFnames map[string]bool,
	mentionedFnames map[string]bool,
	nodeIDs map[string]int64,
) map[int64]float64 {
	if len(chatRelFnames) == 0 && len(mentionedFnames) == 0 {
		return nil
	}
	personalize := 100.0 / float64(len(allRelFnames))
	out := map[int64]float64{}
	for relFname := range allRelFnames {
		var p float64
		if chatRelFnames[relFname] {
			p += personalize
		}
		// max() semantics: a file in both chat and mentioned should not get double weight.
		if mentionedFnames[relFname] && personalize > p {
			p = personalize
		}
		if p > 0 {
			out[nodeIDs[relFname]] = p
		}
	}
	return out
}

// rankTags builds the symbol graph from tags, runs PageRank, and returns tags
// sorted by descending rank score (excluding chatRelFnames from output).
//
// The algorithm models the codebase as a directed graph: nodes are files, edges
// run from the referencing file to the defining file, weighted by how often and
// how meaningfully a symbol is referenced. PageRank then identifies which files
// are structurally central - transitively important because many important files
// depend on them. Personalization biases the walk toward files currently in the
// chat, so the map stays relevant to the active task.
//
// See https://aider.chat/2023/10/22/repomap.html for a high-level description
// of the approach.
func rankTags(
	allTags []Tag,
	chatRelFnames map[string]bool,
	mentionedFnames map[string]bool,
	mentionedIdents map[string]bool,
) []Tag {
	sm := buildSymbolMaps(allTags)
	if len(sm.allRelFnames) == 0 {
		return nil
	}

	sg := buildSymbolGraph(sm, chatRelFnames, mentionedIdents)
	if len(sg.g.nodes) == 0 {
		return nil
	}

	pv := buildPersonalizationVector(sm.allRelFnames, chatRelFnames, mentionedFnames, sg.nodeIDs)
	ranked := personalizedPageRank(sg.g, pv, pageRankDamping, pageRankTolerance)

	defScores := scoreDefinitions(sg, sm, ranked, chatRelFnames, mentionedIdents)
	return collectRankedTags(defScores, sm.definitions, ranked, sg.idNodes, chatRelFnames)
}
