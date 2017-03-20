// Package replicat is a server for n way synchronization of content (rsync for the cloud).
// More information at: http://replic.at
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
	"golang.org/x/crypto/blake2b"
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

// Settings - for this replicat server. This should include everything needed for this server to run and connect with
// its manager and cluster. It should not include anything else.
type Settings struct {
	Name               string
	ManagerAddress     string
	ManagerCredentials string
	ClusterKey         string
	Directory          string
	Address            string
}

var globalSettings Settings

// Event stores the relevant information on events or updates to the storage layer.
type Event struct {
	Source        string
	Name          string
	Path          string
	SourcePath    string
	Time          time.Time
	ModTime       time.Time
	IsDirectory   bool
	NetworkSource string
	RawData       []byte
}

var events = make([]Event, 0, 100)

// GetGlobalSettings -- retrieve the settings for the replicat server
func GetGlobalSettings() Settings {
	return globalSettings
}

// SetGlobalSettings -- set the settings for the replicat server
func SetGlobalSettings(newSettings Settings) {
	globalSettings = newSettings
}

func sendCatalogToManagerAndSiblings(event Event) {
	fmt.Println("sendCatalogToManagerAndSiblings")
	sendEventToManagerAndSiblings(event, "")
	fmt.Println("/sendCatalogToManagerAndSiblings")
}

// SendEvent gets events that have happened off to the peer servers so they can replicate the same change
func SendEvent(event Event, fullPath string) {
	// look back through the events for a similar event in the recent path.
	// Set the event source  (server name)
	event.Source = globalSettings.Name
	event.Time = time.Now()

	// Get the current owner of this entry if any
	path := event.Path
	if path == "" {
		path = event.SourcePath
	}
	ownershipLock.RLock()
	originalEntry, exists := ownership[path]
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

	sendEventToManagerAndSiblings(event, fullPath)
}

func sendEventToManagerAndSiblings(event Event, fullPath string) {
	// sendEvent to manager
	go sendEvent(&event, fullPath, globalSettings.ManagerAddress, globalSettings.ManagerCredentials)

	// SendEvent to all peers
	for k, v := range serverMap {
		if k != globalSettings.Name {
			fmt.Printf("sending to peer %s at %s\n", k, v.Address)
			sendEvent(&event, fullPath, v.Address, globalSettings.ManagerCredentials)
		}
	}
}

func sendFileRequestToServer(serverName string, event Event) {
	go sendEvent(&event, "", globalSettings.ManagerAddress, globalSettings.ManagerCredentials)

	server := serverMap[serverName]
	if server == nil {
		//panic("Server no longer exists when trying to send a file request\n")
		fmt.Printf("Server cannot be reached, skipping sending file: (%s) %s\n", serverName, event.Path)
		return
	}

	go sendEvent(&event, "", server.Address, globalSettings.ManagerCredentials)
}

func sendEvent(event *Event, fullPath string, address string, credentials string) {
	if address == "" {
		fmt.Println("No address for manager, returning")
		return
	}

	url := "http://" + address + "/event/"
	fmt.Printf("target url: %s\nEvent is: %v\n", url, event)

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

	//fmt.Println("response Status:", resp.Status)
	//fmt.Println("response Headers:", resp.Header)
	_, _ = ioutil.ReadAll(resp.Body)
	//fmt.Println("response Body:", string(body))

	switch event.Name {
	case "replicat.Rename", "notify.Create", "notify.Write":
		fmt.Printf("sendEvent - %s, full event: %#v\n", event.Name, event)

		if event.SourcePath == "" {
			fmt.Printf("sendEvent We have a rename in. destination: %s\n", event.Path)
			postHelper(event.Path, fullPath, address, credentials)
		}
	}

}

func postHelper(path, fullPath, address, credentials string) {
	fmt.Printf("Sending file to: %s\npath: %s\n", address, path)
	url := "http://" + address + "/upload/"
	postFile(path, fullPath, url, credentials)
}

var ownership = make(map[string]Event, 100)
var ownershipLock = sync.RWMutex{}

