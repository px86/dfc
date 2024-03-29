package main

import (
	"bufio"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"sync"
)

func mergeChannels[T any](channels []<-chan T) <-chan T {
	merged := make(chan T)
	wg := sync.WaitGroup{}

	for _, channel := range channels {
		wg.Add(1)
		go func() {
			for msg := range channel {
				merged <- msg
			}
			wg.Done()
		}()
	}

	go func() {
		wg.Wait()
		close(merged)
	}()

	return merged
}

func listFiles(rootDir string) <-chan string {
	fsys := os.DirFS(rootDir)
	ch := make(chan string)

	go func() {
		fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				log.Printf("[ERROR] %s\n", err)
				return nil
			}
			if d.IsDir() {
				return nil
			}
			fullPath := filepath.Join(rootDir, path)
			ch <- fullPath
			return nil
		})
		close(ch)

	}()

	return ch
}

type fileHash struct {
	file  string
	value string
	err   error
}

func md5HashFile(file string) (string, error) {
	fin, err := os.Open(file)
	if err != nil {
		log.Printf("[ERROR] %s\n", err)
		return "", err
	}
	defer fin.Close()
	md5 := md5.New()
	if _, err := io.Copy(md5, fin); err != nil {
		log.Printf("[ERROR] %s\n", err)
		return "", err
	}
	return hex.EncodeToString(md5.Sum(nil)), nil
}

func md5Producer(stream <-chan string) <-chan fileHash {
	output := make(chan fileHash)
	go func() {
		for file := range stream {
			hash, err := md5HashFile(file)
			output <- fileHash{file: file, value: hash, err: err}

		}
		close(output)
	}()

	return output
}

func deleteFile(path string) {
	err := os.Remove(path)
	if err != nil {
		log.Println(err)
	} else {
		fmt.Printf("Deleted: %s\n", path)
	}
}

func askAndDelete(hashToFiles map[string][]string) {
	scanner := bufio.NewScanner(os.Stdin)
	counter := 0
	for _, files := range hashToFiles {
		if len(files) == 1 {
			continue
		}
		counter++
	}

	if counter == 0 {
		fmt.Printf("No duplicate files found.\n")
		return
	}

	fmt.Printf("Count of files having duplicates: %d\n", counter)

	for hash, files := range hashToFiles {
		if len(files) == 1 {
			continue
		}
		fmt.Printf("\nmd5hash: %s\n", hash)
		for index, file := range files {
			fmt.Printf("[%d] %s\n", index, file)
		}
		for {
			fmt.Printf("\nWhich file do you want to keep? ")
			scanner.Scan()
			input, err := strconv.Atoi(scanner.Text())
			if err != nil {
				continue
			}
			if input < len(files) && input >= 0 {
				for i, f := range files {
					if i != input {
						deleteFile(f)
					}
				}
				break
			}
		}
	}
}

func main() {
	root := "."
	if len(os.Args) > 1 {
		root = os.Args[1]
	}

	files := listFiles(root)

	var hashStreams []<-chan fileHash
	for i := 0; i < 10; i++ {
		hashStreams = append(hashStreams, md5Producer(files))
	}
	hashStream := mergeChannels(hashStreams)

	hashToFiles := make(map[string][]string)

	for h := range hashStream {
		file, hash, err := h.file, h.value, h.err
		if err != nil {
			continue
		}
		list, ok := hashToFiles[hash]
		if ok {
			hashToFiles[hash] = append(list, file)
		} else {
			hashToFiles[hash] = []string{file}
		}
	}

	askAndDelete(hashToFiles)
}
