// Copyright 2016 Jacob Taylor jacob@ablox.io
// License: Apache2 - http://www.apache.org/licenses/LICENSE-2.0
package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/goji/httpauth"
	"github.com/rjeczalik/notify"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"
	"syscall"
)

type Event struct {
	Source       string
	Name         string
	Message      string
	Time         time.Time
	IsDirectory  bool
}

var events = make([]Event, 0, 100)

func main() {
	fmt.Println("replicat initializing....")
	rand.Seed(int64(time.Now().Nanosecond()))

	SetupCli()

	// Update the path that traffic is served from to be the filsystem canonical path. This will allow the event folders that come in to match what we have.
	symPath, err := filepath.EvalSymlinks(globalSettings.Directory)
	if err != nil {
		var err2, err3 error
		// We have an error. If the directory does not exist, then try to create it. Fail if we cannot creat it with the original error
		if os.IsNotExist(err) {
			err2 = os.Mkdir(globalSettings.Directory, os.ModeDir + os.ModePerm)
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

	listOfFileInfo, err := createListOfFolders()
	if err != nil {
		log.Fatal(err)
	}

	server := serverMap[globalSettings.Name]
	server.CurrentState = listOfFileInfo

	// Make the channel buffered to ensure no event is dropped. Notify will drop
	// an event if the receiver is not able to keep up the sending pace.
	fsEventsChannel := make(chan notify.EventInfo, 1)

	//setup the processing loop for handling filesystem events
	go filesystemMonitorLoop(fsEventsChannel, &listOfFileInfo)

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

	http.Handle("/event/", httpauth.SimpleBasicAuth("replicat", "isthecat")(http.HandlerFunc(eventHandler)))
	http.Handle("/tree/", httpauth.SimpleBasicAuth("replicat", "isthecat")(http.HandlerFunc(folderTreeHandler)))
	http.Handle("/config/", httpauth.SimpleBasicAuth("replicat", "isthecat")(http.HandlerFunc(configHandler)))

	go bootstrapAndServe()

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
			sendFolderTree(listOfFileInfo)
		} else {
			fmt.Print(".")
			dotCount++
			if dotCount%100 == 0 {
				fmt.Println("")
			}
		}
	}
}

// Monitor the filesystem looking for changes to files we are keeping track of.
func filesystemMonitorLoop(c chan notify.EventInfo, listOfFileInfo *DirTreeMap) {
	directoryLength := len(globalSettings.Directory)
	for {
		ei := <-c

		fullPath := string(ei.Path())

		path := fullPath
		if len(fullPath) >= directoryLength && globalSettings.Directory == fullPath[:directoryLength] {
			// update the path to not have this prefix
			path = fullPath[directoryLength+1:]
		}

		event := Event{Name: ei.Event().String(), Message: path}
		log.Printf("Event captured name: %s location: %s", event.Name, event.Message)

		//todo was working on this code. Should have unit tests for the monitor filesystem. It looks like rename has two unrelated events. Is this right? How do we tell it is a rename and not a remove?
		// It looks like there is some sort of reversal of paths now. where adds are being removed from the other side.
		processFilesystemEvent(event, path, fullPath, listOfFileInfo)
	}
}

func processFilesystemEvent(event Event, path, fullPath string, listOfFileInfo *DirTreeMap) {
	// Check to see if this was a path we knew about
	_, isDirectory := (*listOfFileInfo)[path]
	var iNode uint64
	// if we have not found it yet, check to see if it can be stated
	info, err := os.Stat(fullPath)
	if err == nil {
		isDirectory = info.IsDir()
		sysInterface := info.Sys()
		fmt.Printf("sysInterface: %v\n", sysInterface)
		if sysInterface != nil {
			foo := sysInterface.(*syscall.Stat_t)
			iNode = foo.Ino
		}
	}

	fmt.Printf("event raw data: %v with path: %v IsDirectory %v iNode: %d\n", event.Name, path, isDirectory, iNode)

	if isDirectory {
		handleFilesystemEvent(event, path)
	}

}

