// Copyright 2016 Jacob Taylor jacob@ablox.io
// License: Apache2 - http://www.apache.org/licenses/LICENSE-2.0
package main

import (
	"fmt"
	"github.com/rjeczalik/notify"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"syscall"
	"time"
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
func (handler *LogOnlyChangeHandler) FolderCreated(name string) error {
	fmt.Printf("LogOnlyChangeHandler:FolderCreated: %s\n", name)
	return nil
}

// FolderDeleted - track a new folder being deleted
func (handler *LogOnlyChangeHandler) FolderDeleted(name string) error {
	fmt.Printf("LogOnlyChangeHandler:FolderDeleted: %s\n", name)
	return nil
}

// countingChangeHandler - counds the folders created and deleted. Used for testing.
type countingChangeHandler struct {
	FoldersCreated int
	FoldersDeleted int
}

func (handler *countingChangeHandler) FolderCreated(name string) error {
	handler.FoldersCreated++

	fmt.Printf("countingChangeHandler:FolderCreated: %s (%d)\n", name, handler.FoldersCreated)
	return nil
}

func (handler *countingChangeHandler) FolderDeleted(name string) error {
	handler.FoldersDeleted++

	fmt.Printf("countingChangeHandler:FolderDeleted: %s (%d)\n", name, handler.FoldersDeleted)
	return nil
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
}

// Entry - contains the data for a file
type Entry struct {
	os.FileInfo
	setup bool
	hash  string
}

// NewDirectory - creates and returns a new Directory
func NewDirectory() *Entry {
	return &Entry{setup: true}
}

// NewDirectoryFromFileInfo - creates and returns a new Directory based on a fileinfo structure
func NewDirectoryFromFileInfo(info *os.FileInfo) *Entry {
	return &Entry{*info, true, ""}
}

func (handler *FilesystemTracker) printTracker() {
	handler.printLockable(false)
}

func (handler *FilesystemTracker) printLockable(lock bool) {
	if lock {
		fmt.Println("FilesystemTracker:print")
		handler.fsLock.RLock()
		fmt.Println("FilesystemTracker:/print")
	}

	fmt.Println("~~~~~~~~~~~~~~~~~~~~~~~")
	fmt.Printf("~~~~%s Tracker report setup(%v)\n", handler.directory, handler.setup)
	fmt.Printf("~~~~contents: %v\n", handler.contents)
	fmt.Printf("~~~~renames in progress: %v\n", handler.renamesInProgress)
	fmt.Println("~~~~~~~~~~~~~~~~~~~~~~~")

	if lock {
		handler.fsLock.RUnlock()
		fmt.Println("FilesystemTracker://print")
	}
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

func (handler *FilesystemTracker) init(directory string) {
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

	fmt.Println("Setting up filesystemTracker!")
	err := handler.scanFolders()
	if err != nil {
		panic(err)
	}

	handler.renamesInProgress = make(map[uint64]renameInformation, 0)

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
	fmt.Println("FilesystemTracker:CreatePath")
	handler.fsLock.Lock()
	defer handler.fsLock.Unlock()
	fmt.Println("FilesystemTracker:/CreatePath")
	defer fmt.Println("FilesystemTracker://CreatePath")

	if !handler.setup {
		panic("FilesystemTracker:CreatePath called when not yet setup")
	}

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
		directory := Entry{}
		directory.FileInfo, err = os.Stat(absolutePath)
		handler.contents[relativePath] = directory
	} else {
		fmt.Printf("for some reason the directory object already exists in the map: %s\n", relativePath)
	}

	return nil
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

	fmt.Printf("%d before delete of: %s\n", len(handler.contents), name)
	fmt.Printf("DeleteFolder: '%s'\n", name)
	delete(handler.contents, name)
	fmt.Printf("%d after delete of: %s\n", len(handler.contents), name)

	return nil
}

// ListFolders - This storage handler should return a list of contained folders.
func (handler *FilesystemTracker) ListFolders() (folderList []string) {
	fmt.Println("FilesystemTracker:ListFolders")
	handler.fsLock.Lock()
	defer handler.fsLock.Unlock()
	fmt.Println("FilesystemTracker:/ListFolders")
	defer fmt.Println("FilesystemTracker://ListFolders")

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
			path = "."
		} else {
			// update the path to not have this prefix
			path = fullPath[directoryLength+1:]
		}
	}

	return
}

