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

// ChangeHandler - Listener for tracking changes that happen to a storage system
type ChangeHandler interface {
	FolderCreated(name string) (err error)
	FolderDeleted(name string) (err error)
}

// StorageTracker - Listener that allows you to tell the tracker what has happened elsewhere so it can mirror the changes
type StorageTracker interface {
	CreatePath(relativePath string, isDirectory bool) (err error)
	DeleteFolder(name string) (err error)
	ListFolders() (folderList []string)
}

// LogOnlyChangeHandler - sample change handler
type LogOnlyChangeHandler struct {
}

// FolderCreated - track a new folder being created
func (handler *LogOnlyChangeHandler) FolderCreated(name string) (err error) {
	fmt.Printf("LogOnlyChangeHandler:FolderCreated: %s\n", name)
	return nil
}

// FolderDeleted - track a new folder being deleted
func (handler *LogOnlyChangeHandler) FolderDeleted(name string) (err error) {
	fmt.Printf("LogOnlyChangeHandler:FolderDeleted: %s\n", name)
	return nil
}

// FilesystemTracker - Track a filesystem and keep it in sync
type FilesystemTracker struct {
	directory         string
	contents          map[string]Directory
	setup             bool
	watcher           *ChangeHandler
	fsEventsChannel   chan notify.EventInfo
	renamesInProgress map[uint64]string // map from inode to source/destination of items being moved
	fsLock            sync.RWMutex
}

// Directory - struct
type Directory struct {
	os.FileInfo
	contents map[string]os.FileInfo
}

// NewDirectory - creates and returns a new Directory
func NewDirectory() *Directory {
	return &Directory{contents: make(map[string]os.FileInfo)}
}

func (handler *FilesystemTracker) init(directory string) {
	handler.fsLock.Lock()
	defer handler.fsLock.Unlock()

	if handler.setup {
		return
	}

	fmt.Printf("FilesystemTracker:init called with %s\n", directory)
	handler.directory = directory

	// Make the channel buffered to ensure no event is dropped. Notify will drop
	// an event if the receiver is not able to keep up the sending pace.
	handler.fsEventsChannel = make(chan notify.EventInfo, 10000)

	// Update the path that traffic is served from to be the filesystem canonical path. This will allow the event folders that come in to match what we have.
	fullPath := validatePath(directory)
	handler.directory = fullPath

	if fullPath != globalSettings.Directory {
		fmt.Printf("Updating serving directory to: %s\n", fullPath)
		handler.directory = fullPath
	}

	fmt.Println("Setting up filesystemTracker!")
	err := handler.scanFolders()
	if err != nil {
		panic(err)
	}

	handler.setup = true
}

func (handler *FilesystemTracker) cleanup() {
	handler.fsLock.Lock()
	defer handler.fsLock.Unlock()

	if !handler.setup {
		panic("cleanup called when not yet setup")
	}

	notify.Stop(handler.fsEventsChannel)
}

