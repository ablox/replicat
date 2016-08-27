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
	"sort"
	"strings"
	"time"
	"bytes"
	"io/ioutil"
	"net/http"
	"encoding/base64"
	"encoding/json"
)

type Event struct {
	Name    string
	Message string
}

// settings for the server
type Settings struct {
	Directory string

}

type DirTreeMap map[string][]string
//type DirTreeMap map[string][]os.FileInfo

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
			EnvVar: "directory, d",
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

	go func(c chan notify.EventInfo) {
		for {
			ei := <-c
			sendEvent(&Event{Name: ei.Event().String(), Message: ei.Path()})
			log.Println("Got event:" + ei.Event().String() + ", with Path:" + ei.Path())
		}
	}(fsEventsChannel)

	for {
		time.Sleep(time.Second * 5)
		checkForChanges(globalSettings.Directory, listOfFileInfo)
		fmt.Println("******************************************************")
	}




}

func checkForChanges(basePath string, originalState DirTreeMap) {
	//  (map[string][]string, error) {
	updatedState, err := createListOfFolders(basePath)
	if err != nil {
		panic(err)
	}

	// Get a list of paths and compare them
	originalPaths := make([]string, 0, len(originalState))
	updatedPaths := make([]string, 0, len(updatedState))

	for key := range originalState {
		originalPaths = append(originalPaths, key)
	}
	sort.Strings(originalPaths)

	for key := range updatedState {
		updatedPaths = append(updatedPaths, key)
	}
	sort.Strings(updatedPaths)

	// We now have two sworted lists of strings. Go through the original ones and compare the files
	var originalPosition, updatedPosition int

	deletedPaths := make([]string, 0, 100)
	newPaths := make([]string, 0, 100)
	matchingPaths := make([]string, 0, len(originalPaths))

	for {
		if originalPosition >= len(originalPaths) {
			// all remaining updated paths are new
			newPaths = append(newPaths, updatedPaths[updatedPosition:]...)
			break
		} else if updatedPosition >= len(updatedPaths) {
			// all remaining original paths are new
			deletedPaths = append(deletedPaths, originalPaths[originalPosition:]...)
			break
		} else {
			result := strings.Compare(originalPaths[originalPosition], updatedPaths[updatedPosition])
			if result == -1 {
				deletedPaths = append(deletedPaths, originalPaths[originalPosition:]...)
				originalPosition++
			} else if result == 1 {
				newPaths = append(newPaths, updatedPaths[updatedPosition])
				updatedPosition++
			} else {
				matchingPaths = append(matchingPaths, updatedPaths[updatedPosition])
				updatedPosition++
				originalPosition++
			}
		}
	}

	fmt.Printf("Path report: new %d, deleted %d, matching %d, original %d, updated %d\n", len(newPaths), len(deletedPaths), len(matchingPaths), len(originalPaths), len(updatedPaths))
	fmt.Printf("New paths: %v\n", newPaths)
	fmt.Printf("Deleted paths: %v\n", deletedPaths)
}


func createListOfFolders(basePath string) (DirTreeMap, error) {
	paths := make([]string, 0, 100)
	pendingPaths := make([]string, 0, 100)
	pendingPaths = append(pendingPaths, basePath)
	listOfFileInfo := make(DirTreeMap)

	for len(pendingPaths) > 0 {
		currentPath := pendingPaths[0]
		paths = append(paths, currentPath)
		fileList := make([]string, 0, 100)
		//fileList := make([]os.FileInfo, 0, 100)
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
				//newDirectory := filepath.Join(currentPath, entry)
				pendingPaths = append(pendingPaths, newDirectory)
			} else {
				fileList = append(fileList, entry.Name())
			}
		}
		f.Close()
		if err != nil {
			return nil, err
		}

		sort.Strings(fileList)
		listOfFileInfo[currentPath] = fileList
	}

	return listOfFileInfo, nil
}

func sendEvent(event *Event) {
	url := "http://localhost:8080/event/"

	jsonStr, _ := json.Marshal(event)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonStr))
	req.Header.Set("Content-Type", "application/json")

	data := []byte("replicat:isthecat")
	authHash := base64.StdEncoding.EncodeToString(data)
	req.Header.Add("Authorization", "Basic " + authHash)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	fmt.Println("response Status:", resp.Status)
	fmt.Println("response Headers:", resp.Header)
	body, _ := ioutil.ReadAll(resp.Body)
	fmt.Println("response Body:", string(body))
}