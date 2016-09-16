// Copyright 2016 Jacob Taylor jacob@ablox.io
// License: Apache2 - http://www.apache.org/licenses/LICENSE-2.0
package main

import (
	"os"
	"path/filepath"
	"sort"
)

type DirTreeMap map[string][]string

// create a clone of the treemap
func (orig DirTreeMap) clone() (clone DirTreeMap) {
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

//type DirTreeMap map[string][]os.FileInfo

/*
Check for changes between two DirTreeMaps. If the newState map is Nil, it will rescan the folders to update to the state of the filesystem.
If it is not nil, it will not be updated. The updated state will be either the state of the filesystem or the value passed in for newState
*/
func checkForChanges(originalState, newState DirTreeMap) (changed bool, updatedState DirTreeMap, newPaths, deletedPaths, matchingPaths []string) {
	var err error

	if newState == nil {
		updatedState, err = createListOfFolders()
		if err != nil {
			panic(err)
		}
	} else {
		updatedState = newState.clone()
	}

	// Get a list of paths and compare them
	originalPaths := make([]string, 0, len(originalState))
	updatedPaths := make([]string, 0, len(updatedState))

	//todo this should already be sorted. This is very repetitive.
	for key := range originalState {
		originalPaths = append(originalPaths, key)
	}
	sort.Strings(originalPaths)

	for key := range updatedState {
		updatedPaths = append(updatedPaths, key)
	}
	sort.Strings(updatedPaths)
	//todo we should leverage the updated paths to obliviate the need to resort.

	// We now have two sorted lists of strings. Go through the original ones and compare the files
	var originalPosition, updatedPosition int

	deletedPaths = make([]string, 0, 100)
	newPaths = make([]string, 0, 100)
	matchingPaths = make([]string, 0, len(originalPaths))

	//pp := func(name string, stringList []string) {
	//	fmt.Println("***************************")
	//	fmt.Println(name)
	//	fmt.Println("***************************")
	//	for index, value := range stringList {
	//		fmt.Printf("[%3d]: %s\n", index, value)
	//	}
	//	fmt.Println("***************************")
	//}
	//pp("original paths", originalPaths)
	//pp("updated Paths", updatedPaths)

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

func createListOfFolders() (DirTreeMap, error) {
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
				entry.Mode()
				newDirectory := filepath.Join(currentPath, entry.Name())
				//newDirectory := filepath.Join(currentPath, entry)
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
		listOfFileInfo[currentPath] = fileList
	}

	//fmt.Printf("Export:\n")
	//for dir, _ := range listOfFileInfo {
	//	fmt.Printf("%s\n", dir)
	//}
	//fmt.Printf("Export done:\n")

	return listOfFileInfo, nil
}
