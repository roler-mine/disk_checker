/*
main package diskchecker
*/

// diskchecker is a package that provides functionality to scan directories and build a tree structure representing the files and directories within a specified root directory. It includes methods to retrieve information about each entry, such as name, size, and whether it is a directory;
// It also provides methods to retrieve nodes by path or hash, delete nodes, and open files or directories. The package is designed to be used in applications that require file system scanning and analysis.
package diskchecker

import (
	"flag"
	"fmt"
	"os"
)

// Main is the program entry point: it reads the folder to scan from the command line
// (defaulting to the home directory) and opens the disk checker window.
//
//	diskchecker [folder]
func Main() {
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s [folder]\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	rootPath := flag.Arg(0)
	if rootPath == "" {
		rootPath = defaultRootPath()
	}

	// The scanner (scanner.go) runs in the background inside the UI, and the tree
	// operations (tree.go) back the Open, Rename and Delete buttons (render.go).
	Run(rootPath)
}

// defaultRootPath returns the home directory, or the working directory if it is unknown.
func defaultRootPath() string {
	if home, err := os.UserHomeDir(); err == nil {
		return home
	}
	if cwd, err := os.Getwd(); err == nil {
		return cwd
	}
	return "."
}
