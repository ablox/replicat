// Package replicat is a server for n way synchronization of content (rsync for the cloud).
// More information at: http://replic.at
// Copyright 2016 Jacob Taylor jacob@ablox.io
// License: Apache2 - http://www.apache.org/licenses/LICENSE-2.0
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/rjeczalik/notify"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"syscall"
	"time"
)

// ChangeHandler - Listener for tracking changes that happen to a storage system
type ChangeHandler interface {
	FolderCreated(name string) (err error)
	FolderDeleted(name string) (err error)
	FolderUpdated(name string) (err error)
	FileCreated(name string) (err error)
	FileDeleted(name string) (err error)
	FileUpdated(name string) (err error)
}

// StorageTracker - Listener that allows you to tell the tracker what has happened elsewhere so it can mirror the changes
type StorageTracker interface {
	CreatePath(pathName string, isDirectory bool) error
	Rename(sourcePath string, destinationPath string, isDirectory bool) (err error)
	DeleteFolder(name string) (err error)
	ListFolders(getLocks bool) (folderList []string)
	SendCatalog()
	ProcessCatalog(event Event)
	sendRequestedPaths(pathEntries map[string]EntryJSON, targetServerName string)
	getEntryJSON(relativePath string) (EntryJSON, error)
	GetStatistics() map[string]string
}

// FilesystemTracker - Track a filesystem and keep it in sync
type FilesystemTracker struct {
	directory         string
	contents          map[string]Entry
	setup             bool
	watcher           *ChangeHandler
	fsEventsChannel   chan notify.EventInfo
	renamesInProgress map[uint64]renameInformation // map from inode to source/destination of items being moved
	fsLock            sync.RWMutex
	server            *ReplicatServer
	neededFiles       map[string]EntryJSON
	stats             TrackerStats
}

type TrackerStats struct {
	TotalFiles       int
	TotalFolders     int
	FilesSent        int
	FilesReceived    int
	FilesDeleted     int
	CatalogsSent     int
	CatalogsReceived int
}

const (
	TRACKER_TOTAL_FILES       = "TotalFiles"
	TRACKER_TOTAL_FOLDERS     = "TotalFolders"
	TRACKER_FILES_SENT        = "FilesSent"
	TRACKER_FILES_RECEIVED    = "FilesReceived"
	TRACKER_FILES_DELETED     = "FilesDeleted"
	TRACKER_CATALOGS_SENT     = "CatalogsSent"
	TRACKER_CATALOGS_RECEIVED = "CatalogsRecieved"
)

// Entry - contains the data for a file
type Entry struct {
	os.FileInfo
	setup bool
	hash  []byte
}

// NewDirectory - creates and returns a new Directory
func NewDirectory() *Entry {
	return &Entry{setup: true}
}

// NewDirectoryFromFileInfo - creates and returns a new Directory based on a fileinfo structure
func NewDirectoryFromFileInfo(info *os.FileInfo) *Entry {
	return &Entry{*info, true, nil}
}

func (handler *FilesystemTracker) printTracker() {
	handler.printLockable(true)
}

func (handler *FilesystemTracker) startTest(name string) {
	event := Event{Name: "startTest", Path: name, Source: globalSettings.Name}
	SendEvent(event, "")
}

func (handler *FilesystemTracker) endTest(name string) {
	event := Event{Name: "endTest", Path: name, Source: globalSettings.Name}
	SendEvent(event, "")
}

func (handler *FilesystemTracker) printLockable(lock bool) {
	if lock {
		fmt.Println("FilesystemTracker:print")
		handler.fsLock.RLock()
		fmt.Println("FilesystemTracker:/print")
		defer handler.fsLock.RUnlock()
		defer fmt.Println("FilesystemTracker://print")
	}

	folders := make([]string, 0, len(handler.contents))
	for dir := range handler.contents {
		folders = append(folders, dir)
	}
	sort.Strings(folders)

	fmt.Println("~~~~~~~~~~~~~~~~~~~~~~~")
	fmt.Printf("~~~~%s Tracker report setup(%v)\n", handler.directory, handler.setup)
	fmt.Printf("~~~~contents: %v\n", handler.contents)
	fmt.Printf("~~~~folders: %v\n", folders)
	fmt.Printf("~~~~renames in progress: %v\n", handler.renamesInProgress)
	fmt.Println("~~~~~~~~~~~~~~~~~~~~~~~")
}

func (handler *FilesystemTracker) validate() {
	handler.fsLock.RLock()
	defer handler.fsLock.RUnlock()
	fmt.Println("~~~~~~~~~~~~~~~~~~~~~~~")
	fmt.Printf("~~~~%s Tracker validation starting on %d folders\n", handler.directory, len(handler.contents))

	for name, dir := range handler.contents {
		if !dir.setup {
			panic(fmt.Sprintf("tracker validation failed on directory: %s\n", name))
		}
	}

	fmt.Println("~~~~~~~~~~~~~~~~~~~~~~~")
}

