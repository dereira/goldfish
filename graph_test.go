package main

import (
	"math"
	"testing"
)

// TestRankTags_NoReferences verifies that when tags contain only definitions
// and no references, rankTags falls back to treating each def as its own ref
// and still returns a non-empty result.
func TestRankTags_NoReferences(t *testing.T) {
	tags := []Tag{
		{RelFname: "a.go", Fname: "a.go", Line: 0, Name: "Foo", Kind: kindDef},
		{RelFname: "a.go", Fname: "a.go", Line: 5, Name: "Bar", Kind: kindDef},
		{RelFname: "b.go", Fname: "b.go", Line: 0, Name: "Baz", Kind: kindDef},
	}

	got := rankTags(tags, nil, nil, nil)

	if len(got) == 0 {
		t.Error("rankTags() returned empty result for defs-only input; want non-empty")
	}

	// All returned tags should belong to the input files.
	validFiles := map[string]bool{"a.go": true, "b.go": true}
	for _, tag := range got {
		if !validFiles[tag.RelFname] {
			t.Errorf("rankTags() returned tag with unexpected RelFname %q", tag.RelFname)
		}
	}
}

// TestRankTags_EmptyInput verifies that rankTags handles an empty tag list
// without panicking and returns nil.
func TestRankTags_EmptyInput(t *testing.T) {
	got := rankTags(nil, nil, nil, nil)
	if got != nil {
		t.Errorf("rankTags(nil) = %v; want nil", got)
	}
}

// TestRankTags_ChatFilesExcluded verifies that tags belonging to chat files
// are excluded from the ranked output.
func TestRankTags_ChatFilesExcluded(t *testing.T) {
	tags := []Tag{
		{RelFname: "chat.go", Fname: "chat.go", Line: 0, Name: "ChatFunc", Kind: kindDef},
		{RelFname: "chat.go", Fname: "chat.go", Line: 0, Name: "ChatFunc", Kind: kindRef},
		{RelFname: "other.go", Fname: "other.go", Line: 0, Name: "OtherFunc", Kind: kindDef},
		{RelFname: "other.go", Fname: "other.go", Line: 0, Name: "ChatFunc", Kind: kindRef},
	}
	chatFiles := map[string]bool{"chat.go": true}

	got := rankTags(tags, chatFiles, nil, nil)

	for _, tag := range got {
		if tag.RelFname == "chat.go" {
			t.Errorf("rankTags() returned chat file tag: %+v", tag)
		}
	}
}

// TestPersonalizedPageRank_UniformWithoutPersonalization verifies that when no
// personalization is provided, all nodes in a symmetric graph receive equal rank.
func TestPersonalizedPageRank_UniformWithoutPersonalization(t *testing.T) {
	// Triangle: a→b, b→c, c→a - every node has one in-edge and one out-edge,
	// so uniform PageRank should give all nodes the same score.
	g := newGraph()
	g.addEdge(0, 1, 1)
	g.addEdge(1, 2, 1)
	g.addEdge(2, 0, 1)

	got := personalizedPageRank(g, nil, pageRankDamping, pageRankTolerance)

	if len(got) != 3 {
		t.Fatalf("expected ranks for 3 nodes, got %d", len(got))
	}
	// All ranks should be equal (within floating-point tolerance).
	r0, r1, r2 := got[0], got[1], got[2]
	const eps = 1e-4
	if math.Abs(r0-r1) > eps || math.Abs(r1-r2) > eps {
		t.Errorf("expected equal ranks for symmetric graph, got %v %v %v", r0, r1, r2)
	}
}

// TestPersonalizedPageRank_PersonalizationRaisesTargetNode verifies that a node
// given personalization weight ends up with a strictly higher rank than the same
// node without personalization.
func TestPersonalizedPageRank_PersonalizationRaisesTargetNode(t *testing.T) {
	// Linear chain: 0→1→2. Without personalization node 2 (sink) gets the
	// most walk time. With personalization on node 0, the teleport keeps
	// returning to 0, so node 0's rank rises relative to the unbiased run.
	g := newGraph()
	g.addEdge(0, 1, 1)
	g.addEdge(1, 2, 1)

	unbiased := personalizedPageRank(g, nil, pageRankDamping, pageRankTolerance)

	personalization := map[int64]float64{0: 1.0}
	biased := personalizedPageRank(g, personalization, pageRankDamping, pageRankTolerance)

	if biased[0] <= unbiased[0] {
		t.Errorf("personalized rank of node 0 (%v) should exceed unbiased rank (%v)", biased[0], unbiased[0])
	}
}

