// Copyright 2016 Jacob Taylor jacob@ablox.io
// License: Apache2 - http://www.apache.org/licenses/LICENSE-2.0
package main

import (
	"bytes"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"sort"
	"sync"
	"time"
)

// Event stores the relevant information on events or updates to the storage layer.
type Event struct {
	Source        string
	Name          string
	Path          string
	Time          time.Time
	IsDirectory   bool
	NetworkSource string

}

var events = make([]Event, 0, 100)

// SendEvent gets events that have happened off to the peer servers so they can replicate the same change
func SendEvent(event Event, fullPath string) {
	// look back through the events for a similar event in the recent path.
	// Set the event source  (server name)
	event.Source = globalSettings.Name
	event.Time = time.Now()

	// Get the current owner of this entry if any
	ownershipLock.RLock()
	originalEntry, exists := ownership[event.Path]
	ownershipLock.RUnlock()

	if exists {
		log.Printf("Original ownership: %#v\n", originalEntry)
		timeDelta := time.Since(originalEntry.Time)
		if timeDelta > OWNERSHIP_EXPIRATION_TIMEOUT {
			log.Printf("Owndership expired. Delta is: %v\n", timeDelta)
		} else {
			// At this point, someone owns this item.
			// If we are not the owner, we are done.
			if originalEntry.Source != globalSettings.Name {
				log.Println("We do not own this. Do not send")
				return
			}
		}
	} else {
		log.Printf("Send event called. No prior ownership: %s\n", fullPath)
	}

	// At this point, it is our change and our event. Store our ownership
	ownershipLock.Lock()
	ownership[event.Path] = event
	ownershipLock.Unlock()

	// sendEvent to manager
	sendEvent(&event, fullPath, globalSettings.ManagerAddress, globalSettings.ManagerCredentials)

	// SendEvent to all peers
	for k, v := range serverMap {
		fmt.Printf("Considering sending to: %s\n", k)
		if k != globalSettings.Name {
			fmt.Printf("sending to peer %s at %s\n", k, v.Address)
			sendEvent(&event, fullPath, v.Address, globalSettings.ManagerCredentials)
		}
	}
}

func sendEvent(event *Event, fullPath string, address string, credentials string) {
	url := "http://" + address + "/event/"
	fmt.Printf("target url: %s\n", url)

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

	if event.Name == "notify.Write" {
		fmt.Printf("source: notify.Write => %s\n", fullPath)
		url := "http://" + address + "/upload/"
		postFile(event.Path, fullPath, url, credentials)
	}
}

var ownership = make(map[string]Event, 100)
var ownershipLock = sync.RWMutex{}

const (
	// OWNERSHIP_EXPIRATION_TIMEOUT - Duration for a replicated change to hold ownership after they make changes. This leaves time for multiple filesystem events to come back without being reported to other nodes
	OWNERSHIP_EXPIRATION_TIMEOUT = 5 * time.Second
)

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

		log.Println(event.Name + ", path: " + event.Path)
		log.Printf("Event info: %#v\n", event)

		// At this point, the other side should have ownership of this path.
		ownershipLock.Lock()
		originalEntry, exists := ownership[event.Path]
		if exists {
			log.Printf("Original ownership: %#v\n", originalEntry)
		} else {
			log.Println("No original ownership found")
		}
		ownership[event.Path] = event
		ownershipLock.Unlock()

		pathName := globalSettings.Nodes[globalSettings.Name].Directory + "/" + event.Path
		relativePath := event.Path

		//todo remember these events and skip the logging on the other side. Possibly use nodeID?

		switch event.Name {
		case "notify.Create":
			fmt.Printf("notify.Create: %s\n", pathName)

			server, exists := serverMap[globalSettings.Name]
			if !exists {
				panic("Unable to find server definition")
			}

			fmt.Println("eventHandler->CreatePath")
			server.storage.CreatePath(relativePath, event.IsDirectory)
			fmt.Println("eventHandler->/CreatePath")
		case "notify.Remove":
			err = os.Remove(pathName)
			if err != nil && !os.IsNotExist(err) {
				panic(fmt.Sprintf("Error deleting folder %s: %v\n", pathName, err))
			}
			fmt.Printf("notify.Remove: %s\n", pathName)
		// todo fix this to handle the two rename events to be one event
		case "notify.Rename":
			fmt.Printf("notify.Rename: %s\n", pathName)

			server, exists := serverMap[globalSettings.Name]
			if !exists {
				panic("Unable to find server definition")
			}

			fmt.Println("eventHandler->CreatePath")
			server.storage.CreatePath(relativePath, event.IsDirectory)
			fmt.Println("eventHandler->/CreatePath")

		default:
			fmt.Printf("Unknown event found, doing nothing. Event: %v\n", event)
		}

		events = append([]Event{event}, events...)
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

