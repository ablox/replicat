// Copyright 2016 Jacob Taylor jacob@ablox.io
// License: Apache2 - http://www.apache.org/licenses/LICENSE-2.0
package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/rjeczalik/notify"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"syscall"
)

type ChangeHandler interface {
	FolderCreated(name string) (err error)
	FolderDeleted(name string) (err error)
}

type StorageTracker interface {
	CreateFolder(name string) (err error)
	DeleteFolder(name string) (err error)
	ListFolders() (folders []string, err error)
}

// sample change handler
type LogOnlyChangeHandler struct {
}

func (self *LogOnlyChangeHandler) FolderCreated(name string) (err error) {
	fmt.Printf("LogOnlyChangeHandler:FolderCreated: %s\n", name)
	return nil
}

func (self *LogOnlyChangeHandler) FolderDeleted(name string) (err error) {
	fmt.Printf("LogOnlyChangeHandler:FolderDeleted: %s\n", name)
	return nil
}

type Tracker struct {
}

var primaryTracker = new(Tracker)

func (self *Tracker) SendEvent(event Event) {
	// sendEvent to manager
	self.sendEvent(&event, globalSettings.ManagerAddress, globalSettings.ManagerCredentials)

	log.Println("We are NodeA send to our peers")
	// SendEvent to all peers
	for k, v := range serverMap {
		fmt.Printf("Considering sending to: %s\n", k)
		if k != globalSettings.Name {
			fmt.Printf("sending to peer %s at %s\n", k, v.Address)
			self.sendEvent(&event, v.Address, globalSettings.ManagerCredentials)
		}
	}
}

