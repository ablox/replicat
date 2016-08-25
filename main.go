// Copyright 2016 Jacob Taylor jacob@ablox.io
// License: Apache2 - http://www.apache.org/licenses/LICENSE-2.0
package main

import (
	"fmt"
	"github.com/rjeczalik/notify"
	"github.com/urfave/cli"
	"log"
	"os"
	"path/filepath"
)

// settings for the server
type Settings struct {
	Directory string

}

var globalSettings Settings = Settings{
	Directory: "",
}

func main() {
	fmt.Println("replicat initializing....")

	app := cli.NewApp()
	app.Name = "Replicat"
	app.Usage = "rsync for the cloud"
	app.Action = func(c *cli.Context) error {
		globalSettings.Directory = c.GlobalString("directory")

		if globalSettings.Directory == "" {
			panic("directory is required to serve files\n")
		}

		return nil
	}

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:   "directory, d",
			Value:  globalSettings.Directory,
			Usage:  "Specify a directory where the files to share are located.",
			EnvVar: "DIRECTORY, d",
		},
	}

	app.Run(os.Args)

	listOfFileInfo, err := createListOfFolders(globalSettings.Directory)
	if err != nil {
		log.Fatal(err)
	}

	// Make the channel buffered to ensure no event is dropped. Notify will drop
	// an event if the receiver is not able to keep up the sending pace.
	fsEventsChannel := make(chan notify.EventInfo, 1)

	// Set up a watchpoint listening for events within a directory tree rooted at the specified folder
	if err := notify.Watch(globalSettings.Directory + "/...", fsEventsChannel, notify.All); err != nil {
		log.Fatal(err)
	}
	defer notify.Stop(fsEventsChannel)

	fmt.Println("replicat online....")
	defer fmt.Println("End of line")

	fmt.Printf("Now listening on: %d folders under: %s\n", len(listOfFileInfo), globalSettings.Directory)

	totalFiles := 0
	for _, fileInfoList := range listOfFileInfo {
		totalFiles += len(fileInfoList)
	}

	fmt.Printf("Tracking %d folders with %d files\n", len(listOfFileInfo), totalFiles)

	func(c chan notify.EventInfo) {
		for {
			ei := <-c
			log.Println("Got event:", ei)
		}
	}(fsEventsChannel)
}

func createListOfFolders(basePath string) (map[string][]os.FileInfo, error) {
	paths := make([]string, 0, 100)
	pendingPaths := make([]string, 0, 100)
	pendingPaths = append(pendingPaths, basePath)
	listOfFileInfo := make(map[string][]os.FileInfo)

	for len(pendingPaths) > 0 {
		currentPath := pendingPaths[0]
		paths = append(paths, currentPath)
		fileList := make([]os.FileInfo, 0, 100)
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
			} else {
				fileList = append(fileList, entry)
			}
		}
		f.Close()
		if err != nil {
			return nil, err
		}

		listOfFileInfo[currentPath] = fileList
	}

	return listOfFileInfo, nil
}
