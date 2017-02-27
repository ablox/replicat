// Package replicat is a server for n way synchronization of content (rsync for the cloud).
// More information at: http://replic.at
// Copyright 2016 Jacob Taylor jacob@ablox.io
// License: Apache2 - http://www.apache.org/licenses/LICENSE-2.0
package replicat

import (
	"fmt"
	"sync"
	"sort"
)

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

// FolderUpdated - track a folder being updated
func (handler *LogOnlyChangeHandler) FolderUpdated(name string) error {
	fmt.Printf("LogOnlyChangeHandler:FolderUpdated: %s\n", name)
	return nil
}

// FileCreated - track a new folder being created
func (handler *LogOnlyChangeHandler) FileCreated(name string) error {
	fmt.Printf("LogOnlyChangeHandler:FileCreated: %s\n", name)
	return nil
}

// FileDeleted - track a new folder being deleted
func (handler *LogOnlyChangeHandler) FileDeleted(name string) error {
	fmt.Printf("LogOnlyChangeHandler:FileDeleted: %s\n", name)
	return nil
}

// FileUpdated - track a folder being updated
func (handler *LogOnlyChangeHandler) FileUpdated(name string) error {
	fmt.Printf("LogOnlyChangeHandler:FileUpdated: %s\n", name)
	return nil
}

// countingChangeHandler - counds the folders created and deleted. Used for testing.
type countingChangeHandler struct {
	FoldersCreated int
	FoldersDeleted int
	FoldersUpdated int
	FilesCreated   int
	FilesUpdated   int
	FilesDeleted   int
	fsLock         sync.RWMutex
}

func (handler *countingChangeHandler) GetFolderStats() (created, deleted, updated int) {
	handler.fsLock.Lock()
	defer handler.fsLock.Unlock()

	return handler.FoldersCreated, handler.FoldersDeleted, handler.FoldersUpdated
}

func (handler *countingChangeHandler) GetFileStats() (created, deleted, updated int) {
	handler.fsLock.Lock()
	defer handler.fsLock.Unlock()

	return handler.FilesCreated, handler.FilesDeleted, handler.FilesUpdated
}

func (handler *countingChangeHandler) FolderCreated(name string) error {
	handler.fsLock.Lock()
	defer handler.fsLock.Unlock()

	handler.FoldersCreated++

	fmt.Printf("countingChangeHandler:FolderCreated: %s (%d)\n", name, handler.FoldersCreated)
	return nil
}

func (handler *countingChangeHandler) FolderDeleted(name string) error {
	handler.fsLock.Lock()
	defer handler.fsLock.Unlock()

	handler.FoldersDeleted++

	fmt.Printf("countingChangeHandler:FolderDeleted: %s (%d)\n", name, handler.FoldersDeleted)
	return nil
}

func (handler *countingChangeHandler) FolderUpdated(name string) error {
	handler.fsLock.Lock()
	defer handler.fsLock.Unlock()

	handler.FoldersUpdated++

	fmt.Printf("countingChangeHandler:FolderUpdated: %s (%d)\n", name, handler.FoldersUpdated)
	return nil
}

func (handler *countingChangeHandler) FileCreated(name string) error {
	handler.fsLock.Lock()
	defer handler.fsLock.Unlock()

	handler.FilesCreated++

	fmt.Printf("countingChangeHandler:FileCreated: %s (%d)\n", name, handler.FilesCreated)
	return nil
}

func (handler *countingChangeHandler) FileDeleted(name string) error {
	handler.fsLock.Lock()
	defer handler.fsLock.Unlock()

	handler.FilesDeleted++

	fmt.Printf("countingChangeHandler:FileDeleted: %s (%d)\n", name, handler.FilesDeleted)
	return nil
}

func (handler *countingChangeHandler) FileUpdated(name string) error {
	handler.fsLock.Lock()
	defer handler.fsLock.Unlock()

	handler.FilesUpdated++

	fmt.Printf("countingChangeHandler:FileUpdated: %s (%d)\n", name, handler.FilesUpdated)
	return nil
}

// MemoryTracker - Track a pool of memory for testing - i.e. memory only object store
type MemoryTracker struct {
	fsLock    sync.RWMutex
	contents  map[string]Entry  // the typical stat object information
	fullData  map[string][]byte // the actual data of the object
	directory string
	setup     bool
}

func (tracker *MemoryTracker) CreatePath(relativePath string, isDirectory bool) (err error) {
	return nil
}

func (tracker *MemoryTracker) Rename(sourcePath string, destinationPath string, isDirectory bool) (err error) {
	return nil
}

func (tracker *MemoryTracker) DeleteFolder(name string) (err error) {
	return nil
}

func (tracker *MemoryTracker) ListFolders() (folderList []string) {
	return nil
}

func (tracker *MemoryTracker) printLockable(lock bool) {
	if lock {
		fmt.Println("FilesystemTracker:print")
		tracker.fsLock.RLock()
		fmt.Println("FilesystemTracker:/print")
		defer tracker.fsLock.RUnlock()
		defer fmt.Println("FilesystemTracker://print")
	}

	folders := make([]string, 0, len(tracker.contents))
	for dir := range tracker.contents {
		folders = append(folders, dir)
	}
	sort.Strings(folders)

	fmt.Println("~~~~~~~~~~~~~~~~~~~~~~~")
	fmt.Printf("~~~~%s MemoryTracker report setup(%v)\n", tracker.directory, tracker.setup)
	fmt.Printf("~~~~contents: %v\n", tracker.contents)
	fmt.Printf("~~~~folders: %v\n", folders)
	fmt.Println("~~~~~~~~~~~~~~~~~~~~~~~")
}