func getListOfFolders(w http.ResponseWriter) {
	listOfFileInfo, err := scanDirectoryContents()
	if err != nil {
		log.Fatal(err)
	}
	json.NewEncoder(w).Encode(listOfFileInfo)
	fmt.Printf("Sending tree of size %d back to client\n", len(listOfFileInfo))
}

func deletePaths(deletedPaths []string) {
	if globalSettings.Nodes[globalSettings.Name].Directory == "" {
		panic("globalSettings.Directory is not configured correctly. Aborting")
	}

	fmt.Println("Paths were deleted on the other side. Delete them here")

	fmt.Printf("Paths to delete\nBefore sort:\n%v\n", deletedPaths)
	// Reverse sort the paths so the most specific is first. This allows us to get away without a recursive delete
	sort.Sort(sort.Reverse(sort.StringSlice(deletedPaths)))
	fmt.Printf("Paths to delete after sort\nAfter sort:\n%v\n", deletedPaths)
	for _, relativePath := range deletedPaths {
		if relativePath == "" || relativePath == "/" {
			fmt.Printf("We had a request to delete the base path. Skipping: %s\n", relativePath)
			continue
		}
		fullPath := globalSettings.Nodes[globalSettings.Name].Directory + relativePath
		fmt.Printf("Full path is: %s\n", fullPath)

		fmt.Printf("%s: about to remove\n", fullPath)

		// stop on any error except for not exist. We are trying to delete it anyway (or rather, it should have been deleted already)
		err := os.Remove(fullPath)
		if err != nil && !os.IsNotExist(err) {
			panic(err)
		}
		fmt.Printf("%s: done removing (err = %v)\n", fullPath, err)
	}
}

func addPaths(newPaths []string) {
	fmt.Println("Paths were added on the other side. Create them here")
	for _, newPathName := range newPaths {
		fmt.Printf("pathname is: %s\n", newPathName)
		err := os.Mkdir(newPathName, os.ModeDir+os.ModePerm)
		if err != nil && !os.IsExist(err) {
			panic(err)
		}
	}
}

func folderTreeHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		getListOfFolders(w)
	case "POST":
		decoder := json.NewDecoder(r.Body)

		var remoteTreePreTranslate DirTreeMap
		err := decoder.Decode(&remoteTreePreTranslate)
		if err != nil {
			panic("bad json body")
		}

		var remoteTree = make(DirTreeMap)
		for key, value := range remoteTreePreTranslate {
			key = fmt.Sprintf("%s/%s", globalSettings.Nodes[globalSettings.Name].Directory, key)
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
			deletePaths(newPaths)
		}

		// add folders that were created
		fmt.Printf("about to check for new folders: %s\n", deletedPaths)
		// add folders that were added.
		if len(deletedPaths) > 0 {
			addPaths(deletedPaths)
		}
	}
}

func postFile(filename string, fullPath string, targetUrl string, credentials string) error {
	body := &bytes.Buffer{}
	bodyWriter := multipart.NewWriter(body)
	fileWriter, err := bodyWriter.CreateFormFile("uploadfile", filename)
	if err != nil {
		fmt.Println("error writing to buffer")
		return err
	}

	file, err := os.Open(fullPath)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.Copy(fileWriter, file)
	if err != nil {
		fmt.Println("error copying file")
		return err
	}

	myHash, err := fileMd5Hash(fullPath)
	if err != nil {
		fmt.Printf("failed to calculate MD5 Hash for %s\n", fullPath)
		return err
	}

	bodyWriter.WriteField("HASH", myHash)
	contentType := bodyWriter.FormDataContentType()
	err = bodyWriter.Close()
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", targetUrl, body)
	req.Header.Set("Content-Type", contentType)

	data := []byte(credentials)
	authHash := base64.StdEncoding.EncodeToString(data)
	req.Header.Add("Authorization", "Basic "+authHash)

	client := &http.Client{}
	resp, err := client.Do(req)
	defer resp.Body.Close()

	if err != nil {
		return err
	}

	return nil
}

func fileMd5Hash(filePath string) (string, error) {
	var returnMD5String string
	file, err := os.Open(filePath)
	if err != nil {
		return returnMD5String, err
	}
	defer file.Close()

	hash := md5.New()
	_, err = io.Copy(hash, file)
	if err != nil {
		return returnMD5String, err
	}

	hashInBytes := hash.Sum(nil)[:16]

	returnMD5String = hex.EncodeToString(hashInBytes)

	return returnMD5String, nil

}
