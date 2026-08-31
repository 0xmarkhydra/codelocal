package orchestration

import "sort"

// Nodes returns a deterministic cloned DAG view for schedulers/projections.
func (d *TaskDAG) Nodes() []TaskNode {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make([]TaskNode, 0, len(d.nodes))
	for _, node := range d.nodes {
		out = append(out, node.Clone())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