// Monitor the filesystem looking for changes to files we are keeping track of.
func (handler *FilesystemTracker) monitorLoop(c chan notify.EventInfo) {
	for {
		ei := <-c

		fmt.Printf("*****We have an event: %v\nwith Sys: %v\npath: %v\nevent: %v\n", ei, ei.Sys(), ei.Path(), ei.Event())

		path, fullPath := extractPaths(handler, &ei)
		event := Event{Name: ei.Event().String(), Message: path}
		log.Printf("Event captured name: %s location: %s, ei.Path(): %s", event.Name, event.Message, ei.Path())

		isDirectory := handler.checkIfDirectory(event, path, fullPath)
		event.IsDirectory = isDirectory
		handler.processEvent(event, path, fullPath)
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
	TRACKER_RENAME_TIMEOUT = time.Millisecond * 25
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

func trackerTestEmptyDirectoryMovesInOutAround() {
	monitoredFolder, _ := ioutil.TempDir("", "monitored")
	outsideFolder, _ := ioutil.TempDir("", "outside")
	defer os.RemoveAll(monitoredFolder)
	defer os.RemoveAll(outsideFolder)

	tracker := new(FilesystemTracker)
	tracker.init(monitoredFolder)
	defer tracker.cleanup()

	logger := &LogOnlyChangeHandler{}
	var loggerInterface ChangeHandler = logger
	tracker.watchDirectory(&loggerInterface)

	tracker.printTracker()
	folderName := "happy"
	originalFolderName := folderName
	targetMonitoredPath := filepath.Join(monitoredFolder, folderName)
	targetOutsidePath := filepath.Join(outsideFolder, folderName)

	fmt.Printf("making folder: %s going to rename it to: %s\n", targetOutsidePath, targetMonitoredPath)
	os.Mkdir(targetOutsidePath, os.ModeDir+os.ModePerm)
	fmt.Printf("About to move file \nfrom: %s\n  to: %s\n", targetOutsidePath, targetMonitoredPath)
	os.Rename(targetOutsidePath, targetMonitoredPath)
	stats, _ := os.Stat(targetMonitoredPath)
	fmt.Printf("stats for: %s\n%v\n", targetMonitoredPath, stats)

	helper := func(tracker *FilesystemTracker, folder string) bool {
		_, exists := tracker.contents[folder]
		fmt.Printf("Checking the value of exists: %v\n", exists)
		return exists
	}

	if !WaitFor(tracker, folderName, true, helper) {
		panic(fmt.Sprintf("%s not found in contents\ncontents: %v\n", folderName, tracker.contents))
	}

	tracker.printTracker()
	if len(tracker.renamesInProgress) > 0 {
		panic(fmt.Sprint("6 tracker has renames in progress still"))
	}

	// check to make sure that there are no invalid directories
	tracker.validate()

	folderName = folderName + "b"
	moveSourcePath := targetMonitoredPath
	moveDestinationPath := monitoredFolder + "/" + folderName
	fmt.Printf("About to move file \nfrom: %s\n  to: %s\n", moveSourcePath, moveDestinationPath)
	os.Rename(moveSourcePath, moveDestinationPath)

	if !WaitFor(tracker, originalFolderName, false, helper) {
		panic(fmt.Sprintf("Still finding originalFolderName %s after rename timeout \ncontents: %v\n", originalFolderName, tracker.contents))
	}

	if !WaitFor(tracker, folderName, true, helper) {
		panic(fmt.Sprintf("%s not found after renamte timout\ncontents: %v\n", folderName, tracker.contents))
	}
	tracker.printTracker()
	if len(tracker.renamesInProgress) > 0 {
		panic(fmt.Sprint("11 tracker has renames in progress still"))
	}

	// check to make sure that there are no invalid directories
	tracker.validate()

	moveSourcePath = moveDestinationPath
	moveDestinationPath = targetOutsidePath

	fmt.Printf("About to move file \nfrom: %s\n  to: %s\n", moveSourcePath, moveDestinationPath)
	os.Rename(moveSourcePath, moveDestinationPath)

	if !WaitFor(tracker, folderName, false, helper) {
		fmt.Printf("Tracker contents: %v\n", tracker.contents)
		panic(fmt.Sprintf("%s not cleared from contents\ncontents: %v\n", folderName, tracker.contents))
	}
	tracker.printTracker()
}

func trackerTestFileChangeTrackerAddFolders() {
	logHandler := countingChangeHandler{}
	var c ChangeHandler = &logHandler

	tmpFolder, err := ioutil.TempDir("", "blank")
	defer os.RemoveAll(tmpFolder)

	fmt.Println("TestFileChangeTrackerAddFolders: About to call watchDirectory")
	tracker := new(FilesystemTracker)
	tracker.init(tmpFolder)
	defer tracker.cleanup()

	tracker.watchDirectory(&c)
	fmt.Println("TestFileChangeTrackerAddFolders: Done - About to call watchDirectory")

	numberOfSubFolders := 10

	for i := 0; i < numberOfSubFolders; i++ {
		path := fmt.Sprintf("%s/a%d", tmpFolder, i)

		err = os.Mkdir(path, os.ModeDir+os.ModePerm)
		if err != nil {
			panic(err)
		}
	}
	fmt.Printf("done with creating %d different subfolders. :)\n", numberOfSubFolders)

	// todo - convert to waitfor
	cycleCount := 0
	for {
		cycleCount++
		folderList := tracker.ListFolders()
		if len(folderList) < numberOfSubFolders {
			//fmt.Printf("did not get all folders. Current: %v\n", folderList)
			if cycleCount > 20 {
				panic(fmt.Sprintf("Did not find enough folders. Got bored of waiting. What was found: %v\n", folderList))
			}
			time.Sleep(time.Millisecond * 50)
		} else {
			fmt.Println("We have all of our ducks in a row")
			break
		}
	}

	folder1 := fmt.Sprintf("%s/a0", tmpFolder)
	folder2 := fmt.Sprintf("%s/a1", tmpFolder)
	fmt.Printf("about to delete two folders \n%s\n%s\n", folder1, folder2)
	// Delete two folders
	fmt.Println(tracker.ListFolders())
	os.Remove(folder1)
	os.Remove(folder2)
	fmt.Printf("deleted two folders \n%s\n%s\n", folder1, folder2)

	expectedCreated := numberOfSubFolders + 1
	expectedDeleted := 2

	tracker.printTracker()

	// wait for the final tally to come through.
	cycleCount = 0
	for {
		cycleCount++

		if logHandler.FoldersCreated != expectedCreated || logHandler.FoldersDeleted != expectedDeleted {
			if cycleCount > 20 {
				tracker.printTracker()
				panic(fmt.Sprintf("Kept finding bad data. Got bored of waiting. What was found: %v\n", tracker.ListFolders()))
				break
			}
			time.Sleep(time.Millisecond * 50)
		} else {
			fmt.Println("We have all of our ducks in a row again. Yay!")
			break
		}
	}

	if logHandler.FoldersCreated != expectedCreated || logHandler.FoldersDeleted != expectedDeleted {
		panic(fmt.Sprintf("Expected/Found created: (%d/%d) deleted: (%d/%d)\n", expectedCreated, logHandler.FoldersCreated, expectedDeleted, logHandler.FoldersDeleted))
	}

	rootDirectory, exists := tracker.contents["."]
	if !exists {
		fmt.Println("root directory fileinfo is nil")
	} else {
		fmt.Printf("Root directory %v\n", rootDirectory)
		fmt.Printf("Root directory named: %s and has size %d\n", rootDirectory.Name(), rootDirectory.Size())
	}
}

func trackerTestSmallFileCreationAndRename() {
	monitoredFolder, _ := ioutil.TempDir("", "monitored")
	outsideFolder, _ := ioutil.TempDir("", "outside")
	defer os.RemoveAll(monitoredFolder)
	defer os.RemoveAll(outsideFolder)

	tracker := new(FilesystemTracker)
	tracker.init(monitoredFolder)
	defer tracker.cleanup()

	logger := &LogOnlyChangeHandler{}
	var loggerInterface ChangeHandler = logger
	tracker.watchDirectory(&loggerInterface)

	tracker.printTracker()

	fileName := "happy.txt"
	secondFilename := "behappy.txt"
	targetMonitoredPath := filepath.Join(monitoredFolder, fileName)
	secondMonitoredPath := filepath.Join(monitoredFolder, secondFilename)

	fmt.Printf("making file: %s\n", targetMonitoredPath)
	file, err := os.Create(targetMonitoredPath)
	if err != nil {
		panic(err)
	}

	sampleFileContents := "This is the content of the file\n"
	n, err := file.WriteString(sampleFileContents)
	if err != nil {
		panic(err)
	}
	if n != len(sampleFileContents) {
		panic(fmt.Sprintf("Contents of file not correct length n: %d len: %d\n", n, len(sampleFileContents)))
	}

	err = file.Close()
	if err != nil {
		panic(err)
	}

	fmt.Println("YOLO.....Not enough here!")

	helper := func(tracker *FilesystemTracker, path string) bool {
		entry, exists := tracker.contents[path]
		if !exists {
			return false
		}

		fmt.Printf("entry is: %#v\npath: %s\n", entry, path)

		return true
	}

	if !WaitFor(tracker, fileName, true, helper) {
		panic(fmt.Sprintf("%s not found in contents\ncontents: %v\n", fileName, tracker.contents))
	}

	tracker.printTracker()

	fmt.Printf("Moving file \nfrom: %s\n  to: %s\n", targetMonitoredPath, secondMonitoredPath)
	os.Rename(targetMonitoredPath, secondMonitoredPath)

	stats, err := os.Stat(secondMonitoredPath)
	if err != nil {
		panic(fmt.Sprintf("failed to move file: %v", err))
	}
	fmt.Printf("stats for: %s\n%v\n", targetMonitoredPath, stats)

	if !WaitFor(tracker, fileName, false, helper) {
		panic(fmt.Sprintf("%s found in contents\ncontents: %v\n", fileName, tracker.contents))
	}

	if !WaitFor(tracker, secondFilename, true, helper) {
		panic(fmt.Sprintf("%s not found in contents\ncontents: %v\n", secondFilename, tracker.contents))
	}

	if len(tracker.renamesInProgress) > 0 {
		panic(fmt.Sprint("6 tracker has renames in progress still"))
	}

	// check to make sure that there are no invalid directories
	tracker.validate()
}

func trackerTestSmallFileMovesInOutAround() {
	monitoredFolder, _ := ioutil.TempDir("", "monitored")
	outsideFolder, _ := ioutil.TempDir("", "outside")
	defer os.RemoveAll(monitoredFolder)
	defer os.RemoveAll(outsideFolder)

	tracker := new(FilesystemTracker)
	tracker.init(monitoredFolder)
	defer tracker.cleanup()

	logger := &LogOnlyChangeHandler{}
	var loggerInterface ChangeHandler = logger
	tracker.watchDirectory(&loggerInterface)

	tracker.printTracker()

	fileName := "happy"
	//originalFolderName := fileName
	targetMonitoredPath := filepath.Join(monitoredFolder, fileName)
	targetOutsidePath := filepath.Join(outsideFolder, fileName)

	fmt.Printf("making file: %s\n", targetOutsidePath)
	file, err := os.Create(fileName)
	if err != nil {
		panic(err)
	}

	sampleFileContents := "This is the content of the file\n"
	n, err := file.WriteString(sampleFileContents)
	if err != nil {
		panic(err)
	}
	if n != len(sampleFileContents) {
		panic(fmt.Sprintf("Contents of file not correct length n: %d len: %d\n", n, len(sampleFileContents)))
	}

	err = file.Close()
	if err != nil {
		panic(err)
	}

	fmt.Printf("making file: %s going to rename it to: %s\n", targetOutsidePath, targetMonitoredPath)
	os.Rename(targetOutsidePath, targetMonitoredPath)

	stats, _ := os.Stat(targetMonitoredPath)
	fmt.Printf("stats for: %s\n%v\n", targetMonitoredPath, stats)

	fmt.Println("Hit the end of this test.....needs more! Files are currently being treated as directories.....stop stat....\n\n\n\n\n\n\ngaaak")

	//helper := func(tracker *FilesystemTracker, folder string) bool {
	//	_, exists := tracker.contents[folder]
	//	fmt.Printf("Checking the value of exists: %v\n", exists)
	//	return exists
	//}
	//
	//if !WaitFor(tracker, fileName, true, helper) {
	//	panic(fmt.Sprintf("%s not found in contents\ncontents: %v\n", fileName, tracker.contents))
	//}
	//
	//tracker.printTracker()
	//if len(tracker.renamesInProgress) > 0 {
	//	panic(fmt.Sprint("6 tracker has renames in progress still"))
	//}
	//
	//// check to make sure that there are no invalid directories
	//tracker.validate()
	//
	//fileName = fileName + "b"
	//moveSourcePath := targetMonitoredPath
	//moveDestinationPath := monitoredFolder + "/" + fileName
	//fmt.Printf("About to move file \nfrom: %s\n  to: %s\n", moveSourcePath, moveDestinationPath)
	//os.Rename(moveSourcePath, moveDestinationPath)
	//
	//if !WaitFor(tracker, originalFolderName, false, helper) {
	//	panic(fmt.Sprintf("Still finding originalFolderName %s after rename timeout \ncontents: %v\n", originalFolderName, tracker.contents))
	//}
	//
	//if !WaitFor(tracker, fileName, true, helper) {
	//	panic(fmt.Sprintf("%s not found after renamte timout\ncontents: %v\n", fileName, tracker.contents))
	//}
	//tracker.printTracker()
	//if len(tracker.renamesInProgress) > 0 {
	//	panic(fmt.Sprint("11 tracker has renames in progress still"))
	//}
	//
	//// check to make sure that there are no invalid directories
	//tracker.validate()
	//
	//moveSourcePath = moveDestinationPath
	//moveDestinationPath = targetOutsidePath
	//
	//fmt.Printf("About to move file \nfrom: %s\n  to: %s\n", moveSourcePath, moveDestinationPath)
	//os.Rename(moveSourcePath, moveDestinationPath)
	//
	//if !WaitFor(tracker, fileName, false, helper) {
	//	fmt.Printf("Tracker contents: %v\n", tracker.contents)
	//	panic(fmt.Sprintf("%s not cleared from contents\ncontents: %v\n", fileName, tracker.contents))
	//}
	//tracker.printTracker()
}

// completeRenameIfAbandoned - if there is a rename that was started with a source
// but has been left pending for more than TRACKER_RENAME_TIMEOUT, complete it (i.e. move the folder away)
func (handler *FilesystemTracker) completeRenameIfAbandoned(iNode uint64) {
	time.Sleep(TRACKER_RENAME_TIMEOUT)

	//todo add locking
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
	//inProgress, found := handler.renamesInProgress[iNode]
	//fmt.Printf("^^^^^^^retrieving under iNode: %d (found %v) saved transfer inProgress: %v\n", iNode, found, inProgress)

	//startedWithSourceSet := inProgress.sourceSet
	//fmt.Printf("^^^^^^^tmpSourceSet: %v (started: %v) tmpDestinationSet: %v\n", tmpSourceSet, startedWithSourceSet, tmpDestinationSet)

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
	} else {
		//todo schedule this for destruction
		fmt.Printf("^^^^^^^We do not have both a source and destination - schedule and save under iNode: %d Current transfer is: %#v\n", iNode, inProgress)
		inProgress.iNode = iNode
		handler.renamesInProgress[iNode] = inProgress
		go handler.completeRenameIfAbandoned(iNode)
	}
}

func (handler *FilesystemTracker) processEvent(event Event, pathName, fullPath string) {
	fmt.Println("FilesystemTracker:processEvent")
	handler.fsLock.Lock()
	fmt.Println("FilesystemTracker:/processEvent")
	defer fmt.Println("FilesystemTracker://processEvent")

	fmt.Printf("*********************************************************\nhandleFilsystemEvent name: %s pathName: %s\n*********************************************************\n", event.Name, pathName)

	currentValue, exists := handler.contents[pathName]

	switch event.Name {
	case "notify.Create":
		fmt.Printf("processEvent: About to assign from one path to the next. Original: %v Map: %v\n", currentValue, handler.contents)
		// make sure there is an entry in the DirTreeMap for this folder. Since and empty list will always be returned, we can use that
		if !exists {
			info, err := os.Stat(fullPath)
			if err != nil {
				panic(fmt.Sprintf("Could not get stats on directory %s", fullPath))
			}
			directory := NewDirectory()
			directory.FileInfo = info
			handler.contents[pathName] = *directory
		}

		updatedValue, exists := handler.contents[pathName]

		handler.fsLock.Unlock()
		fmt.Println("FilesystemTracker:/+processEvent")

		if handler.watcher != nil {
			(*handler.watcher).FolderCreated(pathName)
		}
		fmt.Printf("notify.Create: Updated value for %s: %v (%t)\n", pathName, updatedValue, exists)

		// sendEvent to manager
		SendEvent(event, "")
		return

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

		handler.fsLock.Unlock()
		fmt.Println("FilesystemTracker:/+processEvent")
		SendEvent(event, "")

		fmt.Printf("notify.Remove: Updated value for %s: %v (%t)\n", pathName, updatedValue, exists)
		return
	case "notify.Rename":
		// todo fix this to handle the two rename events to be one event
		fmt.Printf("Rename attempted %v\n", event)
		handler.handleRename(event, pathName, fullPath)
	case "notify.Write":
		fmt.Printf("File updated %v\n", event)
		SendEvent(event, fullPath)
	default:
		// do not send the event if we do not recognize it
		//SendEvent(event)
		fmt.Printf("%s: %s not known, skipping (%v)\n", event.Name, pathName, event)
	}

	handler.fsLock.Unlock()
	fmt.Println("FilesystemTracker:/+processEvent")

	// sendEvent to manager
	//SendEvent(event)
}

// Scan the files and folders inside of the directory we are watching and add them to the contents. This function
// can only be called inside of a writelock on handler.fsLock
func (handler *FilesystemTracker) scanFolders() error {
	pendingPaths := make([]string, 0, 100)
	pendingPaths = append(pendingPaths, handler.directory)
	handler.contents = make(map[string]Entry)

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
			// Determine the relative path for this file or folder
			relativePath := filepath.Join(currentPath, entry.Name())
			relativePath = relativePath[len(handler.directory):]
			currentEntry := Entry{entry, true, ""}
			handler.contents[relativePath] = currentEntry

			//todo fix this This function
			if entry.IsDir() {
				newDirectory := filepath.Join(currentPath, entry.Name())
				pendingPaths = append(pendingPaths, newDirectory)
			} else {
				//directory.contents[entry.Name()] = entry
			}
			//panic("This function is not completed yet.")
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
