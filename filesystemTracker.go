// Copyright 2016 Jacob Taylor jacob@ablox.io
// License: Apache2 - http://www.apache.org/licenses/LICENSE-2.0
package main

import "fmt"

type ChangeHandler interface {
	FolderCreated(name string) (err error)
	FolderDeleted(name string) (err error)
}

type StorageTracker interface {
	CreateFolder(name string) (err error)
	DeleteFolder(name string) (err error)
	ListFolders() (folders []string, err error)
}

type FilesystemTracker struct {
	directory string
	contents  DirTreeMap
	setup     bool
	watcher   *ChangeHandler
}

func (self *FilesystemTracker) init() {
	if !self.setup {
		self.contents = make(DirTreeMap)
		self.setup = true
	}
}

func (self *FilesystemTracker) watchDirectory(directory string, watcher *ChangeHandler) {
	fmt.Printf("watchDirectory called with %s\n", directory)
	if self.directory == directory {
		return
	}
	self.directory = directory
	self.watcher = watcher
}

func (self *FilesystemTracker) CreateFolder(name string) (err error) {
	self.init()

	_, exists := self.contents[name]
	fmt.Printf("CreateFolder: '%s' (%v)\n", name, exists)

	if !exists {
		self.contents[name] = make([]string, 0, 10)
	}

	return nil
}

func (self *FilesystemTracker) DeleteFolder(name string) (err error) {
	self.init()

	fmt.Printf("DeleteFolder: '%s'\n", name)
	delete(self.contents, name)

	return nil
}

func (self *FilesystemTracker) ListFolders() (list []string) {
	self.init()

	fmt.Println("ListFolders")

	folderList := make([]string, len(self.contents))
	index := 0

	for k, _ := range self.contents {
		folderList[index] = k
		index++
	}

	return folderList
}
