// a basic example of using Bisync through librclone ( https://pkg.go.dev/github.com/rclone/rclone/librclone/librclone )
// rc docs: https://rclone.org/rc/
package main

import (
	"fmt"
	"io"
	"log"
	"math/rand"
	"os"
	"path/filepath"

	"github.com/rclone/rclone/cmd/bisync/librclonebisync/librclonebisync"
)

func main() {
	Example()
}

// Example shows a basic example of how to use
func Example() {
	tempDir, path1, path2 := setup()

	ci := librclonebisync.GetDefaultOptions()
	ci.LogLevel = 6 // fs.LogLevelInfo
	ci.BackupDir = filepath.Join(tempDir, "backup")
	librclonebisync.SetOptions(ci)
	librclonebisync.CheckOptions()

	req := librclonebisync.NewBisyncRequest()
	req.Path1 = path1
	req.Path2 = path2
	req.Resync = true
	req.CreateEmptySrcDirs = true
	req.IgnoreListingChecksum = true
	req.Resilient = true

	librclonebisync.PrintBisyncRPC(req)
}

// just setting up the example
func setup() (tempDir, path1, path2 string) {
	tempDir = filepath.Join(os.TempDir(), "bisync-RPC-Example")
	path1 = filepath.Join(tempDir, "path1")
	path2 = filepath.Join(tempDir, "path2")
	writeFile(path1, "file1.txt", 723)
	writeFile(path2, "file2.txt", 826)
	return tempDir, path1, path2
}

// writeFile writes a random file at dir/name, just for the example
func writeFile(dir, name string, size int64) {
	err := os.MkdirAll(dir, 0777)
	if err != nil {
		log.Fatalf("Failed to make directory %q: %v", dir, err)
	}
	path := filepath.Join(dir, name)
	fd, err := os.Create(path)
	if err != nil {
		log.Fatalf("Failed to open file %q: %v", path, err)
	}
	_, err = io.CopyN(fd, rand.New(rand.NewSource(723)), size)
	if err != nil {
		log.Fatalf("Failed to write %v bytes to file %q: %v", size, path, err)
	}
	err = fd.Close()
	if err != nil {
		log.Fatalf("Failed to close file %q: %v", path, err)
	}
	fmt.Fprintf(os.Stdout, "%s: Written file size %v", path, size)
}
