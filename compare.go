package main

import (
	"sort"
)

// Change is one entry of a comparison between two scans of the same path.
type Change struct {
	Path   string `json:"path"` // relative to the scan root
	IsDir  bool   `json:"dir"`
	Before int64  `json:"before"`
	After  int64  `json:"after"`
	Delta  int64  `json:"delta"`
}

type CompareSummary struct {
	Before  int64 `json:"before"`
	After   int64 `json:"after"`
	Delta   int64 `json:"delta"`
	Added   int   `json:"added"`
	Removed int   `json:"removed"`
	Changed int   `json:"changed"`
}

// compareTrees returns the nodes whose on-disk size differs between a (older)
// and b (newer), sorted by absolute delta, at most limit entries.
func compareTrees(a, b *FileNode, limit int) ([]Change, CompareSummary) {
	index := make(map[string]*FileNode)
	var walk func(n *FileNode, rel string)
	walk = func(n *FileNode, rel string) {
		index[rel] = n
		for _, c := range n.Children {
			walk(c, joinRel(rel, c.Name))
		}
	}
	walk(a, "")

	var changes []Change
	sum := CompareSummary{Before: a.Size, After: b.Size, Delta: b.Size - a.Size}
	seen := make(map[string]bool, len(index))

	var visit func(n *FileNode, rel string)
	visit = func(n *FileNode, rel string) {
		seen[rel] = true
		old, existed := index[rel]
		switch {
		case !existed:
			sum.Added++
			changes = append(changes, Change{Path: rel, IsDir: n.IsDir, After: n.Size, Delta: n.Size})
			return // children of a new node are implied
		case old.Size != n.Size:
			sum.Changed++
			changes = append(changes, Change{Path: rel, IsDir: n.IsDir, Before: old.Size, After: n.Size, Delta: n.Size - old.Size})
		}
		for _, c := range n.Children {
			visit(c, joinRel(rel, c.Name))
		}
	}
	visit(b, "")

	for rel, n := range index {
		if seen[rel] || rel == "" {
			continue
		}
		// Only report the topmost removed node, not every descendant.
		if parent := parentRel(rel); parent != "" && !seen[parent] {
			continue
		}
		sum.Removed++
		changes = append(changes, Change{Path: rel, IsDir: n.IsDir, Before: n.Size, Delta: -n.Size})
	}

	sort.Slice(changes, func(i, j int) bool {
		di, dj := abs64(changes[i].Delta), abs64(changes[j].Delta)
		if di != dj {
			return di > dj
		}
		return changes[i].Path < changes[j].Path
	})
	if limit > 0 && len(changes) > limit {
		changes = changes[:limit]
	}
	return changes, sum
}

func joinRel(rel, name string) string {
	if rel == "" {
		return name
	}
	return rel + "/" + name
}

func parentRel(rel string) string {
	for i := len(rel) - 1; i >= 0; i-- {
		if rel[i] == '/' {
			return rel[:i]
		}
	}
	return ""
}

func abs64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
