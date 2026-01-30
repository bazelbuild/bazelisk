package main

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s <file>\n", os.Args[0])
		os.Exit(1)
	}

	path := os.Args[1]
	f, err := os.Open(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to open file %s for sha256 checksum calculation: %v\n", path, err)
		os.Exit(1)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		fmt.Fprintf(os.Stderr, "failed to compute sha256 of %s: %v\n", path, err)
		os.Exit(1)
	}

	fmt.Printf("%x\n", h.Sum(nil))
}
