package docker

import (
	"fmt"
	"github.com/vemilyus/borg-collective/internal/drone/model"
	"strings"
)

type backupNode struct {
	Project  *model.ContainerBackupProject
	Backup   model.ContainerBackup
	NeededBy []*backupNode
	requires []*backupNode
}

func (n *backupNode) doesRequire(other *backupNode, path []string) (bool, []string) {
	path = append(path, n.Backup.ServiceName)
	for _, node := range n.requires {
		if node == other {
			return true, path
		} else {
			required, res := node.doesRequire(other, path)
			if required {
				return required, res
			}
		}
	}

	return false, nil
}

func createBackupNodes(project *model.ContainerBackupProject) ([]*backupNode, error) {
	nodes := make([]*backupNode, len(project.Containers))
	for _, backup := range project.Containers {
		nodes = append(nodes,
			&backupNode{
				project,
				backup,
				make([]*backupNode, len(backup.Dependencies)),
				make([]*backupNode, 0),
			},
		)
	}

	for _, node := range nodes {
		for _, depRaw := range node.Backup.Dependencies {
			depNode := findNodeForServiceName(nodes, depRaw)
			if depNode == nil {
				return nil, fmt.Errorf("dependency %s not found", depRaw)
			}

			node.requires = append(node.requires, depNode)
			depNode.NeededBy = append(depNode.NeededBy, node)
		}
	}

	for _, node := range nodes {
		if required, path := node.doesRequire(node, make([]string, 0)); required {
			return nil, fmt.Errorf("%s depends on itself via %s", node.Backup.ServiceName, strings.Join(path, " -> "))
		}
	}

	return nodes, nil
}

func findNodeForServiceName(nodes []*backupNode, serviceName string) *backupNode {
	for _, node := range nodes {
		if node.Backup.ServiceName == serviceName {
			return node
		}
	}

	return nil
}

func topologicalSort(nodes []*backupNode) []*backupNode {
	sortedNodes := make([]*backupNode, len(nodes))
	marked := make(map[*backupNode]bool)

	var visit func(node *backupNode)
	visit = func(node *backupNode) {
		if marked[node] {
			return
		}

		for _, dependent := range node.NeededBy {
			visit(dependent)
		}

		marked[node] = true

		sortedNodes = append([]*backupNode{node}, sortedNodes...)
	}

	for _, node := range nodes {
		if !marked[node] {
			visit(node)
		}
	}

	return sortedNodes
}