func (handler *FilesystemTracker) GetStatistics() (stats map[string]string) {
	stats = make(map[string]string, 7)
	stats[TRACKER_TOTAL_FILES] = strconv.Itoa(handler.stats.TotalFiles)
	stats[TRACKER_TOTAL_FOLDERS] = strconv.Itoa(handler.stats.TotalFolders)
	stats[TRACKER_FILES_SENT] = strconv.Itoa(handler.stats.FilesSent)
	stats[TRACKER_FILES_RECEIVED] = strconv.Itoa(handler.stats.FilesReceived)
	stats[TRACKER_FILES_DELETED] = strconv.Itoa(handler.stats.FilesDeleted)
	stats[TRACKER_CATALOGS_SENT] = strconv.Itoa(handler.stats.CatalogsSent)
	stats[TRACKER_CATALOGS_RECEIVED] = strconv.Itoa(handler.stats.CatalogsReceived)

	fmt.Printf("Files: %d\tFolders:%d\tFiles Sent: %d\tReceived %d\tDeleted: %d\tCatalogs Sent: %d\tReceived: %d\n", handler.stats.TotalFiles, handler.stats.TotalFolders, handler.stats.FilesSent, handler.stats.FilesReceived, handler.stats.FilesDeleted, handler.stats.CatalogsSent, handler.stats.CatalogsReceived)

	return
}

func (handler *FilesystemTracker) init(directory string, server *ReplicatServer) {
	fmt.Println("FilesystemTracker:init")
	handler.fsLock.Lock()
	defer handler.fsLock.Unlock()
	fmt.Println("FilesystemTracker:/init")
	defer fmt.Println("FilesystemTracker://init")

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

	handler.renamesInProgress = make(map[uint64]renameInformation, 0)

	fmt.Println("Setting up filesystemTracker!")
	handler.printLockable(false)

	fmt.Println("FilesystemTracker:init starting folder scan looking for initial files")
	err := handler.scanFolders()
	if err != nil {
		panic(err)
	}

	// Set the status to be done with initial scan
	server.SetStatus(REPLICAT_STATUS_JOINING_CLUSTER)
	handler.printLockable(false)
	handler.setup = true
}

