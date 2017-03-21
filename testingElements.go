// Package replicat is a server for n way synchronization of content (Replication for the cloud).
// Copyright 2016 Jacob Taylor jacob@replic.at       More Info: http://replic.at
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as
// published by the Free Software Foundation, either version 3 of the
// License, or (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.
package main

import (
	"fmt"
	"sync"
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

// GetFolderStats - get folder statistics
func (handler *countingChangeHandler) GetFolderStats() (created, deleted, updated int) {
	handler.fsLock.Lock()
	defer handler.fsLock.Unlock()

	return handler.FoldersCreated, handler.FoldersDeleted, handler.FoldersUpdated
}

// GetFileStats - get file statistics
func (handler *countingChangeHandler) GetFileStats() (created, deleted, updated int) {
	handler.fsLock.Lock()
	defer handler.fsLock.Unlock()

	return handler.FilesCreated, handler.FilesDeleted, handler.FilesUpdated
}

// FolderCreated - folder is created
func (handler *countingChangeHandler) FolderCreated(name string) error {
	handler.fsLock.Lock()
	defer handler.fsLock.Unlock()

	handler.FoldersCreated++

	fmt.Printf("countingChangeHandler:FolderCreated: %s (%d)\n", name, handler.FoldersCreated)
	return nil
}

// FolderDeleted - folder was deleted
func (handler *countingChangeHandler) FolderDeleted(name string) error {
	handler.fsLock.Lock()
	defer handler.fsLock.Unlock()

	handler.FoldersDeleted++

	fmt.Printf("countingChangeHandler:FolderDeleted: %s (%d)\n", name, handler.FoldersDeleted)
	return nil
}

// FolderUpdated - folder was updated
func (handler *countingChangeHandler) FolderUpdated(name string) error {
	handler.fsLock.Lock()
	defer handler.fsLock.Unlock()

	handler.FoldersUpdated++

	fmt.Printf("countingChangeHandler:FolderUpdated: %s (%d)\n", name, handler.FoldersUpdated)
	return nil
}

// FileCreated -- a file was created
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