const (
	// OWNERSHIP_EXPIRATION_TIMEOUT - Duration for a replicated change to hold ownership after they make changes. This leaves time for multiple filesystem events to come back without being reported to other nodes
	OWNERSHIP_EXPIRATION_TIMEOUT = 20 * time.Second
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

		path := event.Path
		if path == "" {
			path = event.SourcePath
		}

		ownershipLock.Lock()
		originalEntry, exists := ownership[path]
		if exists {
			log.Printf("Original ownership: %#v\n", originalEntry)
		} else {
			log.Println("No original ownership found")
		}
		ownership[path] = event
		ownershipLock.Unlock()

		pathName := globalSettings.Directory + "/" + event.Path
		relativePath := event.Path

		server, exists := serverMap[globalSettings.Name]
		if !exists {
			panic("Unable to find server definition")
		}

		switch event.Name {
		case "notify.Create":
			fmt.Printf("notify.Create: %s\n", pathName)
			fmt.Println("eventHandler->CreatePath")
			server.storage.CreatePath(relativePath, event.IsDirectory)
			fmt.Println("eventHandler->/CreatePath")
		case "notify.Remove":
			fmt.Printf("notify.Remove: %s\n", pathName)
			err = os.Remove(pathName)
			if err != nil && !os.IsNotExist(err) {
				panic(fmt.Sprintf("Error deleting folder %s: %v\n", pathName, err))
			}
		case "notify.Rename":
			fmt.Printf("notify.Rename: %s\n", pathName)
			fmt.Println("eventHandler->CreatePath")
			server.storage.CreatePath(relativePath, event.IsDirectory)
			fmt.Println("eventHandler->/CreatePath")
		case "replicat.Rename":
			fmt.Println("eventHandler->Rename")
			server.storage.Rename(event.SourcePath, event.Path, event.IsDirectory)
			fmt.Println("eventHandler->/Rename")
		case "replicat.Catalog":
			fmt.Printf("eventHandler->Catalog\n%#v\n", event)
			server.storage.ProcessCatalog(event)
			fmt.Println("eventHandler->/Catalog")
		case "replicat.FileRequest":
			fmt.Printf("Received request to send files from: %s\n", event.Source)
			fileMap := make(map[string]EntryJSON)
			json.Unmarshal(event.RawData, &fileMap)
			go server.storage.sendRequestedPaths(fileMap, event.Source)
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
	if globalSettings.Directory == "" {
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
		fullPath := globalSettings.Directory + relativePath
		fmt.Printf("Full path is: %s\n", fullPath)

		fmt.Printf("%s: about to remove\n", fullPath)

		// stop on any error except for not exist. We are trying to delete it anyway (or rather, it should have been deleted already)
		err := os.Remove(fullPath)
		if err != nil && !os.IsNotExist(err) {
			panic(err)
		}
		serverMap[globalSettings.Name].storage.IncrementStatistic(TRACKER_FILES_DELETED, 1)

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
		serverMap[globalSettings.Name].storage.IncrementStatistic(TRACKER_TOTAL_FOLDERS, 1)
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

func postFile(filename string, fullPath string, address string, credentials string) error {
	fmt.Printf("postFile: filename: %s, fullPath: %s\n", filename, fullPath)
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

	// Copy the file to the request
	_, err = io.CopyBuffer(fileWriter, file, nil)
	if err != nil {
		fmt.Printf("error copying file: %s (%s)\n", filename, err)
		//panic("again?")
		return err
	}

	// File data
	server := serverMap[globalSettings.Name]
	entryJSON, err := server.storage.getEntryJSON(filename)
	entryString, err := json.Marshal(&entryJSON)
	bodyWriter.WriteField("EntryJSON", string(entryString))

	myHash, err := fileMd5Hash(fullPath)
	if err != nil {
		fmt.Printf("failed to calculate MD5 Hash for %s\n", fullPath)
		return err
	}

	// Get the mod time and blake2 and filesize from the contents

	bodyWriter.WriteField("HASH", myHash)
	contentType := bodyWriter.FormDataContentType()
	err = bodyWriter.Close()
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", address, body)
	req.Header.Set("Content-Type", contentType)

	data := []byte(credentials)
	authHash := base64.StdEncoding.EncodeToString(data)
	req.Header.Add("Authorization", "Basic "+authHash)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Error sending a file (%s) to another node(%s) error(%s)\n", filename, address, err)
		return err
	}

	resp.Body.Close()

	serverMap[globalSettings.Name].storage.IncrementStatistic(TRACKER_FILES_SENT, 1)

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

func fileBlake2bHash(filePath string) ([]byte, error) {
	var hashResult []byte
	file, err := os.Open(filePath)
	if err != nil {
		return hashResult, err
	}
	defer file.Close()

	hash, _ := blake2b.New256(nil)
	_, err = io.Copy(hash, file)
	if err != nil {
		return hashResult, err
	}

	hashResult = hash.Sum(nil)

	fmt.Printf("%v - %s\n", hex.EncodeToString(hashResult), filePath)

	return hashResult, nil
}
