package kubernetes

import (
	"fmt"

	"go.uber.org/multierr"
)

// The following code has been lifted from https://github.com/ctfer-io/ctfops/blob/main/pkg/graphs/depgraph.go

type Resource interface {
	GetID() string
	GetDependencies() []string
}

// DepGraph is a dependency graph of resource.
type DepGraph[T Resource] struct {
	// Nodes maps a Node per its resource ID
	Nodes map[string]*Node[T]
}

// Node represents a resource
type Node[T Resource] struct {
	Res          T
	Dependencies []string
	Dependents   []string
}

// NewDepGraph builds a dependency graph of resource, with for each both
// the dependencies and dependents.
// It resolves dependencies thus ensure there is no missing nodes in the graph.
func NewDepGraph[T Resource](res []T) (*DepGraph[T], error) {
	dg := &DepGraph[T]{
		Nodes: map[string]*Node[T]{},
	}

	// Build nodes along with dependents
	for _, r := range res {
		dg.Nodes[r.GetID()] = &Node[T]{
			Res:          r,
			Dependencies: r.GetDependencies(),
			Dependents:   []string{},
		}
	}

	// Check dependencies could be resolved
	for rid, node := range dg.Nodes {
		for _, req := range node.Dependencies {
			if _, ok := dg.Nodes[req]; !ok {
				return nil, fmt.Errorf("resource %s references unexisting resource %s", rid, req)
			}
		}
	}

	// Revert process to build dependencies
	for rid, node := range dg.Nodes {
		for _, req := range node.Dependencies {
			dg.Nodes[req].Dependents = append(dg.Nodes[req].Dependents, rid)
		}
	}

	return dg, nil
}

// The following code has been lifted from https://github.com/ctfer-io/ctfops/blob/main/pkg/graphs/scc.go

func (dg DepGraph[T]) SCCs() [][]*Node[T] {
	// Build a simpler dependency graph
	ndg := map[string]*node{}
	for k, n := range dg.Nodes {
		ndg[k] = &node{
			resID:        n.Res.GetID(),
			dependencies: n.Dependencies,
		}
	}

	// Find the SCCs
	c := &cycle{
		index: 0,
		stack: []*node{},
		sccs:  [][]*node{},
		dg:    ndg,
	}
	c.find()

	// Map resulting SCCs to a manageable list
	sccs := make([][]*Node[T], 0, len(c.sccs))
	for _, scc := range c.sccs {
		nscc := make([]*Node[T], 0, len(scc))
		for _, n := range scc {
			nscc = append(nscc, dg.Nodes[n.resID])
		}
		sccs = append(sccs, nscc)
	}
	return sccs
}

// The following has been copied from https://github.com/pandatix/go-abnf/blob/main/dag.go

type depGraph map[string]*node

type node struct {
	resID          string
	index, lowlink int
	onStack        bool
	dependencies   []string
}

type cycle struct {
	index int
	stack []*node
	sccs  [][]*node

	dg depGraph
}

func (c *cycle) find() {
	for _, v := range c.dg {
		if v.index == 0 {
			c.strongconnect(v)
		}
	}
}

func (c *cycle) strongconnect(v *node) {
	// Set the depth index for v to the smallest unused index
	v.index = c.index
	v.lowlink = c.index
	c.index++
	c.stack = append(c.stack, v)
	v.onStack = true

	// Consider successors of v
	for _, dep := range v.dependencies {
		w, ok := c.dg[dep]
		if !ok {
			// core rules, as we know they won't have a cycle thus
			// no SCC, we don't need to recurse.
			continue
		}
		if w.index == 0 {
			// Successor w has not yet been visited; recurse on it
			c.strongconnect(w)
			v.lowlink = min(v.lowlink, w.lowlink)
		} else {
			if w.onStack {
				// Successor w is in stack S and hence in the current SCC
				// If w is not on stack, then (v, w) is an edge pointing
				// to an SCC already found and must be ignored.
				v.lowlink = min(v.lowlink, w.index)
			}
		}
	}
	// If v is a root node, pop the stack and generate an SCC
	if v.lowlink == v.index {
		scc := []*node{}
		w := (*node)(nil)
		for w == nil || v.resID != w.resID {
			w = c.stack[len(c.stack)-1]
			c.stack = c.stack[:len(c.stack)-1]
			w.onStack = false
			scc = append(scc, w)
		}
		c.sccs = append(c.sccs, scc)
	}
}

// The following code has been lifted from https://github.com/ctfer-io/ctfops/blob/main/pkg/graphs/sort.go

// Sort proceed to a topological sorting of the resources, and return
// an error if the references to other resources could not be satisfied
// or if a cycle is detected.
func Sort[T Resource](res []T) ([]T, error) {
	// Build the dependency graph of the incoming resources.
	// It will later be deteriorated to perform the topological sort.
	dg, err := NewDepGraph(res)
	if err != nil {
		return nil, err
	}

	// Compute SCCs
	sccs := dg.SCCs()
	var merr error
	for _, scc := range sccs {
		if len(scc) != 1 {
			var resStr string
			for _, n := range scc {
				resStr += n.Res.GetID() + ","
			}
			resStr = resStr[:len(resStr)-1] // cut trailing ","
			merr = multierr.Append(merr, fmt.Errorf("cycle detected between resources [%s]", resStr))
		}
	}
	if merr != nil {
		return nil, merr
	}
	// From now, no cycle is possible.

	// L ← Empty list that will contain the sorted elements
	l := []*Node[T]{}
	// S ← Set of all nodes with no incoming edge
	s := []*Node[T]{}
	for _, node := range dg.Nodes {
		if len(node.Dependencies) == 0 {
			s = append(s, node)
		}
	}

	// while S is not empty do
	for len(s) != 0 {
		// remove a node n from S
		n := s[0]
		s = s[1:]
		// add n to L
		l = append(l, n)
		// for each node m with an edge e from n to m do
		for _, mID := range n.Dependents {
			// remove edge e from the graph
			n.Dependents = remove(n.Dependents, mID)
			m := dg.Nodes[mID]
			m.Dependencies = remove(m.Dependencies, n.Res.GetID())
			// if m has no other incoming edges then
			if len(m.Dependencies) == 0 {
				// insert m into S
				s = append(s, m)
			}
		}
	}

	// return L (a topologically sorted order)
	sorted := make([]T, 0, len(res))
	for _, ch := range l {
		sorted = append(sorted, ch.Res)
	}
	return sorted, nil
}

func remove(slc []string, k string) []string {
	out := make([]string, 0, len(slc)-1)
	for _, v := range slc {
		if v == k {
			continue
		}
		out = append(out, v)
	}
	return out
}