func (handler *FilesystemTracker) watchDirectory(watcher *ChangeHandler) {
	handler.fsLock.Lock()
	defer handler.fsLock.Unlock()

	if !handler.setup {
		panic("FilesystemTracker:watchDirectory called when not yet setup")
	}

	if handler.watcher != nil {
		panic("watchDirectory called a second time. Not allowed")
	}

	handler.watcher = watcher

	go handler.monitorLoop(handler.fsEventsChannel)

	// Set up a watch point listening for events within a directory tree rooted at the specified folder
	err := notify.Watch(handler.directory+"/...", handler.fsEventsChannel, notify.All)
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

// CreatePath tells the storage tracker to create a new path
func (handler *FilesystemTracker) CreatePath(relativePath string, isDirectory bool) (err error) {
	if !handler.setup {
		panic("FilesystemTracker:CreatePath called when not yet setup")
	}

	handler.fsLock.Lock()
	defer handler.fsLock.Unlock()

	absolutePath := handler.directory + "/" + relativePath

	if isDirectory {
		err = os.MkdirAll(absolutePath, os.ModeDir+os.ModePerm)
	} else {
		_, err = os.Create(absolutePath)
	}

	if err != nil && !os.IsExist(err) {
		panic(fmt.Sprintf("Error creating folder %s: %v\n", absolutePath, err))
	}

	_, exists := handler.contents[relativePath]
	fmt.Printf("CreatePath: '%s' (%v)\n", relativePath, exists)

	if !exists {
		directory := Directory{}
		directory.FileInfo, err = os.Stat(absolutePath)
		handler.contents[relativePath] = directory
	} else {
		fmt.Printf("for some reason the directory object already exists in the map: %s\n", relativePath)
	}

	return nil
}

// DeleteFolder - This storage handler should remove the specified path
func (handler *FilesystemTracker) DeleteFolder(name string) (err error) {
	if !handler.setup {
		panic("FilesystemTracker:DeleteFolder called when not yet setup")
	}

	handler.fsLock.Lock()
	defer handler.fsLock.Unlock()

	fmt.Printf("%d before delete of: %s\n", len(handler.contents), name)
	fmt.Printf("DeleteFolder: '%s'\n", name)
	delete(handler.contents, name)
	fmt.Printf("%d after delete of: %s\n", len(handler.contents), name)

	return nil
}

// ListFolders - This storage handler should return a list of contained folders.
func (handler *FilesystemTracker) ListFolders() (folderList []string) {
	if !handler.setup {
		panic("FilesystemTracker:ListFolders called when not yet setup")
	}

	handler.fsLock.Lock()
	defer handler.fsLock.Unlock()

	folderList = make([]string, len(handler.contents))
	index := 0

	for k := range handler.contents {
		folderList[index] = k
		index++
	}

	sort.Strings(folderList)
	return
}

// Monitor the filesystem looking for changes to files we are keeping track of.
func (handler *FilesystemTracker) monitorLoop(c chan notify.EventInfo) {
	directoryLength := len(handler.directory)
	for {
		ei := <-c

		fmt.Println("We have an event")
		fullPath := string(ei.Path())

		path := fullPath
		if len(fullPath) >= directoryLength && handler.directory == fullPath[:directoryLength] {
			if len(fullPath) == directoryLength {
				path = "."
			} else {
				// update the path to not have this prefix
				path = fullPath[directoryLength+1:]
			}
		}
		event := Event{Name: ei.Event().String(), Message: path}
		log.Printf("Event captured name: %s location: %s, ei.Path(): %s", event.Name, event.Message, ei.Path())

		isDirectory := handler.checkIfDirectory(event, path, fullPath)
		event.IsDirectory = isDirectory
		handler.processEvent(event, path)
	}
}

func (handler *FilesystemTracker) checkIfDirectory(event Event, path, fullPath string) bool {
	// Check to see if this was a path we knew about
	_, isDirectory := handler.contents[path]
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

func (handler *FilesystemTracker) processEvent(event Event, pathName string) {
	log.Printf("handleFilsystemEvent name: %s pathName: %s serverMap: %v\n", event.Name, pathName, serverMap)

	currentValue, exists := handler.contents[pathName]

	switch event.Name {
	case "notify.Create":
		fmt.Printf("processEvent: About to assign from one path to the next. Original: %s Map: %s\n", currentValue, handler.contents)
		// make sure there is an entry in the DirTreeMap for this folder. Since and empty list will always be returned, we can use that
		if !exists {
			handler.contents[pathName] = Directory{}
		}

		updatedValue, exists := handler.contents[pathName]
		if handler.watcher != nil {
			(*handler.watcher).FolderCreated(pathName)
		}
		fmt.Printf("notify.Create: Updated  value for %s: %v (%t)\n", pathName, updatedValue, exists)

	case "notify.Remove":
		// clean out the entry in the DirTreeMap for this folder
		delete(handler.contents, pathName)

		//todo FIXME: uhm, we just deleted this and now we are checking it? Uhm, ....
		updatedValue, exists := handler.contents[pathName]

		if handler.watcher != nil {
			(*handler.watcher).FolderDeleted(pathName)
		} else {
			fmt.Println("In the notify.Remove section but did not see a watcher")
		}

		fmt.Printf("notify.Remove: Updated  value for %s: %v (%t)\n", pathName, updatedValue, exists)

	// todo fix this to handle the two rename events to be one event
	case "notify.Rename":
		currentValue, exists := handler.contents[pathName]
		var iNode uint64
		//if exists && !(currentValue == nil) {
		if exists {
			sysInterface := currentValue.Sys()
			if sysInterface != nil {
				foo := sysInterface.(*syscall.Stat_t)
				iNode = foo.Ino
			}

			// At this point, the item is a directory and this is the original location
			destinationPath, exists := handler.renamesInProgress[iNode]
			if exists {
				handler.contents[destinationPath] = currentValue
				delete(handler.contents, pathName)

				fmt.Printf("Renamed: %s to: %s", pathName, destinationPath)
				//os.Rename(pathName, destinationPath)
			} else {
				fmt.Printf("Could not find rename in progress for iNode: %d\n%v\n", iNode, handler.renamesInProgress)
			}

		} else {
			fmt.Printf("Could not find %s in handler.contents\n%v\n", pathName, handler.contents)
		}

		//err := os.Remove(pathName)
		//if err != nil && !os.IsNotExist(err) {
		//	panic(fmt.Sprintf("Error deleting folder that was renamed %s: %v\n", pathName, err))
		//}
		//fmt.Printf("notify.Rename: %s\n", pathName)
	default:
		fmt.Printf("%s: %s not known, skipping\n", event.Name, pathName)
	}

	currentValue, exists = handler.contents[pathName]
	fmt.Printf("After: %s: Existing value for %s: %v (%v)\n", event.Name, pathName, currentValue, exists)

	// sendEvent to manager
	//sendEvent(&event, globalSettings.ManagerAddress, globalSettings.ManagerCredentials)
	SendEvent(event)
}

// Scan the files and folders inside of the directory we are watching and add them to the contents. This function
// can only be called inside of a writelock on handler.fsLock
func (handler *FilesystemTracker) scanFolders() error {
	pendingPaths := make([]string, 0, 100)
	pendingPaths = append(pendingPaths, handler.directory)
	handler.contents = make(map[string]Directory)

	for len(pendingPaths) > 0 {
		currentPath := pendingPaths[0]
		directory := NewDirectory()
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
				directory.contents[entry.Name()] = entry
			}
		}

		f.Close()
		if err != nil {
			return err
		}

		// Strip the base path off of the current path
		// make sure all of the paths are still '/' prefixed
		relativePath := currentPath[len(handler.directory):]
		if relativePath == "" {
			relativePath = "."
		}

		//todo add the directory stat into fileinfo
		info, err := os.Stat(currentPath)
		if err != nil {
			panic(fmt.Sprintf("Could not get stats on directory %s", currentPath))
		}
		directory.FileInfo = info
		handler.contents[relativePath] = *directory
	}

	return nil
}

// FileInfoSlice attaches the methods of Interface to []os.FileInfo, sorting in increasing order.
type FileInfoSlice []os.FileInfo

func (p FileInfoSlice) Len() int           { return len(p) }
func (p FileInfoSlice) Less(i, j int) bool { return p[i].Name() < p[j].Name() }
func (p FileInfoSlice) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }

// Sort is a convenience method.
//func (p FileInfoSlice) Sort() { Sort(p) }

// Strings sorts a slice of strings in increasing order.
//func fileInfos(a []os.FileInfo) { Sort(FileInfoSlice(a)) }

// sample code for monitoring the filesystem
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
