package node_failure_controller

import (
	"sync"

	"k8s.io/api/core/v1"
)

// NodeMap is the interface to store local volumes
type NodeMap interface {
	GetNode(key string) *v1.Node

	UpdateNode(key string, node *v1.Node)

	DeleteNode(key string)
}

type nodeMap struct {
	// for guarding access to node map
	sync.RWMutex

	// nodeMap storage node map of unique node name and node obj
	nodeMap map[string]*v1.Node
}

// NewNodeMap returns new NodeMap which acts as a cache
// for holding Nodes.
func NewNodeMap() NodeMap {
	nodeMap := &nodeMap{}
	nodeMap.nodeMap = make(map[string]*v1.Node)
	return nodeMap
}

func (nm *nodeMap) GetNode(key string) *v1.Node {
	node, ok := nm.nodeMap[key]
	if ok {
		return node
	} else {
		return nil
	}
}

func (nm *nodeMap) UpdateNode(key string, node *v1.Node) {
	nm.Lock()
	defer nm.Unlock()

	nm.nodeMap[key] = node
}

func (nm *nodeMap) DeleteNode(key string) {
	nm.Lock()
	defer nm.Unlock()

	delete(nm.nodeMap, key)
}

func (nm *nodeMap) GetNodes() []*v1.Node {
	nm.Lock()
	defer nm.Unlock()

	nodes := []*v1.Node{}
	for _, node := range nm.nodeMap {
		nodes = append(nodes, node)
	}

	return nodes
}