func (self *Tracker) sendEvent(event *Event, address string, credentials string) {
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

type FilesystemTracker struct {
	directory       string
	contents        DirTreeMap
	setup           bool
	watcher         *ChangeHandler
	fsEventsChannel chan notify.EventInfo
	fsLock          sync.RWMutex
}

func (self *FilesystemTracker) init(directory string) {
	self.fsLock.Lock()
	defer self.fsLock.Unlock()

	if self.setup {
		return
	}

	fmt.Printf("FilesystemTracker:init called with %s\n", directory)
	self.directory = directory

	// Make the channel buffered to ensure no event is dropped. Notify will drop
	// an event if the receiver is not able to keep up the sending pace.
	self.fsEventsChannel = make(chan notify.EventInfo, 1)

	// Update the path that traffic is served from to be the filesystem canonical path. This will allow the event folders that come in to match what we have.
	fullPath := validatePath(directory)
	self.directory = fullPath

	if fullPath != globalSettings.Directory {
		fmt.Printf("Updating serving directory to: %s\n", fullPath)
		self.directory = fullPath
	}

	fmt.Println("Setting up filesystemTracker!")
	err := self.scanFolders()
	if err != nil {
		panic(err)
	}
	self.setup = true
}

func (self *FilesystemTracker) cleanup() {
	self.fsLock.Lock()
	defer self.fsLock.Unlock()

	if !self.setup {
		panic("cleanup called when not yet setup")
	}

	notify.Stop(self.fsEventsChannel)
}

func (self *FilesystemTracker) watchDirectory(watcher *ChangeHandler) {
	self.fsLock.Lock()
	defer self.fsLock.Unlock()

	if !self.setup {
		panic("FilesystemTracker:watchDirectory called when not yet setup")
	}

	if self.watcher != nil {
		panic("watchDirectory called a second time. Not allowed")
	}

	self.watcher = watcher

	go self.monitorLoop(self.fsEventsChannel)

	// Set up a watch point listening for events within a directory tree rooted at the specified folder
	err := notify.Watch(self.directory+"/...", self.fsEventsChannel, notify.All)
	if err != nil {
		log.Panic(err)
	}
}

func validatePath(directory string) (fullPath string) {
	fullPath, err := filepath.EvalSymlinks(directory)
	if err != nil {
		var err2, err3 error
		// We have an error. If the directory does not exist, then try to create it. Fail if we cannot create it and return the original error
		if os.IsNotExist(err) {
			err2 = os.Mkdir(directory, os.ModeDir+os.ModePerm)
			fullPath, err3 = filepath.EvalSymlinks(directory)
			if err2 != nil || err3 != nil {
				panic(fmt.Sprintf("err: %v\nerr2: %v\nerr3: %v\n", err, err2, err3))
			}
		} else {
			panic(err)
		}
	}

	return
}

func (self *FilesystemTracker) CreateFolder(name string) (err error) {
	if !self.setup {
		panic("FilesystemTracker:CreateFolder called when not yet setup")
	}

	self.fsLock.Lock()
	defer self.fsLock.Unlock()

	_, exists := self.contents[name]
	fmt.Printf("CreateFolder: '%s' (%v)\n", name, exists)

	if !exists {
		self.contents[name] = make([]string, 0, 10)
	}

	return nil
}

func (self *FilesystemTracker) DeleteFolder(name string) (err error) {
	if !self.setup {
		panic("FilesystemTracker:DeleteFolder called when not yet setup")
	}

	self.fsLock.Lock()
	defer self.fsLock.Unlock()

	fmt.Printf("%d before delete of: %s\n", len(self.contents), name)
	fmt.Printf("DeleteFolder: '%s'\n", name)
	delete(self.contents, name)
	fmt.Printf("%d after delete of: %s\n", len(self.contents), name)

	return nil
}

func (self *FilesystemTracker) ListFolders() (folderList []string) {
	if !self.setup {
		panic("FilesystemTracker:ListFolders called when not yet setup")
	}

	self.fsLock.Lock()
	defer self.fsLock.Unlock()

	folderList = make([]string, len(self.contents))
	index := 0

	for k, _ := range self.contents {
		folderList[index] = k
		index++
	}

	sort.Strings(folderList)
	return
}

// Monitor the filesystem looking for changes to files we are keeping track of.
func (self *FilesystemTracker) monitorLoop(c chan notify.EventInfo) {
	directoryLength := len(self.directory)
	for {
		ei := <-c

		fmt.Println("We have an event")
		fullPath := string(ei.Path())

		path := fullPath
		if len(fullPath) >= directoryLength && self.directory == fullPath[:directoryLength] {
			if len(fullPath) == directoryLength {
				path = "."
			} else {
				// update the path to not have this prefix
				path = fullPath[directoryLength+1:]
			}
		}

		event := Event{Name: ei.Event().String(), Message: path}
		log.Printf("Event captured name: %s location: %s", event.Name, event.Message)

		// It looks like there is some sort of reversal of paths now. where adds are being removed from the other side.
		isDirectory := self.checkIfDirectory(event, path, fullPath)

		if isDirectory {
			self.processEvent(event, path)
		}

	}
}

func (self *FilesystemTracker) checkIfDirectory(event Event, path, fullPath string) bool {
	// Check to see if this was a path we knew about
	_, isDirectory := self.contents[path]
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

	fmt.Printf("checkIfDirectory: event raw data: %s with path: %s fullPath: %s isDirectory: %v iNode: %v\n", event.Name, path, fullPath, isDirectory, iNode)

	return isDirectory

}

func (self *FilesystemTracker) processEvent(event Event, pathName string) {
	log.Printf("handleFilsystemEvent name: %s pathName: %s serverMap: %v\n", event.Name, pathName, serverMap)

	currentValue, exists := self.contents[pathName]
	//fmt.Printf("Before: %s: Existing value for %s: %v (%v)\n", event.Name, pathName, currentValue, exists)

	switch event.Name {
	case "notify.Create":
		fmt.Printf("processEvent: About to assign from one path to the next. Original: %s Map: %s\n", self.contents[pathName], self.contents)
		// make sure there is an entry in the DirTreeMap for this folder. Since and empty list will always be returned, we can use that
		_, exists := self.contents[pathName]
		if !exists {
			self.contents[pathName] = make([]string, 0)
		}

		updated_value, exists := self.contents[pathName]
		if self.watcher != nil {
			(*self.watcher).FolderCreated(pathName)
		}
		fmt.Printf("notify.Create: Updated  value for %s: %v (%s)\n", pathName, updated_value, exists)

	case "notify.Remove":
		// clean out the entry in the DirTreeMap for this folder
		delete(self.contents, pathName)

		updated_value, exists := self.contents[pathName]

		if self.watcher != nil {
			(*self.watcher).FolderDeleted(pathName)
		} else {
			fmt.Println("In the notify.Remove section but did not see a watcher")
		}

		fmt.Printf("notify.Remove: Updated  value for %s: %v (%s)\n", pathName, updated_value, exists)

	// todo fix this to handle the two rename events to be one event
	//case "notify.Rename":
	//	err = os.Remove(pathName)
	//	if err != nil && !os.IsNotExist(err) {
	//		panic(fmt.Sprintf("Error deleting folder that was renamed %s: %v\n", pathName, err))
	//	}
	//	fmt.Printf("notify.Rename: %s\n", pathName)
	default:
		fmt.Printf("%s: %s not known, skipping\n", event.Name, pathName)
	}

	currentValue, exists = self.contents[pathName]
	fmt.Printf("After: %s: Existing value for %s: %v (%v)\n", event.Name, pathName, currentValue, exists)

	// sendEvent to manager
	sendEvent(&event, globalSettings.ManagerAddress, globalSettings.ManagerCredentials)

	// todo - make this actually send to the peers
	log.Println("TODO Send to peers here")
}

// Scan the files and folders inside of the directory we are watching and add them to the contents. This function
// can only be called inside of a writelock on self.fsLock
func (self *FilesystemTracker) scanFolders() error {
	pendingPaths := make([]string, 0, 100)
	pendingPaths = append(pendingPaths, self.directory)
	self.contents = make(DirTreeMap)

	for len(pendingPaths) > 0 {
		currentPath := pendingPaths[0]
		fileList := make([]string, 0, 100)
		pendingPaths = pendingPaths[1:]

		// Read the directories in the path
		f, err := os.Open(currentPath)
		if err != nil {
			return err
		}

		dirEntries, err := f.Readdir(-1)
		for _, entry := range dirEntries {
			if entry.IsDir() {
				newDirectory := filepath.Join(currentPath, entry.Name())
				pendingPaths = append(pendingPaths, newDirectory)
			} else {
				fileList = append(fileList, entry.Name())
			}
		}

		f.Close()
		if err != nil {
			return err
		}

		sort.Strings(fileList)

		// Strip the base path off of the current path
		// make sure all of the paths are still '/' prefixed
		relativePath := currentPath[len(self.directory):]
		if relativePath == "" {
			relativePath = "."
		}

		self.contents[relativePath] = fileList
	}

	return nil
}
