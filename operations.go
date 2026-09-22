// Operations provides methods to perform operations on the file system tree,
// such as retrieving nodes by path or hash,
// deleting nodes, and opening files or directories.

package diskchecker

import (
	"errors"
	"os"
)

// GetNodeByPath retrieves a node by its path in the file system tree
func (s *Scanner) GetNodeByPath(path string) (*node, error) {
	if s.root == nil {
		return nil, errors.New("scanner has not been initialized")
	}

	currentNode := s.root
	if currentNode.name != path {
		return nil, errors.New("path does not match root node")
	}

	for _, child := range currentNode.children {
		if child.name == path {
			return child, nil
		}
	}

	return nil, errors.New("node not found for the given path")
}

// GetNodeByHash retrieves a node by its hash value
func (s *Scanner) GetNodeByHash(hash byte) (*node, error) {
	if s.root == nil {
		return nil, errors.New("scanner has not been initialized")
	}

	// searchNode is a recursive function that traverses the tree to find a node with the given hash
	var searchNode func(n *node) *node
	searchNode = func(n *node) *node {
		if n.hash == hash {
			return n
		}
		for _, child := range n.children {
			if result := searchNode(child); result != nil {
				return result
			}
		}
		return nil
	}

	result := searchNode(s.root)
	if result == nil {
		return nil, errors.New("node not found for the given hash")
	}
	return result, nil
}

// DeleteNodeByPath deletes the node at the given path and removes it from the tree structure
func (s *Scanner) DeleteNodeByPath(path string) error {
	nodeToDelete, err := s.GetNodeByPath(path)
	if err != nil {
		return err
	}

	if nodeToDelete.parent == nil {
		return errors.New("cannot delete the root node")
	}

	os.RemoveAll(path)
	nodeToDelete.RemoveNode()
	return nil
}

// OpenNodeByPath opens the file or directory at the given path using the default system application
func (s *Scanner) OpenNodeByPath(path string) error {
	nodeToOpen, err := s.GetNodeByPath(path)
	if err != nil {
		return err
	}

	if nodeToOpen.isDir {
		os.StartProcess("explorer", []string{path}, &os.ProcAttr{})
		return nil
	}

	os.Open(path)
	return nil
}
