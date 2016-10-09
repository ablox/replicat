// Copyright 2016 Jacob Taylor jacob@ablox.io
// License: Apache2 - http://www.apache.org/licenses/LICENSE-2.0
package main

import (
	"fmt"
	"math/rand"
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

	fmt.Printf("replicat %s online....\n", globalSettings.Name)
	defer fmt.Println("End of line")

	bootstrapAndServe()


	for {
		time.Sleep(time.Millisecond * 500)
	}


	//listOfFileInfo, err := scanDirectoryContents()
	//if err != nil {
	//	   log.Fatal(err)
	//}
	//
	//dotCount := 0
	//sleepSeconds := time.Duration(25 + rand.Intn(10))
	//fmt.Printf("Full Sync time set to: %d seconds\n", sleepSeconds)
	//for {
	//	// Randomize the sync time to decrease oscillation
	//	time.Sleep(time.Second * sleepSeconds)
	//	changed, updatedState, newPaths, deletedPaths, matchingPaths := checkForChanges(listOfFileInfo, nil)
	//	if changed {
	//		fmt.Println("\nWe have changes, ship it (also updating saved state now that the changes were tracked)")
	//		fmt.Printf("@Path report: new %d, deleted %d, matching %d, original %d, updated %d\n", len(newPaths), len(deletedPaths), len(matchingPaths), len(listOfFileInfo), len(updatedState))
	//		fmt.Printf("@New paths: %v\n", newPaths)
	//		fmt.Printf("@Deleted paths: %v\n", deletedPaths)
	//		fmt.Println("******************************************************")
	//		listOfFileInfo = updatedState
	//
	//		// Post the changes to the other side.
	//		//sendFolderTree(listOfFileInfo)
	//	} else {
	//		fmt.Print(".")
	//		dotCount++
	//		if dotCount%100 == 0 {
	//			fmt.Println("")
	//		}
	//	}
	//}
}
