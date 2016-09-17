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
	"net"
	"strconv"
)

type ReplicatServer struct {
	Name         string
	Address      string
	PublicKey    string
	LastReceived DirTreeMap
	LastSent     DirTreeMap
}

type Node struct {
	Cluster string
	Name    string
	Addr    string
}

var nodes = make(map[string]Node)


type ServerMap map[string]ReplicatServer

var GlobalServerMap = ServerMap{
	"NodeA": ReplicatServer{
		Address: ":8001",
	},
	"NodeB": ReplicatServer{
		Address: ":8002",
	},
}

type Event struct {
	Source       string
	Name         string
	OriginalName string
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
		panic(err)
	}

	if symPath != globalSettings.Directory {
		fmt.Printf("Updating serving directory to: %s\n", symPath)
		globalSettings.Directory = symPath
	}

	listOfFileInfo, err := createListOfFolders()
	if err != nil {
		log.Fatal(err)
	}

	// Make the channel buffered to ensure no event is dropped. Notify will drop
	// an event if the receiver is not able to keep up the sending pace.
	fsEventsChannel := make(chan notify.EventInfo, 1)

	// Set up a watch point listening for events within a directory tree rooted at the specified folder
	err = notify.Watch(globalSettings.Directory+"/...", fsEventsChannel, notify.All)
	if err != nil {
		log.Fatal(err)
	}
	defer notify.Stop(fsEventsChannel)

	fmt.Printf("replicat %s online....\n", globalSettings.Name)
	defer fmt.Println("End of line")

	//globalSettings.Address = GlobalServerMap[globalSettings.Name].Address

	totalFiles := 0
	for _, fileInfoList := range listOfFileInfo {
		totalFiles += len(fileInfoList)
	}

	fmt.Printf("Now keeping an eye on %d folders and %d files located under: %s\n", len(listOfFileInfo), totalFiles, globalSettings.Directory)
	go func(c chan notify.EventInfo) {
		for {
			ei := <-c

			// todo add ignore file
			// sendEvent to manager (if it's available)
			fullPath := string(ei.Path())
			directoryLength := len(globalSettings.Directory)

			path := fullPath
			if len(fullPath) >= directoryLength && globalSettings.Directory == fullPath[:directoryLength] {
				// update the path to not have this prefix
				path = fullPath[directoryLength+1:]
			}

			//fmt.Printf("Call to isdir resulted in %v\n", ei.IsDir())

			//// Check if it is a directory based on our directory tree first. Then check the file system
			var isDirectory bool
			//_, exists := listOfFileInfo[fullPath]
			//if exists == true {
			//	isDirectory = true
			//} else {
			//}
			// Get the file info. Useful if we need to restrict our actions to directories
			info, err := os.Stat(fullPath)
			if err == nil {
				isDirectory = info.IsDir()
			} else {
				_, exists := listOfFileInfo[fullPath]
				if exists == true {
					isDirectory = true
				}
			}

			event := Event{Name: ei.Event().String(), Message: path, IsDirectory: isDirectory}
			fmt.Printf("event raw data: %v with path: %v\n", ei.Event(), path)

			// todo copy the source folder on a rename to the event
			sendEvent(&event, globalSettings.ManagerAddress, globalSettings.ManagerCredentials)

			// sendEvent to peers (if any)
			//for name, server := range GlobalServerMap {
			//	if name != globalSettings.Name {
			//		fmt.Printf("sending to peer %s\n", name)
			//		sendEvent(&event, server.Address, globalSettings.ManagerCredentials)
			//	}
			//}
			for k, v := range nodes {
				if k != globalSettings.Name {
					fmt.Printf("sending to peer %s at %s\n", k, v.Addr)
					sendEvent(&event, v.Addr, globalSettings.ManagerCredentials)
				}
			}

			log.Println("Got event:" + ei.Event().String() + ", with Path:" + ei.Path())
		}
	}(fsEventsChannel)

	http.Handle("/event/", httpauth.SimpleBasicAuth("replicat", "isthecat")(http.HandlerFunc(eventHandler)))
	http.Handle("/tree/", httpauth.SimpleBasicAuth("replicat", "isthecat")(http.HandlerFunc(folderTreeHandler)))
	http.Handle("/config/", httpauth.SimpleBasicAuth("replicat", "isthecat")(http.HandlerFunc(configHandler)))

	//fmt.Printf("listening on: %s\n", globalSettings.Address)

	go func() {
		lsnr, err := net.Listen("tcp4", ":0")
		if err != nil {
			fmt.Println("Error listening:", err)
			os.Exit(1)
		}
		fmt.Println("Listening on:", lsnr.Addr().String())

		go sendConfigToServer(lsnr.Addr())

		//defer resp.Body.Close()
		//if err != nil {
		//	log.Println(err)
		//} else {
		//
		//	fmt.Println("response Status:", resp.Status)
		//	fmt.Println("response Headers:", resp.Header)
		//	body, _ := ioutil.ReadAll(resp.Body)
		//	fmt.Println("response Body:", string(body))
		//}
		//
		err = http.Serve(lsnr, nil)
		if err != nil {
			panic(err)
		}

	}()

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

