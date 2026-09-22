package diskchecker

import (
	"fmt"
	"os"
	"unsafe"
)

// Scanner represents a file system scanner that builds a tree structure of nodes starting from a root node
type Scanner struct {
	root *node // root node of the file system tree
}

// NewScanner creates a new Scanner instance and initializes the file system tree starting from the given root path
func NewScanner(rootPath string) (*Scanner, error) {
	rootInfo, err := os.Stat(rootPath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat root path: %v", err)
	}

	rootNode := &node{
		name:       rootInfo.Name(),
		size:       rootInfo.Size(),
		hash:       byte(rootInfo.Size() % 64),
		parent:     nil,
		isDir:      rootInfo.IsDir(),
		memAddress: uintptr(unsafe.Pointer(&rootInfo)),
	}

	scanner := &Scanner{root: rootNode}
	err = scanner.ScanDirectory(rootPath, rootNode)
	if err != nil {
		return nil, fmt.Errorf("failed to scan directory: %v", err)
	}

	return scanner, nil
}

// ScanDirectory scans the directory at the given path and populates the tree structure
func (s *Scanner) ScanDirectory(path string, parentNode *node) error {
	entries, err := os.ReadDir(path)
	if err != nil {
		return fmt.Errorf("failed to read directory: %v", err)
	}

	for _, entry := range entries {
		entryInfo, erroring := entry.Info()

		childNode := &node{
			name:       entryInfo.Name(),
			size:       entryInfo.Size(),
			hash:       byte(entryInfo.Size() % 64), // simple hash based on size for demonstration
			memAddress: uintptr(unsafe.Pointer(&entryInfo)),
			isDir:      entryInfo.IsDir(),
			children:   []*node{},
			parent:     parentNode,
			err:        erroring,
		}
		parentNode.AddChild(childNode)
		if erroring != nil {
			return fmt.Errorf("failed to get entry info: %v", erroring)
		}

		if entryInfo.IsDir() {
			err = s.ScanDirectory(path+"/"+entryInfo.Name(), childNode)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (s *Scanner) GetRoot() *node {
	return s.root
}
