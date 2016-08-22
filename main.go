// Copyright 2016 Jacob Taylor jacob@ablox.io
// License: Apache2 - http://www.apache.org/licenses/LICENSE-2.0
package main

import (
	"fmt"
	"github.com/fsnotify/fsnotify"
	"log"
	"os"
	"path/filepath"
	"github.com/urfave/cli"
)

// settings for the server
type Settings struct {
    Directory    string
}

var globalSettings Settings = Settings {
	Directory: "",
}

func main() {
	fmt.Printf("replicat initializing....\n")

	app := cli.NewApp()
	app.Name = "Replicat"
	app.Usage = "rsync for the cloud"
	app.Action = func(c *cli.Context) error {
		globalSettings.Directory = c.GlobalString("directory")

		if globalSettings.Directory == "" {
			panic("Directory is required to serve files\n")
		}

		return nil
	}

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name: "directory, d",
			Value: globalSettings.Directory,
			Usage: "Specify a directory where the files to share are located.",
		},
	}

	app.Run(os.Args)

	fmt.Printf("replicat online....\n")
	fmt.Printf("serving files from: %s\n", globalSettings.Directory)

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}
	defer watcher.Close()
	defer fmt.Printf("End of line\n")

	done := make(chan bool)
	go func() {
		for {
			select {
			case event := <-watcher.Events:
				if event.Op & fsnotify.Write == fsnotify.Write {
					log.Println("file updated:", event.Name)
				} else {
					log.Println("event:", event)
				}
			case err := <-watcher.Errors:
				log.Println("error:", err)
			}
		}
	}()

	listOfFolders, err := createListOfFolders(globalSettings.Directory)
	if err != nil {
		log.Fatal(err)
	}

	for _, folder := range listOfFolders {
		err = watcher.Add(folder)
		if err != nil {
			log.Fatal(err)
		}
	}

	fmt.Printf("Now listing on: %d folders under: %s\n", len(listOfFolders), globalSettings.Directory)

	// Let's read the

	<-done

}

func createListOfFolders(basePath string) ([]string, error) {
	paths := make([]string, 0, 100)
	pendingPaths := make([]string, 0, 100)
	pendingPaths = append(pendingPaths, basePath)

	for len(pendingPaths) > 0 {
		currentPath := pendingPaths[0]
		paths = append(paths, currentPath)
		pendingPaths = pendingPaths[1:]

		// Read the directories in the path
		f, err := os.Open(currentPath)
		if err != nil {
			return nil, err
		}
		dirEntries, err := f.Readdir(-1)
		for _, entry := range dirEntries {
			if entry.IsDir() {
				entry.Mode()
				newDirectory := filepath.Join(currentPath, entry.Name())
				pendingPaths = append(pendingPaths, newDirectory)
			}
		}
		f.Close()
		if err != nil {
			return nil, err
		}
	}

	return paths, nil
}

