// Copyright 2016 Jacob Taylor jacob@ablox.io
// License: Apache2 - http://www.apache.org/licenses/LICENSE-2.0
package main

import (
	"fmt"
	"github.com/rjeczalik/notify"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"time"
)

func main() {
	fmt.Println("replicat initializing....")
	rand.Seed(int64(time.Now().Nanosecond()))

	SetupCli()
	logOnlyHandler := LogOnlyChangeHandler{}
	tracker := FilesystemTracker{}
	tracker.init(globalSettings.Directory)
	var c ChangeHandler
	c = &logOnlyHandler
	tracker.watchDirectory(&c)

	// Update the path that traffic is served from to be the filesystem canonical path. This will allow the event folders that come in to match what we have.
	symPath, err := filepath.EvalSymlinks(globalSettings.Directory)
	if err != nil {
		var err2, err3 error
		// We have an error. If the directory does not exist, then try to create it. Fail if we cannot creat it with the original error
		if os.IsNotExist(err) {
			err2 = os.Mkdir(globalSettings.Directory, os.ModeDir+os.ModePerm)
			symPath, err3 = filepath.EvalSymlinks(globalSettings.Directory)
			if err2 != nil || err3 != nil {
				panic(fmt.Sprintf("err: %v\nerr2: %v\nerr3: %v\n", err, err2, err3))
			}
		} else {
			panic(err)
		}
	}

	if symPath != globalSettings.Directory {
		fmt.Printf("Updating serving directory to: %s\n", symPath)
		globalSettings.Directory = symPath
	}

	listOfFileInfo, err := scanDirectoryContents()
	if err != nil {
		log.Fatal(err)
	}

	server := serverMap[globalSettings.Name]
	server.CurrentState = listOfFileInfo

	// Make the channel buffered to ensure no event is dropped. Notify will drop
	// an event if the receiver is not able to keep up the sending pace.
	fsEventsChannel := make(chan notify.EventInfo, 1)

	//setup the processing loop for handling filesystem events
	//go filesystemMonitorLoop(fsEventsChannel, &listOfFileInfo)

	// Set up a watch point listening for events within a directory tree rooted at the specified folder
	err = notify.Watch(globalSettings.Directory+"/...", fsEventsChannel, notify.All)
	if err != nil {
		log.Fatal(err)
	}
	defer notify.Stop(fsEventsChannel)

	fmt.Printf("replicat %s online....\n", globalSettings.Name)
	defer fmt.Println("End of line")

	totalFiles := 0
	for _, fileInfoList := range listOfFileInfo {
		totalFiles += len(fileInfoList)
	}

	fmt.Printf("Now keeping an eye on %d folders and %d files located under: %s\n", len(listOfFileInfo), totalFiles, globalSettings.Directory)

	bootstrapAndServe()

	dotCount := 0
	sleepSeconds := time.Duration(25 + rand.Intn(10))
	fmt.Printf("Full Sync time set to: %d seconds\n", sleepSeconds)
	for {
		// Randomize the sync time to decrease oscillation
		time.Sleep(time.Second * sleepSeconds)
		changed, updatedState, newPaths, deletedPaths, matchingPaths := checkForChanges(listOfFileInfo, nil)
		if changed {
			fmt.Println("\nWe have changes, ship it (also updating saved state now that the changes were tracked)")
			fmt.Printf("@Path report: new %d, deleted %d, matching %d, original %d, updated %d\n", len(newPaths), len(deletedPaths), len(matchingPaths), len(listOfFileInfo), len(updatedState))
			fmt.Printf("@New paths: %v\n", newPaths)
			fmt.Printf("@Deleted paths: %v\n", deletedPaths)
			fmt.Println("******************************************************")
			listOfFileInfo = updatedState

			// Post the changes to the other side.
			//sendFolderTree(listOfFileInfo)
		} else {
			fmt.Print(".")
			dotCount++
			if dotCount%100 == 0 {
				fmt.Println("")
			}
		}
	}
}
