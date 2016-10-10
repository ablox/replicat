// Copyright 2016 Jacob Taylor jacob@ablox.io
// License: Apache2 - http://www.apache.org/licenses/LICENSE-2.0
package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"sort"
	"time"
	"mime/multipart"
	"io"
)

// Event stores the relevant information on events or updates to the storage layer.
type Event struct {
	Source      string
	Name        string
	Message     string
	Time        time.Time
	IsDirectory bool
}

var events = make([]Event, 0, 100)

// FileEvent stores the nodeid, string, and time of file related events
type FileEvent struct {
	NodeID int32
	Name   string
	Time   time.Time
}

// SendEvent gets events that have happened off to the peer servers so they can replicate the same change
func SendEvent(event Event) {
	// sendEvent to manager
	sendEvent(&event, globalSettings.ManagerAddress, globalSettings.ManagerCredentials)

	log.Println("We are NodeA send to our peers")
	// SendEvent to all peers
	for k, v := range serverMap {
		fmt.Printf("Considering sending to: %s\n", k)
		if k != globalSettings.Name {
			fmt.Printf("sending to peer %s at %s\n", k, v.Address)
			sendEvent(&event, v.Address, globalSettings.ManagerCredentials)
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
		relativePath := event.Message

		//todo remember these events and skip the logging on the other side. Possibly use nodeID?

		switch event.Name {
		case "notify.Create":
			fmt.Printf("notify.Create: %s\n", pathName)

			server, exists := serverMap[globalSettings.Name]
			if !exists {
				panic("Unable to find server definition")
			}

			fmt.Printf("server is %v\n", server)
			fmt.Printf("server.storage is %v\n", server.storage)

			server.storage.CreatePath(relativePath, event.IsDirectory)

			//err = os.MkdirAll(pathName, os.ModeDir+os.ModePerm)
			//if err != nil && !os.IsExist(err) {
			//	panic(fmt.Sprintf("Error creating folder %s: %v\n", pathName, err))
			//}
		case "notify.Remove":
			err = os.Remove(pathName)
			if err != nil && !os.IsNotExist(err) {
				panic(fmt.Sprintf("Error deleting folder %s: %v\n", pathName, err))
			}
			fmt.Printf("notify.Remove: %s\n", pathName)
		// todo fix this to handle the two rename events to be one event
		case "notify.Rename":
			err = os.RemoveAll(pathName)
			if err != nil && !os.IsNotExist(err) {
				panic(fmt.Sprintf("Error deleting folder that was renamed %s: %v\n", pathName, err))
			}
			fmt.Printf("notify.Rename: %s\n", pathName)
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

func folderTreeHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		listOfFileInfo, err := scanDirectoryContents()
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

func postFile(filename string, targetUrl string) error {
	bodyBuf := &bytes.Buffer{}
	bodyWriter := multipart.NewWriter(bodyBuf)

	fileWriter, err := bodyWriter.CreateFormFile("uploadfile", filename)
	if err != nil {
		fmt.Println("error writing to buffer")
		return err
	}

	fh, err := os.Open(filename)
	if err != nil {
		fmt.Println("error opening file")
		return err
	}

	_, err = io.Copy(fileWriter, fh)
	if err != nil{
		fmt.Println("error copying file")
		return err
	}

	contentType := bodyWriter.FormDataContentType()
	bodyWriter.Close()

	resp, err := http.Post(targetUrl, contentType, bodyBuf)
	if err != nil {
		fmt.Println("error post to file upload url")
		return err
	}
	defer resp.Body.Close()
	resp_body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("error reading resp body")
		return err
	}

	fmt.Println(resp.Status)
	fmt.Println(string(resp_body))
	return nil
}
