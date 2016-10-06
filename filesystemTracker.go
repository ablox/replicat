// Copyright 2016 Jacob Taylor jacob@ablox.io
// License: Apache2 - http://www.apache.org/licenses/LICENSE-2.0
package main

import (
	"fmt"
	"github.com/rjeczalik/notify"
	"log"
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

type FilesystemTracker struct {
	directory       string
	contents        DirTreeMap
	setup           bool
	watcher         *ChangeHandler
	fsEventsChannel chan notify.EventInfo
	fsLock          sync.RWMutex
}

func (self *FilesystemTracker) init() {
	self.fsLock.Lock()
	defer self.fsLock.Unlock()

	if self.setup {
		return
	}

	fmt.Println("Setting up filesystemTracker!")
	self.contents = make(DirTreeMap)
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

func (self *FilesystemTracker) watchDirectory(directory string, watcher *ChangeHandler) {
	self.fsLock.Lock()
	defer self.fsLock.Unlock()

	if !self.setup {
		panic("watchDirectory called when not yet setup")
	}

	fmt.Printf("watchDirectory called with %s\n", directory)

	self.directory = directory
	self.watcher = watcher

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
	self.init()

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
	self.init()

	self.fsLock.Lock()
	defer self.fsLock.Unlock()

	fmt.Printf("%s before delete of: %s\n", len(self.contents), name)
	fmt.Printf("DeleteFolder: '%s'\n", name)
	delete(self.contents, name)
	fmt.Printf("%s after delete of: %s\n", len(self.contents), name)

	return nil
}

func (self *FilesystemTracker) ListFolders() (folderList []string) {
	self.init()

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
	//log.Println("We are NodeA send to our peers")
	//// SendEvent to all peers
	//for k, v := range serverMap {
	//	fmt.Printf("Considering sending to: %s\n", k)
	//	if k != globalSettings.Name {
	//		fmt.Printf("sending to peer %s at %s\n", k, v.Address)
	//		sendEvent(&event, v.Address, globalSettings.ManagerCredentials)
	//	}
	//}
}
