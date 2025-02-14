package control

import (
	"errors"
	"github.com/edup2p/common/types/key"
	"github.com/edup2p/common/types/msgcontrol"
	"sync"
)

type EdgeGraph struct {
	mu    sync.RWMutex
	graph map[ClientID]map[ClientID]*VisibilityPair
}

func NewEdgeGraph() *EdgeGraph {
	return &EdgeGraph{
		graph: make(map[ClientID]map[ClientID]*VisibilityPair),
	}
}

func (g *EdgeGraph) UpsertEdge(from, to ClientID, pair *VisibilityPair) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if to == from {
		return errors.New("cannot insert pair to itself")
	}

	fromMap := g.graph[from]

	if fromMap == nil {
		fromMap = make(map[ClientID]*VisibilityPair)
		g.graph[from] = fromMap
	}

	toMap := g.graph[to]

	if toMap == nil {
		toMap = make(map[ClientID]*VisibilityPair)
		g.graph[to] = toMap
	}

	fromMap[to] = pair
	toMap[from] = pair

	return nil
}

func (g *EdgeGraph) RemoveEdge(from, to ClientID) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	// invariant: if g.graph[a][b] exists, then g.graph[b][a] also exists

	fromMap := g.graph[from]

	if fromMap == nil {
		// from does not exist
		return nil
	}

	toMap := g.graph[to]

	if toMap == nil {
		// to does not exist
		return nil
	}

	delete(fromMap, to)
	if len(fromMap) == 0 {
		delete(g.graph, from)
	}

	delete(toMap, from)
	if len(toMap) == 0 {
		delete(g.graph, to)
	}

	return nil
}

func (g *EdgeGraph) RemoveNode(node ClientID) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	// invariant: if g.graph[a][b] exists, then g.graph[b][a] also exists

	original := g.graph[node]

	if original == nil {
		// node does not exist
		return nil
	}

	var visitNodes []ClientID

	// Copy from the original map to the target map
	for k := range original {
		visitNodes = append(visitNodes, k)
	}

	for _, id := range visitNodes {
		delete(g.graph[id], node)
		if len(g.graph[id]) == 0 {
			delete(g.graph, id)
		}
	}

	delete(g.graph, node)

	return nil
}

// GetEdges gets all edges connected to node.
func (g *EdgeGraph) GetEdges(node ClientID) map[ClientID]VisibilityPair {
	g.mu.RLock()
	defer g.mu.RUnlock()

	// Create the target map
	targetMap := make(map[ClientID]VisibilityPair)

	originalMap := g.graph[node]

	if originalMap == nil {
		return nil
	}

	// Copy from the original map to the target map
	for k, v := range originalMap {
		targetMap[k] = *v
	}

	return targetMap
}

func (g *EdgeGraph) GetEdge(from, to ClientID) *VisibilityPair {
	g.mu.RLock()
	defer g.mu.RUnlock()

	targetMap := g.graph[from]

	if targetMap == nil {
		return nil
	}

	pair := targetMap[to]

	if pair != nil {
		pair = &(*pair)
	}

	return pair
}

type VisibilityPair struct {
	// One of the two ClientID's, or nil
	// Will quarantine all incoming connections FROM the referenced ClientID
	Quarantine *ClientID

	MDNS bool
}

func (vp *VisibilityPair) PropertiesFor(peer key.NodePublic) msgcontrol.Properties {
	p := msgcontrol.Properties{
		MDNS: vp.MDNS,
	}

	if vp.Quarantine != nil && *vp.Quarantine != ClientID(peer) {
		p.Quarantine = true
	}

	return p
}