// TestPersonalizedPageRank_RanksSumToOne verifies that the output is a proper
// probability distribution (ranks sum to ~1).
func TestPersonalizedPageRank_RanksSumToOne(t *testing.T) {
	g := newGraph()
	g.addEdge(0, 1, 2)
	g.addEdge(1, 2, 1)
	g.addEdge(2, 0, 3)
	g.addEdge(0, 2, 1)

	got := personalizedPageRank(g, nil, pageRankDamping, pageRankTolerance)

	var sum float64
	for _, r := range got {
		sum += r
	}
	const eps = 1e-3
	if math.Abs(sum-1.0) > eps {
		t.Errorf("ranks sum to %v; want ~1.0", sum)
	}
}

// TestRankTags_MentionedFnamesRaisesRank verifies that mentionedFnames causes
// the mentioned file's PageRank score to increase relative to the unbiased run.
// We test this at the personalizedPageRank level directly, which is the mechanism
// rankTags uses to wire mentionedFnames into the ranking.
func TestRankTags_MentionedFnamesRaisesRank(t *testing.T) {
	// Build a graph where hub.go has strong inbound weight and leaf.go has none.
	// Without personalization leaf.go gets minimal rank.
	// With personalization on leaf.go its teleportation probability rises,
	// which must increase its PageRank score.
	g := newGraph()
	// hub.go (id=0) is referenced by four files (1-4)
	g.addEdge(1, 0, 1)
	g.addEdge(2, 0, 1)
	g.addEdge(3, 0, 1)
	g.addEdge(4, 0, 1)
	// leaf.go (id=5) has only a self-edge (isolated def)
	g.addEdge(5, 5, isolatedDefWeight)

	unbiased := personalizedPageRank(g, nil, pageRankDamping, pageRankTolerance)

	// Personalize on leaf.go (id=5)
	pv := map[int64]float64{5: 1.0}
	biased := personalizedPageRank(g, pv, pageRankDamping, pageRankTolerance)

	if biased[5] <= unbiased[5] {
		t.Errorf(
			"personalized rank of leaf.go (id=5) should exceed unbiased rank: biased=%v unbiased=%v",
			biased[5], unbiased[5],
		)
	}
}

// TestBuildPersonalizationVector_MentionedFileGetsWeight verifies that a file
// in mentionedFnames receives a positive weight in the personalization vector.
func TestBuildPersonalizationVector_MentionedFileGetsWeight(t *testing.T) {
	allRelFnames := map[string]bool{"a.go": true, "b.go": true, "c.go": true}
	nodeIDs := map[string]int64{"a.go": 0, "b.go": 1, "c.go": 2}

	pv := buildPersonalizationVector(
		allRelFnames,
		nil,
		map[string]bool{"b.go": true},
		nodeIDs,
	)

	if pv[1] == 0 {
		t.Error("buildPersonalizationVector: b.go (id=1) should have non-zero weight")
	}
	if pv[0] != 0 || pv[2] != 0 {
		t.Error("buildPersonalizationVector: unmentioned files should have zero weight")
	}
}

// TestBuildPersonalizationVector_NilWhenEmpty verifies that the function returns
// nil when neither chat files nor mentioned files are provided, signaling that
// uniform teleportation should be used.
func TestBuildPersonalizationVector_NilWhenEmpty(t *testing.T) {
	allRelFnames := map[string]bool{"a.go": true}
	nodeIDs := map[string]int64{"a.go": 0}

	pv := buildPersonalizationVector(allRelFnames, nil, nil, nodeIDs)
	if pv != nil {
		t.Errorf("expected nil personalization vector when no hints given, got %v", pv)
	}
}

// TestBuildPersonalizationVector_ChatAndMentionedNoDuplicate verifies that a
// file appearing in both chatRelFnames and mentionedFnames is not double-counted.
func TestBuildPersonalizationVector_ChatAndMentionedNoDuplicate(t *testing.T) {
	allRelFnames := map[string]bool{"a.go": true, "b.go": true}
	nodeIDs := map[string]int64{"a.go": 0, "b.go": 1}

	// b.go is both chat and mentioned
	pvBoth := buildPersonalizationVector(
		allRelFnames,
		map[string]bool{"b.go": true},
		map[string]bool{"b.go": true},
		nodeIDs,
	)
	pvChatOnly := buildPersonalizationVector(
		allRelFnames,
		map[string]bool{"b.go": true},
		nil,
		nodeIDs,
	)

	// Both should give b.go exactly one unit of personalization weight (max, not sum).
	if pvBoth[1] != pvChatOnly[1] {
		t.Errorf(
			"double membership should not increase weight: both=%v chatOnly=%v",
			pvBoth[1], pvChatOnly[1],
		)
	}
}
