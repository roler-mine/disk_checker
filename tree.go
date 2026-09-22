package diskchecker

import "errors"

// node represents a file or directory in the file system tree
type node struct {
	name       string  // name of the file or directory
	size       int64   // size of the file or directory in bytes
	hash       byte    // hash of the file or directory
	memAddress uintptr // memory address of the file or directory
	isDir      bool    // indicates if the node is a directory
	children   []*node // children of the directory
	parent     *node   // parent of the node
	err        error   // error associated with the node eg. permission denied
}

/*
AddChild adds a child node to the current node and sets the parent
of the child node to the current node
*/
func (n *node) AddChild(child *node) {
	child.parent = n
	n.children = append(n.children, child)
}

/*
IsRoot checks if the current node is the root of the tree
*/
func (n *node) IsRoot() bool {
	return n.parent == nil
}

/*
IsFile checks if the current node is a file (i.e., not a directory)
*/
func (n *node) IsFile() bool {
	return !n.isDir
}

/*
GetAbsolutePath returns the full path of the current node by
traversing up the tree to the root
*/
func (n *node) GetAbsolutePath() string {
	if n.IsRoot() {
		return n.name
	}
	return n.parent.GetAbsolutePath() + "/" + n.name
}

/*
GetRelativePath returns the relative path of the current node from target node
by traversing up the tree to a common ancestor and then down to the target node
returns an empty string if the current/target node is the root
*/
func (n *node) GetRelativePath(target *node) (string, error) {
	if n.IsRoot() || n == target || target.IsRoot() {
		return "", nil
	}
	if n.GetRoot() == target.GetRoot() {
		// find common ancestor
		nAncestors := make(map[*node]bool)
		for current := n; current != nil; current = current.parent {
			nAncestors[current] = true
		}
		for current := target; current != nil; current = current.parent {
			if nAncestors[current] {
				commonAncestor := current
				// build relative path from n to common ancestor
				var relativePath string
				for current := n; current != commonAncestor; current = current.parent {
					relativePath = "../" + relativePath
				}
				// build relative path from common ancestor to target
				for current := target; current != commonAncestor; current = current.parent {
					relativePath += "./" + current.name + "/"
				}
				return relativePath, nil
			}
		}
	}
	return "", errors.New("nodes are not in the same tree")
}

/*
IsEmptyDir checks if the current node is an empty directory
*/
func (n *node) IsEmptyDir() bool {
	return n.isDir && len(n.children) == 0
}

/*
Duplicates returns a dictionary of hash values as keys with amount of occurences as values
showing Duplicates in the file system
*/
func (n *node) Duplicates() map[byte]int {
	duplicateMap := make(map[byte]int)
	if n.IsFile() {
		duplicateMap[n.hash]++
	} else {
		for _, child := range n.children {
			childDuplicates := child.Duplicates()
			for hash, count := range childDuplicates {
				duplicateMap[hash] += count
			}
		}
	}
	return duplicateMap
}

/*
getRoot returns the root node of the tree by traversing up the tree
*/
func (n *node) GetRoot() *node {
	if n.IsRoot() {
		return n
	}
	return n.parent.GetRoot()
}

/*
RemoveNode removes the current node from its parent's children and returns the parent node
*/
func (n *node) RemoveNode() *node {
	if n.IsRoot() {
		return nil
	}
	parent := n.parent
	for i, child := range parent.children {
		if child == n {
			parent.children = append(parent.children[:i], parent.children[i+1:]...)
			break
		}
	}
	return parent
}

// Rename changes the name of the current node to the new name provided
func (n *node) Rename(newName string) {
	n.name = newName
}