func handleFilesystemEvent(event Event, pathName string) {
	// todo  Update the global directories

	log.Printf("handleFilsystemEvent name: %s pathName: %s serverMap: %v\n", event.Name, pathName, serverMap)

	serverMapLock.RLock()
	defer serverMapLock.RUnlock()
	currentServer := serverMap[globalSettings.Name]
	currentServer.Lock.Lock()
	defer currentServer.Lock.Unlock()

	currentValue, exists := currentServer.CurrentState[pathName]
	fmt.Printf("Before: %s: Existing value for %s: %v (%v)\n", event.Name, pathName, currentValue, exists)

	switch event.Name {
	case "notify.Create":
		// make sure there is an entry in the DirTreeMap for this folder. Since and empty list will always be returned, we can use that
		currentServer.CurrentState[pathName] = currentServer.CurrentState[pathName]

		updated_value, exists := currentServer.CurrentState[pathName]
		fmt.Printf("notify.Create: Updated  value for %s: %v (%s)\n", pathName, updated_value, exists)

	case "notify.Remove":
		// clean out the entry in the DirTreMap for this folder
		delete(currentServer.CurrentState, pathName)

		updated_value, exists := currentServer.CurrentState[pathName]
		fmt.Printf("notify.Remove: Updated  value for %s: %v (%s)\n", pathName, updated_value, exists)

	// todo fix this to handle the two rename events to be one event
	//case "notify.Rename":
	//	err = os.Remove(pathName)
	//	if err != nil && !os.IsNotExist(err) {
	//		panic(fmt.Sprintf("Error deleting folder that was renamed %s: %v\n", pathName, err))
	//	}
	//	fmt.Printf("notify.Rename: %s\n", pathName)
	default:
	}

	currentValue, exists = currentServer.CurrentState[pathName]
	fmt.Printf("After: %s: Existing value for %s: %v (%v)\n", event.Name, pathName, currentValue, exists)

	// sendEvent to manager
	sendEvent(&event, globalSettings.ManagerAddress, globalSettings.ManagerCredentials)

	// todo - remove this if condition, it is temporary for debugging.
	if globalSettings.Name == "NodeA" {
		log.Println("We are NodeA send to our peers")
		// SendEvent to all peers
		for k, v := range serverMap {
			fmt.Printf("Considering sending to: %s\n", k)
			if k != globalSettings.Name {
				fmt.Printf("sending to peer %s at %s\n", k, v.Address)
				sendEvent(&event, v.Address, globalSettings.ManagerCredentials)
			}
		}
	} else {
		log.Println("We are not NodeA tell nobody what happened")
		for k, v := range serverMap {
			fmt.Printf("Considering: %s\n", k)
			if k != globalSettings.Name {
				fmt.Printf("NOT - sending to peer %s at %s\n", k, v.Address)
				sendEvent(&event, v.Address, globalSettings.ManagerCredentials)
			}
		}
	}
}

/*
Send the folder tree from this node to another node for comparison
*/
func sendFolderTree(initialTree DirTreeMap) {
	var tree = make(DirTreeMap)
	prefixLength := 1 // the slash prefix
	for key, value := range initialTree {
		if key == "" {
			continue
		}
		fmt.Printf("key is: '%s', length is: %d\n", key, prefixLength)
		key = key[prefixLength:]
		tree[key] = value
	}

	// package up the tree
	jsonStr, err := json.Marshal(tree)
	if err != nil {
		panic(err)
	}
	buffer := bytes.NewBuffer(jsonStr)
	client := &http.Client{}

	data := []byte(globalSettings.ManagerCredentials)
	authHash := base64.StdEncoding.EncodeToString(data)

	for k, v := range serverMap {
		// don't send an update to ourselves
		if k == globalSettings.Name {
			continue
		}

		url := "http://" + v.Address + "/tree/"
		fmt.Printf("Posting folder tree to node: %s at URL: %s\n", k, url)

		req, err := http.NewRequest("POST", url, buffer)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Basic "+authHash)

		resp, err := client.Do(req)
		if err != nil {
			fmt.Printf("we encountered an error!\n%s\n", err)
			//panic(err)
			continue
		}

		//fmt.Println("response Status:", resp.Status)
		//fmt.Println("response Headers:", resp.Header)
		_, _ = ioutil.ReadAll(resp.Body)
		//body, _ := ioutil.ReadAll(resp.Body)
		//fmt.Println("response Body:", string(body))
		resp.Body.Close()
	}
}

func folderTreeHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		listOfFileInfo, err := createListOfFolders()
		if err != nil {
			log.Fatal(err)
		}
		json.NewEncoder(w).Encode(listOfFileInfo)
		fmt.Printf("Sending tree of size %d back to client\n", len(listOfFileInfo))
	case "POST":
		//log.Println("Got POST: ", r.Body)
		decoder := json.NewDecoder(r.Body)

		var remoteTreePreTranslate DirTreeMap
		err := decoder.Decode(&remoteTreePreTranslate)
		if err != nil {
			panic("bad json body")
		}

		var remoteTree = make(DirTreeMap)
		for key, value := range remoteTreePreTranslate {
			key = fmt.Sprintf("%s/%s", globalSettings.Directory, key)
			remoteTree[key] = value
		}

		// todo get the node name in here
		fmt.Printf("Received a tree map from: %s\n%s\n", r.RemoteAddr, remoteTree)

		// let's compare to the current one we have.
		_, _, newPaths, deletedPaths, _ := checkForChanges(remoteTree, nil)

		// update the paths to be consistent. Remember, everything is backwards since this is comparing the other side to us instead of two successive states
		// deletion will show up as a new path.

		fmt.Printf("about to check for deletions: %s\n", newPaths)

		// delete folders that were deleted. We delete first, then add to make sure that the old ones will not be in the way
		if len(newPaths) > 0 {
			if globalSettings.Directory == "" {
				panic("globalSettings.Directory is not configured correctly. Aborting")
			}

			fmt.Println("Paths were deleted on the other side. Delete them here")

			fmt.Printf("Paths to delete\nBefore sort:\n%v\n", newPaths)
			// Reverse sort the paths so the most specific is first. This allows us to get away without a recursive delete
			sort.Sort(sort.Reverse(sort.StringSlice(newPaths)))
			fmt.Printf("Paths to delete after sort\nAfter sort:\n%v\n", newPaths)
			for _, relativePath := range newPaths {
				if relativePath == "" || relativePath == "/" {
					fmt.Printf("We had a request to delete the base path. Skipping: %s\n", relativePath)
					continue
				}
				fullPath := globalSettings.Directory + relativePath
				fmt.Printf("Full path is: %s\n", fullPath)

				fmt.Printf("%s: about to remove\n", fullPath)

				// stop on any error except for not exist. We are trying to delete it anyway (or rather, it should have been deleted already)
				err = os.Remove(fullPath)
				if err != nil && !os.IsNotExist(err) {
					panic(err)
				}
				fmt.Printf("%s: done removing (err = %v)\n", fullPath, err)
				err = nil
			}
		}

		// add folders that were created
		fmt.Printf("about to check for new folders: %s\n", deletedPaths)
		if len(deletedPaths) > 0 {
			fmt.Println("Paths were added on the other side. Create them here")
			for _, newPathName := range deletedPaths {
				fmt.Printf("pathname is: %s\n", newPathName)
				err = os.Mkdir(newPathName, os.ModeDir+os.ModePerm)
				//todo figure out how to catch a path exists error.
				if err != nil && !os.IsExist(err) {
					panic(err)
				}
			}
		}

	}
}

func sendEvent(event *Event, address string, credentials string) {
	url := "http://" + address + "/event/"
	fmt.Printf("target url: %s\n", url)

	// Set the event source (server name)
	event.Source = globalSettings.Name

	jsonStr, _ := json.Marshal(event)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonStr))
	req.Header.Set("Content-Type", "application/json")

	data := []byte(credentials)
	authHash := base64.StdEncoding.EncodeToString(data)
	req.Header.Add("Authorization", "Basic "+authHash)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Println(err)
		return
	}

	defer resp.Body.Close()

	fmt.Println("response Status:", resp.Status)
	fmt.Println("response Headers:", resp.Header)
	body, _ := ioutil.ReadAll(resp.Body)
	fmt.Println("response Body:", string(body))
}

func eventHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		json.NewEncoder(w).Encode(events)
	case "POST":
		log.Println("Got POST: ", r.Body)
		decoder := json.NewDecoder(r.Body)
		var event Event
		err := decoder.Decode(&event)
		if err != nil {
			panic("bad json body")
		}

		log.Println(event.Name + ", path: " + event.Message)
		log.Printf("Event info: %v\n", event)

		pathName := globalSettings.Directory + "/" + event.Message

		//todo remember these events and skip the logging on the other side. Possibly use nodeID?

		switch event.Name {
		case "notify.Create":
			err = os.MkdirAll(pathName, os.ModeDir+os.ModePerm)
			if err != nil && !os.IsExist(err) {
				panic(fmt.Sprintf("Error creating folder %s: %v\n", pathName, err))
			}
			fmt.Printf("notify.Create: %s\n", pathName)
		case "notify.Remove":
			err = os.Remove(pathName)
			if err != nil && !os.IsNotExist(err) {
				panic(fmt.Sprintf("Error deleting folder %s: %v\n", pathName, err))
			}
			fmt.Printf("notify.Remove: %s\n", pathName)
		// todo fix this to handle the two rename events to be one event
		//case "notify.Rename":
		//	err = os.Remove(pathName)
		//	if err != nil && !os.IsNotExist(err) {
		//		panic(fmt.Sprintf("Error deleting folder that was renamed %s: %v\n", pathName, err))
		//	}
		//	fmt.Printf("notify.Rename: %s\n", pathName)
		default:
			fmt.Printf("Unknown event found, doing nothing. Event: %v\n", event)
		}

		events = append([]Event{event}, events...)
	}
}