func sendConfigToServer(addr net.Addr) {
	url := "http://" + globalSettings.BootstrapAddress + "/config/"
	fmt.Printf("Manager location: %s\n", url)

	jsonStr, _ := json.Marshal(Node{Name: globalSettings.Name, Addr: "127.0.0.1:" + strconv.Itoa(addr.(*net.TCPAddr).Port)})
	fmt.Printf("jsonStr: %s\n", jsonStr)

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonStr))
	req.Header.Set("Content-Type", "application/json")

	data := []byte(globalSettings.ManagerCredentials)
	authHash := base64.StdEncoding.EncodeToString(data)
	req.Header.Add("Authorization", "Basic "+authHash)

	client := &http.Client{}
	_, err = client.Do(req)
	if err != nil {
		panic(err)
	}
}

func configHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("configHandler")

	switch r.Method {
	case "POST" :
		decoder := json.NewDecoder(r.Body)
		err := decoder.Decode(&nodes)
		fmt.Printf("nodes: %s", nodes)

		if err != nil {
			fmt.Println(err)
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
		if key == globalSettings.Directory {
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

	for k, v := range nodes {
		if k != globalSettings.Name {
			fmt.Printf("skipping %s\n", k)
			continue
		}
		fmt.Printf("sending tree update to: %s\n", k)

		url := "http://" + v.Addr + "/tree/"
		fmt.Printf("Posting folder tree to: %s\n", url)

		req, err := http.NewRequest("POST", url, buffer)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Basic "+authHash)

		resp, err := client.Do(req)
		if err != nil {
			fmt.Printf("we encountered an error!\n%s\n", err)
			//panic(err)
			continue
		}

		fmt.Println("response Status:", resp.Status)
		fmt.Println("response Headers:", resp.Header)
		body, _ := ioutil.ReadAll(resp.Body)
		fmt.Println("response Body:", string(body))
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
			//fmt.Printf("key is: '%s'\n", key)
			key = fmt.Sprintf("%s/%s", globalSettings.Directory, key)
			remoteTree[key] = value
		}

		fmt.Printf("Received a tree map from: %s\n%s\n", r.RemoteAddr, remoteTree)

		// let's compare to the current one we have.
		_, _, newPaths, deletedPaths, _ := checkForChanges(remoteTree, nil)

		// update the paths to be consistent. Remember, everything is backwards since this is comparing the other side to us instead of two successive states
		// deletion will show up as a new path.

		fmt.Printf("about to check for deletions: %s\n", newPaths)

		// delete folders that were deleted. We delete first, then add to make sure that the old ones will not be in the way
		if len(newPaths) > 0 {
			fmt.Println("Paths were deleted on the other side. Delete them here")
			// Reverse sort the paths so the most specific is first. This allows us to get away without a recursive delete
			sort.Sort(sort.Reverse(sort.StringSlice(newPaths)))
			for _, pathName := range newPaths {
				if pathName == "" || globalSettings.Directory == "" {
					fmt.Sprintf("Trying to delete invalid path: %s\n", pathName)
					panic("Path information is not right. Do not delete")
				} else if pathName == globalSettings.Directory {
					fmt.Printf("We had a request to delete the base path. Skipping: %s\n", pathName)
					continue
				}

				err = os.Remove(pathName)
				if err != nil {
					panic(err)
				}
			}
		}

		// add folders that were created
		fmt.Printf("about to check for new folders: %s\n", deletedPaths)
		if len(deletedPaths) > 0 {
			fmt.Println("Paths were added on the other side. Create them here")
			for _, newPathName := range deletedPaths {
				fmt.Printf("pathname is: %s\n", newPathName)
				//if newPathName == "" {
				//	continue
				//}
				//newFullPath := fmt.Sprintf("%s/%s", globalSettings.Directory, newPathName)
				//fmt.Printf("new full path is: %s\n", newFullPath)
				err = os.Mkdir(newPathName, os.ModeDir+os.ModePerm)
				//todo figure out how to catch a path exists error.
				//if err != nil && err != os.ErrExist {
				//	panic(err)
				//}
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
	} else {
		defer resp.Body.Close()

		fmt.Println("response Status:", resp.Status)
		fmt.Println("response Headers:", resp.Header)
		body, _ := ioutil.ReadAll(resp.Body)
		fmt.Println("response Body:", string(body))
	}
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

		//// Only do stuff for directories at the moment.
		//if event.IsDirectory == false {
		//	fmt.Println("The event is not for a directory, skipping")
		//	return
		//}

		pathName := globalSettings.Directory + "/" + event.Message

		switch event.Name {
		case "notify.Create":
			os.Mkdir(pathName, os.ModeDir+os.ModePerm)
			// todo figure out how to catch a path exists error
			fmt.Printf("create event found. Should be creating: %s\n", pathName)
		case "notify.Remove":
			os.Remove(pathName)
			fmt.Printf("remove attempted on: %s\n", pathName)
		case "notify.Rename":
			os.Remove(pathName)
			fmt.Printf("rename attempted on: %s\n", pathName)
		default:
			fmt.Printf("Unknown event found, doing nothing. Event: %s\n", event.Name)
		}

		events = append([]Event{event}, events...)
	}
}
