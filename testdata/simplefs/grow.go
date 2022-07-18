package main

import (
	"fmt"
	"io"
	"os"
)

func main() {
	fn := os.Args[1]
	fmt.Printf("opening '%v'\n", fn)

	f, err := os.OpenFile(fn, os.O_RDONLY, 0644)
	if err != nil {
		fmt.Printf("unable to open file: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		if err := f.Close(); err != nil {
			fmt.Printf("unable to close file handle: %v\n", err)
		}
	}()

	s, err := f.Stat()
	if err != nil {
		fmt.Printf("unable to stat file: %v\n", err)
		os.Exit(1)
	}

	data, err := io.ReadAll(f)
	if err != nil {
		fmt.Printf("unable to read entire '%v' file: %v\n", fn, err)
		os.Exit(1)
	}

	fmt.Printf("read all %v bytes (len: %v) of '%v'\n", s.Size(), len(data), fn)
}