func (handler *FilesystemTracker) cleanup() {
	fmt.Println("FilesystemTracker:cleanup")
	handler.fsLock.Lock()
	defer handler.fsLock.Unlock()
	fmt.Println("FilesystemTracker:/cleanup")
	defer fmt.Println("FilesystemTracker://cleanup")

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

func createPath(pathName string, absolutePathName string) (pathCreated bool, stat os.FileInfo, err error) {
	for maxCycles := 0; maxCycles < 5 && pathName != ""; maxCycles++ {
		stat, err = os.Stat(absolutePathName)

		if err == nil {
			fmt.Printf("Path existed: %s\n", absolutePathName)
			pathCreated = true
			return
		} else {
			// if there is an error, go to create the path
			fmt.Printf("Creating path: %s\n", absolutePathName)
			err = os.MkdirAll(absolutePathName, os.ModeDir+os.ModePerm)
		}

		// after attempting to create the path, check the err again
		if err == nil {
			fmt.Printf("Path was created or existed: %s\n", absolutePathName)
			pathCreated = true
			stat, err = os.Stat(absolutePathName)
			fmt.Printf("os.Stat returned stat: %#v, err: %#v\n", stat, err)
			return
		} else if os.IsExist(err) {
			fmt.Printf("Path already exists: %s\n", absolutePathName)
			pathCreated = true
			stat, err = os.Stat(absolutePathName)
			return
		}

		fmt.Printf("Error (%v) encountered creating path, going to try again. Attempt: %d\n", err, maxCycles)
		time.Sleep(20 * time.Millisecond)
	}

	pathCreated = false
	return pathCreated, stat, err
}

func (handler *FilesystemTracker) sendRequestedPaths(pathEntries map[string]EntryJSON, targetServerName string) {
	serverAddress := serverMap[targetServerName].Address
	currentPath := globalSettings.Directory

	handler.fsLock.RLock()
	defer handler.fsLock.RUnlock()

	for p, entry := range pathEntries {
		fullPath := filepath.Join(currentPath, p)
		realEntry := handler.contents[p]
		fmt.Printf("File info for: %s\nProvided: %#v\nFile info for: %s\nStorege : %#v\n", p, entry, p, realEntry)
		if !entry.IsDirectory {
			fmt.Printf("Requested file information: %s\n%#v\n", p, entry)
			go postHelper(p, fullPath, serverAddress, globalSettings.ManagerCredentials)
		}
	}
}

func (handler *FilesystemTracker) getEntryJSON(relativePath string) (EntryJSON, error) {
	handler.fsLock.RLock()
	defer handler.fsLock.RUnlock()

	// get the current entry
	currentEntry, exists := handler.contents[relativePath]
	if exists == false {
		return EntryJSON{}, errors.New("File Does Not Exist")
	}

	if currentEntry.setup == false {
		return EntryJSON{}, errors.New("File information is unset")
	}

	result := EntryJSON{RelativePath: relativePath,
		IsDirectory: currentEntry.IsDir(),
		Hash:        currentEntry.hash,
		ModTime:     currentEntry.ModTime(),
		Size:        currentEntry.Size(),
		ServerName:  globalSettings.Name}

	return result, nil
}

// createPath implements the new path/file creation. Locking is done outside this call.
func (handler *FilesystemTracker) createPath(pathName string, isDirectory bool) (err error) {
	relativePathName := pathName
	file := ""

	if !isDirectory {
		relativePathName, file = filepath.Split(pathName)
	}

	//fmt.Printf("Path name before any adjustments (directory: %v)\npath: %s\nfile: %s\n", isDirectory, relativePathName, file)
	if len(relativePathName) > 0 && relativePathName[len(relativePathName)-1] == filepath.Separator {
		fmt.Println("Stripping out path ending")
		relativePathName = relativePathName[:len(relativePathName)-1]
	}

	absolutePathName := filepath.Join(handler.directory, relativePathName)
	fmt.Printf("**********\npath was split into \nrelativePathName: %s \nfile: %s \nabsolutePath: %s\n**********\n", relativePathName, file, absolutePathName)

	pathCreated, stat, err := createPath(pathName, absolutePathName)

	if err != nil && !os.IsExist(err) {
		panic(fmt.Sprintf("Error creating folder %s: %v\n", relativePathName, err))
	}

	if pathCreated {
		handler.contents[relativePathName] = *NewDirectoryFromFileInfo(&stat)
	}

	if !isDirectory {
		completeAbsoluteFilePath := filepath.Join(handler.directory, pathName)
		fmt.Printf("We are creating a file at: %s\n", completeAbsoluteFilePath)

		// We also need to create the file.
		for maxCycles := 0; maxCycles < 5; maxCycles++ {
			stat, err = os.Stat(completeAbsoluteFilePath)
			fmt.Printf("Stat call done for\npath: %s\nerr: %v\nstat: %v\n", completeAbsoluteFilePath, err, stat)
			var newFile *os.File

			// if there is an error, go to create the file
			fmt.Println("before")
			if err != nil {
				fmt.Printf("Creating file: %s\n", completeAbsoluteFilePath)
				newFile, err = os.Create(completeAbsoluteFilePath)
				fmt.Printf("Attempt to create file finished\n err: %v\n path: %s\n", err, completeAbsoluteFilePath)
			}
			fmt.Println("after")

			// after attempting to create the file, check the err again
			if err == nil {
				newFile.Close()
				stat, err = os.Stat(completeAbsoluteFilePath)
				fmt.Printf("file was created or existed: %s\n", completeAbsoluteFilePath)
				break
			}

			fmt.Printf("Error (%v) encountered creating file, going to try again. Attempt: %d\n", err, maxCycles)
			time.Sleep(20 * time.Millisecond)
		}

		if err != nil {
			panic(fmt.Sprintf("Error creating file %s: %v\n", completeAbsoluteFilePath, err))
		}

		handler.contents[relativePathName] = *NewDirectoryFromFileInfo(&stat)
	}

	return
}

// CreatePath tells the storage tracker to create a new path
func (handler *FilesystemTracker) CreatePath(pathName string, isDirectory bool) error {
	fmt.Printf("FilesystemTracker:CreatePath called with relativePath: %s isDirectory: %v\n", pathName, isDirectory)
	handler.fsLock.Lock()
	defer handler.fsLock.Unlock()
	fmt.Println("FilesystemTracker:/CreatePath")
	defer fmt.Println("FilesystemTracker://CreatePath")

	if !handler.setup {
		panic("FilesystemTracker:CreatePath called when not yet setup")
	}

	return handler.createPath(pathName, isDirectory)
}

func (handler *FilesystemTracker) handleCompleteRename(sourcePath string, destinationPath string, isDirectory bool) (err error) {

	// First we do the simple rename where both sides are here.
	absoluteSourcePath := filepath.Join(handler.directory, sourcePath)
	absoluteDestinationPath := filepath.Join(handler.directory, destinationPath)

	for maxCycles := 0; maxCycles < 5; maxCycles++ {
		err = os.Rename(absoluteSourcePath, absoluteDestinationPath)

		// after attempting to create the path, check the err again
		if err == nil {
			fmt.Println("Rename Complete")
			break
		}

		fmt.Printf("Error (%v) encountered moving path, going to try again. Attempt: %d\n", err, maxCycles)
		time.Sleep(20 * time.Millisecond)
	}

	if err != nil {
		panic(fmt.Sprintf("Rename failed (%v)!  source: %s dest: %s directory %v\n", err, sourcePath, destinationPath, isDirectory))
	}

	return err
}

// Rename - Rename a file or folder from one location to another
func (handler *FilesystemTracker) Rename(sourcePath string, destinationPath string, isDirectory bool) (err error) {
	fmt.Println("FilesystemTracker:Rename")
	handler.fsLock.Lock()
	defer handler.fsLock.Unlock()
	fmt.Println("FilesystemTracker:/Rename")
	defer fmt.Println("FilesystemTracker://Rename")

	fmt.Printf("We have a rename event. source: %s dest: %s directory %v\n", sourcePath, destinationPath, isDirectory)

	if !handler.setup {
		panic("FilesystemTracker:CreatePath called when not yet setup")
	}

	// If we have a source and destination, perform the move to mirror the other side
	if len(sourcePath) > 0 && len(destinationPath) > 0 {
		fmt.Println("FilesystemTracker:Rename completing")
		err = handler.handleCompleteRename(sourcePath, destinationPath, isDirectory)
	} else if sourcePath == "" {
		fmt.Println("FilesystemTracker:Rename creating a new path")
		// If the file was moved into the monitored folder from nowhere, ...
		// bypass the locking by using the internal method since we already have the lockings and safety checks
		err = handler.createPath(destinationPath, isDirectory)
	} else if destinationPath == "" {
		fmt.Println("FilesystemTracker:Rename deleting existing path")
		// If the file or folder was moved out of the monitored folder, get rid of it.
		delete(handler.contents, sourcePath)
		// todo - shortcut the events on this one. Should create a bit of a storm.
		absolutePathForDeletion := filepath.Join(handler.directory, sourcePath)
		fmt.Printf("About to call os.RemoveAll on: %s\n", absolutePathForDeletion)
		err = os.RemoveAll(absolutePathForDeletion)
		if err != nil {
			panic(fmt.Sprintf("%v encountered when attempting to os.RemoveAll(%s)", err, absolutePathForDeletion))
		}
	} else {
		panic("Enexpected case encountered in rename")
	}

	fmt.Printf("Rename Complete source: %s dest: %s directory %v\n", sourcePath, destinationPath, isDirectory)
	return
}

// DeleteFolder - This storage handler should remove the specified path
func (handler *FilesystemTracker) DeleteFolder(name string) error {
	fmt.Println("FilesystemTracker:DeleteFolder")
	handler.fsLock.Lock()
	defer handler.fsLock.Unlock()
	fmt.Println("FilesystemTracker:/DeleteFolder")
	defer fmt.Println("FilesystemTracker://DeleteFolder")

	if !handler.setup {
		panic("FilesystemTracker:DeleteFolder called when not yet setup")
	}

	fmt.Printf("DeleteFolder: '%s'\n", name)
	delete(handler.contents, name)
	fmt.Printf("%d after delete of: %s\n", len(handler.contents), name)

	return nil
}

// ListFolders - This storage handler should return a list of contained folders.
func (handler *FilesystemTracker) ListFolders(getLocks bool) (folderList []string) {
	fmt.Printf("FilesystemTracker:ListFolders getLocks: %v\n", getLocks)
	if getLocks {
		handler.fsLock.Lock()
		defer handler.fsLock.Unlock()
		fmt.Println("FilesystemTracker:/ListFolders")
		defer fmt.Println("FilesystemTracker://ListFolders")
	}

	if !handler.setup {
		panic("FilesystemTracker:ListFolders called when not yet setup")
	}

	folderList = make([]string, len(handler.contents))
	index := 0

	for k := range handler.contents {
		folderList[index] = k
		index++
	}

	sort.Strings(folderList)
	return
}

func extractPaths(handler *FilesystemTracker, ei *notify.EventInfo) (path, fullPath string) {
	directoryLength := len(handler.directory)
	fullPath = string((*ei).Path())

	path = fullPath
	if len(fullPath) >= directoryLength && handler.directory == fullPath[:directoryLength] {
		if len(fullPath) == directoryLength {
			return "", ""
		}

		// update the path to not have this prefix
		path = fullPath[directoryLength+1:]
	}

	return
}

// Monitor the filesystem looking for changes to files we are keeping track of.
func (handler *FilesystemTracker) monitorLoop(c chan notify.EventInfo) {
	// filesToIgnore - If you run into one of these files, do not sync it to other side.
	filesToIgnore := map[string]bool{
		".DS_Store": true,
		"Thumbs.db": true,
	}

	for {
		ei := <-c

		fmt.Printf("*****We have an event: %v\nwith Sys: %v\npath: %v\nevent: %v\n", ei, ei.Sys(), ei.Path(), ei.Event())

		path, fullPath := extractPaths(handler, &ei)
		// Skip empty paths
		if path == "" {
			fmt.Println("blank path. Ignore!")
			continue
		}

		//.DS_Store
		// split out the filename and check to see if it is on the ignore list.
		_, testFile := filepath.Split(ei.Path())
		_, exists := filesToIgnore[testFile]
		if exists {
			fmt.Printf("Ignoring event for file on ignore list: %s - %s\n", ei.Event(), testFile)
			continue
		}

		event := Event{Name: ei.Event().String(), Path: path, Source: globalSettings.Name}
		log.Printf("Event captured name: %s location: %s, ei.Path(): %s", event.Name, event.Path, ei.Path())

		isDirectory := handler.checkIfDirectory(event, path, fullPath)
		event.IsDirectory = isDirectory
		handler.processEvent(event, path, fullPath, true)
	}
}

func (handler *FilesystemTracker) checkIfDirectory(event Event, path, fullPath string) bool {
	fmt.Println("FilesystemTracker:checkIfDirectory")
	handler.fsLock.Lock()
	defer handler.fsLock.Unlock()
	fmt.Println("FilesystemTracker:/checkIfDirectory")
	defer fmt.Println("FilesystemTracker://checkIfDirectory")

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

type renameInformation struct {
	iNode           uint64
	sourceDirectory *Entry
	sourcePath      string
	sourceSet       bool
	destinationStat os.FileInfo
	destinationPath string
	destinationSet  bool
}

func getiNodeFromStat(stat os.FileInfo) uint64 {
	fmt.Printf("getiNodeFromStat called with %#v\n", stat)
	if stat == nil {
		return 0
	}

	sysInterface := stat.Sys()
	if sysInterface != nil {
		foo := sysInterface.(*syscall.Stat_t)
		return foo.Ino
	}
	return 0
}

const (
	// TRACKER_RENAME_TIMEOUT - the amount of time to sleep while waiting for the second event in a rename process
	TRACKER_RENAME_TIMEOUT = time.Millisecond * 250
)

// WaitFor - a helper function that will politely wait until the helper argument function evaluates to the value of waitingFor.
// this is great for waiting for the tracker to catch up to the filesystem in tests, for example.
func WaitFor(tracker *FilesystemTracker, folder string, waitingFor bool, helper func(tracker *FilesystemTracker, folder string) bool) bool {
	roundTrips := 0
	for {
		if waitingFor == helper(tracker, folder) {
			return true
		}

		if roundTrips > 50 {
			return false
		}
		roundTrips++

		time.Sleep(50 * time.Millisecond)
	}
}

// completeRenameIfAbandoned - if there is a rename that was started with a source
// but has been left pending for more than TRACKER_RENAME_TIMEOUT, complete it (i.e. move the folder away)
func (handler *FilesystemTracker) completeRenameIfAbandoned(iNode uint64) {
	time.Sleep(TRACKER_RENAME_TIMEOUT)

	handler.fsLock.Lock()
	defer handler.fsLock.Unlock()

	inProgress, exists := handler.renamesInProgress[iNode]
	if !exists {
		fmt.Printf("Rename for iNode %d appears to have been completed\n", iNode)
		return
	}

	// We have the inProgress. Clean it up
	if inProgress.sourceSet {
		relativePath := inProgress.sourcePath[len(handler.directory)+1:]
		fmt.Printf("File at: %s (iNode %d) appears to have been moved away. Removing it\n", relativePath, iNode)
		delete(handler.contents, relativePath)

		// tell the other nodes that a rename was done.
		event := Event{Name: "replicat.Rename", Source: globalSettings.Name, SourcePath: relativePath}
		SendEvent(event, inProgress.sourcePath)
	} else if inProgress.destinationSet {
		fmt.Printf("directory: %s src: %s dest: %s\n", handler.directory, inProgress.sourcePath, inProgress.destinationPath)
		fmt.Printf("inProgress: %v\n", handler.renamesInProgress)
		fmt.Printf("this inProgress: %v\n", inProgress)

		//this code failed because of an slice index out of bounds error. It is a reasonable copy with both sides and
		//yet it was initialized blank. The first event was for the copy and it should have existed. Ahh, it is a file
		if len(inProgress.destinationPath) < len(handler.directory)+1 {
			fmt.Print("About to pop.....")
		}

		relativePath := inProgress.destinationPath[len(handler.directory)+1:]
		handler.contents[relativePath] = *NewDirectoryFromFileInfo(&inProgress.destinationStat)

		// tell the other nodes that a rename was done.
		event := Event{Name: "replicat.Rename", Source: globalSettings.Name, Path: relativePath, ModTime: inProgress.destinationStat.ModTime(),
			IsDirectory: inProgress.destinationStat.IsDir()}
		SendEvent(event, inProgress.destinationPath)

		// find the source if the destination is an iNode in our system
		// This is probably expensive. :) Wait until there is a flag of a cleanup
		// We also have to watch out for hardlinks if those would share iNodes.
		//for name, directory := range handler.contents {
		//	directoryID := getiNodeFromStat(directory)
		//	if directoryID == iNode {
		//		fmt.Println("found the source file, using it!")
		//		handler.contents[relativePath] = directory
		//		delete(handler.contents, name)
		//		return
		//	}
		//}
	} else {
		fmt.Printf("FilesystemTracker:completeRenameIfAbandoned In progress item that appears to be not set. iNode %d, inProgress: %#v\n", iNode, inProgress)
	}

	delete(handler.renamesInProgress, iNode)
}

func (handler *FilesystemTracker) handleRename(event Event, pathName, fullPath string) {
	fmt.Printf("FilesystemTracker:handleRename: pathname: %s fullPath: %s\n", pathName, fullPath)
	// Either the path should exist in the filesystem or it should exist in the stored tree. Find it.

	// check to see if this folder currently exists. If it does, it is the destination
	tmpDestinationStat, err := os.Stat(fullPath)
	tmpDestinationSet := false
	if err == nil {
		tmpDestinationSet = true
	}

	// Check to see if we have source information for this folder, if we do, it is the source.
	tmpSourceDirectory, tmpSourceSet := handler.contents[pathName]

	//todo can it be possible to get real values in error? i.e. can I move to an existing folder and pick up its information accidentally?
	var iNode uint64

	if tmpDestinationSet {
		iNode = getiNodeFromStat(tmpDestinationStat)
	} else if tmpSourceSet && tmpSourceDirectory.setup {
		//fmt.Printf("About to get iNodefromStat. Handler: %#v\n", handler)
		//fmt.Printf("tmpSourceDirectory: %#v\n", tmpSourceDirectory)
		iNode = getiNodeFromStat(tmpSourceDirectory)
	}

	inProgress, _ := handler.renamesInProgress[iNode]

	if !inProgress.sourceSet && tmpSourceSet {
		inProgress.sourceDirectory = &tmpSourceDirectory
		inProgress.sourcePath = fullPath
		inProgress.sourceSet = true
		fmt.Printf("^^^^^^^Source found, deleting pathName '%s' from contents. Current transfer is: %v\n", pathName, inProgress)
		delete(handler.contents, pathName)
	}

	if !inProgress.destinationSet && tmpDestinationSet {
		inProgress.destinationStat = tmpDestinationStat
		inProgress.destinationPath = fullPath
		inProgress.destinationSet = true
	}

	fmt.Printf("^^^^^^^Current transfer is: %#v\n", inProgress)

	if inProgress.destinationSet && inProgress.sourceSet {
		fmt.Printf("directory: %s src: %s dest: %s\n", handler.directory, inProgress.sourcePath, inProgress.destinationPath)

		relativeDestination := inProgress.destinationPath[len(handler.directory)+1:]
		relativeSource := inProgress.sourcePath[len(handler.directory)+1:]
		fmt.Printf("moving from source: %s (%s) to destination: %s (%s)\n", inProgress.sourcePath, relativeSource, inProgress.destinationPath, relativeDestination)

		handler.contents[relativeDestination] = *NewDirectoryFromFileInfo(&inProgress.destinationStat)
		delete(handler.contents, relativeSource)
		delete(handler.renamesInProgress, iNode)

		// tell the other nodes that a rename was done.
		event := Event{Name: "replicat.Rename", Path: relativeDestination, Source: globalSettings.Name, SourcePath: relativeSource}
		// todo - verify relativeDestination is the right thing to send here
		SendEvent(event, relativeDestination)

	} else {
		fmt.Printf("^^^^^^^We do not have both a source and destination - schedule and save under iNode: %d Current transfer is: %#v\n", iNode, inProgress)
		inProgress.iNode = iNode
		handler.renamesInProgress[iNode] = inProgress
		go handler.completeRenameIfAbandoned(iNode)
	}
}

//Create pile of folders with other servers offline
//stop C
//Duplicate all of C
//Start C
//Remove all of the duplicates in C
//C says, we do not own this, do not send
//2017/03/06 23:36:20.917030 server.go:88: Original ownership: main.Event{Source:"NodeA", Name:"notify.Create", Path:"trackertest copy.go", SourcePath:"", Time:time.Time{sec:63624468969, nsec:768126848, loc:(*time.Location)(0x45168c0)}, ModTime:time.Time{sec:0, nsec:0, loc:(*time.Location)(0x4512d60)}, IsDirectory:false, NetworkSource:"", RawData:[]uint8(nil)}
//
//let them replicate

func (handler *FilesystemTracker) processEvent(event Event, pathName, fullPath string, lock bool) {
	fmt.Println("FilesystemTracker:processEvent")
	if lock {
		handler.fsLock.Lock()
	}
	fmt.Println("FilesystemTracker:/processEvent")
	defer fmt.Println("FilesystemTracker://processEvent")

	fmt.Printf("*********************************************************\nhandleFilsystemEvent name: %s pathName: %s\n*********************************************************\n", event.Name, pathName)

	currentValue, exists := handler.contents[pathName]

	switch event.Name {
	case "notify.Create":
		fmt.Printf("processEvent: About to assign from one path to the next. \n\tOriginal: %v \n\tEvent: %v\n", currentValue, event)
		// make sure there is an entry in the DirTreeMap for this folder. Since an empty list will always be returned, we can use that
		if !exists {
			info, err := os.Stat(fullPath)
			if err != nil {
				fmt.Printf("Could not get stats on directory %s\n", fullPath)
				if lock {
					handler.fsLock.Unlock()
				}
				//todo send error event up
				return
			}
			directory := NewDirectory()
			directory.FileInfo = info
			handler.contents[pathName] = *directory
		}

		updatedValue, exists := handler.contents[pathName]

		if lock {
			handler.fsLock.Unlock()
		}
		fmt.Println("FilesystemTracker:/+processEvent")

		if handler.watcher != nil {
			if event.IsDirectory {
				(*handler.watcher).FolderCreated(pathName)
			} else {
				(*handler.watcher).FileCreated(pathName)
			}
		}

		fmt.Printf("notify.Create: Updated value for %s: %v (%t)\n", pathName, updatedValue, exists)

		// sendEvent to manager
		SendEvent(event, fullPath)
		return

	case "notify.Remove":
		_, exists := handler.contents[pathName]

		delete(handler.contents, pathName)

		if handler.watcher != nil && exists {
			(*handler.watcher).FolderDeleted(pathName)
		} else {
			fmt.Println("In the notify.Remove section but did not see a watcher")
		}

		if lock {
			handler.fsLock.Unlock()
			fmt.Println("FilesystemTracker:/+processEvent 1")
		}
		SendEvent(event, "")

		fmt.Printf("notify.Remove: %s (%t)\n", pathName, exists)
		return
	case "notify.Rename":
		fmt.Printf("Rename attempted %v\n", event)
		handler.handleRename(event, pathName, fullPath)
	case "notify.Write":
		fmt.Printf("File updated %v\n", event)
		if handler.watcher != nil {
			if event.IsDirectory {
				(*handler.watcher).FolderUpdated(pathName)
			} else {
				(*handler.watcher).FileUpdated(pathName)
			}
		}

		SendEvent(event, fullPath)

	default:
		// do not send the event if we do not recognize it
		fmt.Printf("%s: %s not known, skipping (%v)\n", event.Name, pathName, event)
	}

	if lock {
		handler.fsLock.Unlock()
		fmt.Println("FilesystemTracker:/+processEvent 2")
	}
}

// Scan for existing files and add them to the list of files that we have with create events. this has to be called inside of a lock
func (handler *FilesystemTracker) scanFolders() error {
	fmt.Println("FileSystemTracker ScanFolders - start")
	pendingPaths := make([]string, 0, 100)
	pendingPaths = append(pendingPaths, handler.directory)
	handler.contents = make(map[string]Entry)

	for len(pendingPaths) > 0 {
		currentPath := pendingPaths[0]
		pendingPaths = pendingPaths[1:]

		// Read the directories in the path
		f, err := os.Open(currentPath)
		if err != nil {
			return err
		}

		dirEntries, err := f.Readdir(-1)
		for _, entry := range dirEntries {
			// Determine the relative path for this file or folder
			absolutePath := filepath.Join(currentPath, entry.Name())
			relativePath := absolutePath[len(handler.directory)+1:]

			var hash []byte

			if entry.IsDir() {
				handler.stats.TotalFolders++
			} else {
				hash, err = fileBlake2bHash(absolutePath)
				if err != nil {
					panic(err)
				}
				handler.stats.TotalFiles++
			}

			// add to contents
			info, err := os.Stat(absolutePath)
			if err != nil {
				fmt.Printf("ScanFolders: Could not get stats on directory %s\n", absolutePath)
				panic(err)
			}

			directory := NewDirectory()
			directory.FileInfo = info
			directory.hash = hash
			handler.contents[relativePath] = *directory

			fmt.Printf("Packing up: %s = %#v\n", relativePath, directory)

			if entry.IsDir() {
				newDirectory := filepath.Join(currentPath, entry.Name())
				pendingPaths = append(pendingPaths, newDirectory)
			}
		}

		err = f.Close()
		if err != nil {
			return err
		}
	}

	fmt.Println("About to set status of joining cluster")
	handler.server.SetStatus(REPLICAT_STATUS_JOINING_CLUSTER)
	fmt.Println("Done set status of joining cluster. About to send catalog")
	handler.SendCatalog()
	fmt.Println("Done sending catalog")

	return nil
}

// EntryJSON
type EntryJSON struct {
	RelativePath string
	IsDirectory  bool
	Hash         []byte
	ModTime      time.Time
	Size         int64
	ServerName   string
}

// This needs to be called with handler.fsLock engaged
func (handler *FilesystemTracker) SendCatalog() {
	fmt.Printf("FileSystemTracker ScanFolders - end - Found %d items\n", len(handler.contents))

	handler.stats.CatalogsSent++
	// transfer the normal contents structure to the JSON friendly version
	//rawData := make([]EntryJSON, 0, len(handler.contents))
	rawData := make([]EntryJSON, 0, len(handler.contents))

	for k, v := range handler.contents {
		entry := EntryJSON{RelativePath: k, IsDirectory: v.IsDir(), Hash: v.hash, ModTime: v.ModTime(), Size: v.Size()}
		fmt.Printf("Packing up: %s=%#v\n", k, entry)
		rawData = append(rawData, entry)
	}

	jsonData, err := json.Marshal(rawData)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Analyzed all files in the folder and the total size is: %d\n", len(jsonData))

	event := Event{
		Name:          "replicat.Catalog",
		Source:        globalSettings.Name,
		Time:          time.Now(),
		NetworkSource: globalSettings.Name,
		RawData:       jsonData,
	}

	fmt.Printf("About to directly send out full catalog event with: %v\n", event)
	sendCatalogToManagerAndSiblings(event)
	fmt.Println("catalog sent")
}

// ProcessCatalog - handle a catalog passed from another replicat node
func (handler *FilesystemTracker) ProcessCatalog(event Event) {
	fmt.Printf("FilesystemTracker ProcessCatalog from Server: %s\n", event.Source)

	handler.stats.CatalogsReceived++
	remoteServer := event.Source
	// pull the directory tree from the payload
	remoteContents := make([]EntryJSON, 0)
	err := json.Unmarshal(event.RawData, &remoteContents)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Done unmarshalling event. len %d\n", len(remoteContents))

	// Let's go through the other side's files and see if any of them are more up to date than what we have.
	handler.fsLock.Lock()

	fmt.Printf("Data retrieved from: %s\n%#v\n", event.Source, remoteContents)

	if handler.neededFiles == nil {
		handler.neededFiles = make(map[string]EntryJSON)
	}

	for _, remoteEntry := range remoteContents {
		// Get the path out
		path := remoteEntry.RelativePath

		// check for a local value
		local, exists := handler.contents[path]

		// Request transfer of the file if we do not have a local copy already
		transfer := !exists

		fmt.Printf("Considering: %s\nexists: %v\nremote: %#v\nlocal:  %#v\n", path, exists, remoteEntry, local)
		// Make missing directories immediately so we have a place to put the files
		if !exists && remoteEntry.IsDirectory {
			fmt.Printf("ProcessCatalog(%s) %s\nIs a directory, creating now\n", remoteEntry.ServerName, path)
			// Make a missing directory
			handler.createPath(path, true)
			// Skip to the next entry
			continue
		}

		if !transfer {
			fmt.Printf("ProcessCatalog(%s) %s\remote: %v\nlocal:  %v\n", remoteEntry.ServerName, path, remoteEntry, local)
			fmt.Printf("Comparing times for (%s) remote: %v local: %v\n", path, remoteEntry.ModTime, local.ModTime())
			if local.hash == nil || local.ModTime().Before(remoteEntry.ModTime) {
				transfer = true
			}
		}

		hashSame := bytes.Equal(remoteEntry.Hash, local.hash)
		fmt.Printf("Done considering(%s) transfer is: %s\n", path, transfer)
		//todo should we do something if the transfer is set to true yet the hash is the same?

		// If the hashes differ, we need to do something -- unless the other side is the older one
		//if !transfer && !hashSame {
		//	panic(fmt.Sprintf("We have a problem. file: %s\nremoteHash: %s\nlocalHash:  %s\n", path, remoteEntry.Hash, local.hash))
		//}

		if transfer {
			currentEntry := handler.neededFiles[path]
			fmt.Printf("We have decided to request transfer of this file: %s\nCurrent: %#v\nRemote: %#v\n", path, currentEntry, remoteEntry)

			// If the current modification time is before the remote modification time, use the remote one
			useNew := currentEntry.ModTime.Before(remoteEntry.ModTime)
			fmt.Printf("Use New: %v HashSame: %v\n", useNew, hashSame)

			// If the hash and time are the same, randomly decide which one to use
			if !useNew && hashSame {
				useNew = rand.Intn(1) == 1
			}

			// If we are going to use the new file, update the information
			if useNew {
				fmt.Println("decided to use new")
				currentEntry.ModTime = remoteEntry.ModTime
				currentEntry.Hash = remoteEntry.Hash
				currentEntry.Size = remoteEntry.Size
				currentEntry.ServerName = remoteServer

				fmt.Printf("About to save %s \n%#v to neededFiles \n%#v\n", path, currentEntry, handler.neededFiles)
				handler.neededFiles[path] = currentEntry
			} else {
				fmt.Println("decided to use old")

			}
		}
	}

	if len(handler.neededFiles) > 0 {
		handler.server.SetStatus(REPLICAT_STATUS_JOINING_CLUSTER)
		handler.requestNeededFiles()
	} else {
		handler.server.SetStatus(REPLICAT_STATUS_ONLINE)
	}

	handler.fsLock.Unlock()
}

// send out the actual requests for needed files when necessary. Call when inside of a lock!
func (handler *FilesystemTracker) requestNeededFiles() {
	// Collect the files needed for each server.
	fmt.Printf("start collecting what we need from each server %#v\n", handler.neededFiles)
	filesToFetch := make(map[string]map[string]EntryJSON)

	for path, entry := range handler.neededFiles {
		fmt.Printf("%s: %s (%#v)\n", entry.ServerName, path, entry)
		server := entry.ServerName
		fileMap := filesToFetch[server]
		if fileMap == nil {
			fileMap = make(map[string]EntryJSON)
		}
		fileMap[path] = entry
		filesToFetch[server] = fileMap
	}

	for server, fileMap := range filesToFetch {
		fmt.Printf("Files needed from: %s\n", server)
		for filename, entry := range fileMap {
			fmt.Printf("\t%s(%d) - %v\n", filename, entry.Size, entry.Hash)
		}
		handler.SendRequestForFiles(server, fileMap)
	}
	fmt.Println("done collecting what we need from each server")

}

// This needs to be called with handler.fsLock engaged
func (handler *FilesystemTracker) SendRequestForFiles(server string, fileMap map[string]EntryJSON) {
	fmt.Printf("FileSystemTracker SendRequestForFiles - end - Found %d items\n", len(handler.contents))

	jsonData, err := json.Marshal(fileMap)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Files needed from %s: %d    Data: %d\n", server, len(fileMap), len(jsonData))

	event := Event{
		Name:          "replicat.FileRequest",
		Source:        globalSettings.Name,
		Time:          time.Now(),
		NetworkSource: globalSettings.Name,
		RawData:       jsonData,
	}

	fmt.Printf("About to directly send out request for files from %s: %v\n", server, event)
	sendFileRequestToServer(server, event)
	fmt.Println("request sent")
}
