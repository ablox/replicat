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
	"os"
	"path/filepath"
	"sort"
)

// DirTreeMap is a mapping between directories and lists of file names.
type DirTreeMap map[string][]string

// Clone a DirTreeMap
func (orig DirTreeMap) Clone() (clone DirTreeMap) {
	clone = make(DirTreeMap)
	for k, v := range orig {
		cloneV := make([]string, len(v))
		for i, str := range v {
			cloneV[i] = str
		}
		clone[k] = cloneV
	}

	return
}

// move is tracked by nodeid node id to string list of relative paths
// Create and delete

//type DirTreeMap map[string][]os.FileInfo

/*
Check for changes between two DirTreeMaps. If the newState map is Nil, it will rescan the folders to update to the state of the filesystem.
If it is not nil, it will not be updated. The updated state will be either the state of the filesystem or the value passed in for newState
*/
func checkForChanges(originalState, newState DirTreeMap) (changed bool, updatedState DirTreeMap, newPaths, deletedPaths, matchingPaths []string) {
	var err error

	// if no updated state is provided, resurvey the drive. If there is updated state, use it
	if newState == nil {
		updatedState, err = scanDirectoryContents()
		if err != nil {
			panic(err)
		}
	} else {
		updatedState = newState.Clone()
	}

	// Get a list of paths and compare them
	originalPaths := make([]string, len(originalState))
	updatedPaths := make([]string, len(updatedState))

	//todo this should already be sorted. This is very repetitive.
	index := 0
	for key := range originalState {
		originalPaths[index] = key
		index++
	}
	sort.Strings(originalPaths)

	index = 0
	for key := range updatedState {
		updatedPaths[index] = key
		index++
	}
	sort.Strings(updatedPaths)

	// We now have two sorted lists of strings. Go through the original ones and compare the files
	var originalPosition, updatedPosition int

	deletedPaths = make([]string, 0, 100)
	newPaths = make([]string, 0, 100)
	matchingPaths = make([]string, 0, len(originalPaths))

	for {
		//fmt.Printf("Original Position %3d    Updated Position %3d\n", originalPosition, updatedPosition)
		if originalPosition >= len(originalPaths) {
			// all remaining updated paths are new
			newPaths = append(newPaths, updatedPaths[updatedPosition:]...)
			//fmt.Println("Adding remaining paths")
			break
		} else if updatedPosition >= len(updatedPaths) {
			// all remaining original paths are new
			//fmt.Println("Deleting remaining paths")
			deletedPaths = append(deletedPaths, originalPaths[originalPosition:]...)
			break
		} else {
			oldPath := originalPaths[originalPosition]
			updPath := updatedPaths[updatedPosition]
			//fmt.Printf("comparing paths: '%s' and '%s'\n", oldPath, updPath)

			// Start with nothing changed. Base case
			if oldPath == updPath {
				//fmt.Println("match")
				matchingPaths = append(matchingPaths, updatedPaths[updatedPosition])
				updatedPosition++
				originalPosition++
			} else if oldPath > updPath {
				//fmt.Println("adding new path")
				newPaths = append(newPaths, updatedPaths[updatedPosition])
				updatedPosition++
			} else {
				//fmt.Println("Deleting old path")
				deletedPaths = append(deletedPaths, originalPaths[originalPosition])
				originalPosition++
			}
		}
	}

	//fmt.Printf("Path report: new %d, deleted %d, matching %d, original %d, updated %d\n", len(newPaths), len(deletedPaths), len(matchingPaths), len(originalPaths), len(updatedPaths))
	//fmt.Printf("New paths: %v\n", newPaths)
	//fmt.Printf("Deleted paths: %v\n", deletedPaths)

	if len(newPaths) > 0 || len(deletedPaths) > 0 {
		changed = true
	}

	return changed, updatedState, newPaths, deletedPaths, matchingPaths
}

func scanDirectoryContents() (DirTreeMap, error) {
	fmt.Println("scanning directory contents - start")
	pendingPaths := make([]string, 0, 100)
	pendingPaths = append(pendingPaths, globalSettings.Directory)
	listOfFileInfo := make(DirTreeMap)

	for len(pendingPaths) > 0 {
		currentPath := pendingPaths[0]
		// Strip off of the base path before adding it to the list of folders
		//paths = append(paths, currentPath[len(globalSettings.Directory)+1:])
		fileList := make([]string, 0, 100)
		//fileList := make([]os.FileInfo, 0, 100)
		pendingPaths = pendingPaths[1:]

		// Read the directories in the path
		f, err := os.Open(currentPath)
		if err != nil {
			return nil, err
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
			return nil, err
		}

		sort.Strings(fileList)

		// Strip the base path off of the current path
		// make sure all of the paths are still '/' prefixed
		relativePath := currentPath[len(globalSettings.Directory):]
		if relativePath == "" {
			relativePath = "/"
		}

		listOfFileInfo[relativePath] = fileList
	}

	fmt.Println("scanning directory contents - end")
	return listOfFileInfo, nil
}
