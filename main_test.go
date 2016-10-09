// Copyright 2016 Jacob Taylor jacob@ablox.io
// License: Apache2 - http://www.apache.org/licenses/LICENSE-2.0
package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"reflect"
	"testing"
)

func TestDirectoryScan(t *testing.T) {
	tmpFolder, err := ioutil.TempDir("", "blank")
	defer os.RemoveAll(tmpFolder)
	globalSettings.Directory = tmpFolder
	emptyState, err := scanDirectoryContents()
	if err != nil {
		t.Fail()
	}

	// Create 5 folders
	numberOfSubFolders := 5
	newFolders := make([]string, 0, numberOfSubFolders)
	for i := 0; i < numberOfSubFolders; i++ {
		path := fmt.Sprintf("%s/a%d", tmpFolder, i)
		newFolders = append(newFolders, path)
		err = os.Mkdir(path, os.ModeDir+os.ModePerm)
		if err != nil {
			t.Fatal(err)
		}
	}

	totalFolders := numberOfSubFolders + 1
	dirState, err := scanDirectoryContents()
	verifyClonedDirTree(t, dirState)
	if len(dirState) != totalFolders {
		t.Fatalf("Unexpected Number of items in state. Expected %d, found %d\n", totalFolders, len(dirState))
	}

	changed, updatedState, newPaths, deletedPaths, matchingPaths := checkForChanges(dirState, nil)
	if changed || (len(newPaths)+len(deletedPaths)+len(matchingPaths) != totalFolders) {
		t.Fatal(fmt.Sprintf("Comparison of folder trees failed. changed %v, len (new, deleted, matching, total) (%d, %d, %d, %d)\n%v\n%v\n%v", changed, len(newPaths), len(deletedPaths), len(matchingPaths), totalFolders, newPaths, deletedPaths, matchingPaths))
	}

	assertEqualsTwoDirTreeMap(t, dirState, updatedState)
	verifyClonedDirTree(t, updatedState)

	// add longer paths
	subDirs := []string{"a", "b", "c"}
	baseDir := tmpFolder + "/a0"
	addNestedSubDirs(t, baseDir, subDirs)
	totalFolders += len(subDirs)

	changed, updatedState, newPaths, deletedPaths, matchingPaths = checkForChanges(dirState, nil)
	if !changed || (len(newPaths)+len(deletedPaths)+len(matchingPaths) != totalFolders) {
		t.Fatal("comparison of current state with current state did not result in empty....ouch\n")
	}

	if len(newPaths) != len(subDirs) {
		t.Fatal(fmt.Sprintf("wrong number of new paths. expected %d, got %d....ouch\n", len(subDirs), len(newPaths)))
	}
	verifyClonedDirTree(t, updatedState)

	// get caught up and add more!
	dirState, err = scanDirectoryContents()
	verifyClonedDirTree(t, dirState)

	subDirs = []string{"1", "2", "3", "4", "5"}
	baseDir = tmpFolder + "/a1"
	addNestedSubDirs(t, baseDir, subDirs)
	totalFolders += len(subDirs)

	changed, updatedState, newPaths, deletedPaths, matchingPaths = checkForChanges(dirState, nil)
	if !changed || (len(newPaths)+len(deletedPaths)+len(matchingPaths) != totalFolders) {
		t.Fatal("comparison of changed state with current state does not add up....ouch\n")
	}
	if len(newPaths) != len(subDirs) {
		t.Fatal(fmt.Sprintf("wrong number of new paths. expected %d, got %d....ouch\n", len(subDirs), len(newPaths)))
	}
	verifyClonedDirTree(t, updatedState)

	// add a, b, c, d, e, ab, abc, abd
	tmpFolder, err = ioutil.TempDir("", "blank")
	totalFolders = 1
	globalSettings.Directory = tmpFolder
	defer os.RemoveAll(tmpFolder)
	subDirs = []string{"a", "b", "c", "d", "e", "ab", "abc", "abd"}
	addFlatSubDirs(t, tmpFolder, subDirs)
	totalFolders += len(subDirs)
	dirState, err = scanDirectoryContents()
	verifyClonedDirTree(t, dirState)

	changed, updatedState, newPaths, deletedPaths, matchingPaths = checkForChanges(dirState, nil)
	if changed || (len(newPaths)+len(deletedPaths)+len(matchingPaths) != totalFolders) {
		t.Fatal(fmt.Sprintf("comparison of current state with current state did not result in empty....ouch\nChanged %v\nnewPaths: %v\ndeletedPaths: %v\nmatchingPaths: %v\nlen of matchingPaths: %d, totalFolders: %d\n", changed, newPaths, deletedPaths, matchingPaths, len(matchingPaths), totalFolders))
	}
	verifyClonedDirTree(t, updatedState)

	// delete ab and make sure it is the only one deleted
	deletePath := tmpFolder + "/ab"
	os.Remove(deletePath)
	changed, updatedState, newPaths, deletedPaths, matchingPaths = checkForChanges(dirState, nil)
	if !changed || (len(newPaths)+len(deletedPaths)+len(matchingPaths) != totalFolders) {
		t.Fatal("comparison of changed state with current state does not add up....ouch\n")
	}
	if len(deletedPaths) != 1 {
		t.Fatal(fmt.Sprintf("wrong number of deleted paths. expected 1, got %d....ouch\n", len(deletedPaths)))
	}
	verifyClonedDirTree(t, updatedState)

	// get caught up and delete the start and end ones
	totalFolders -= len(deletedPaths)
	dirState, err = scanDirectoryContents()
	verifyClonedDirTree(t, dirState)
	deletePath = tmpFolder + "/a"
	os.Remove(deletePath)
	deletePath = tmpFolder + "/abd"
	os.Remove(deletePath)
	changed, updatedState, newPaths, deletedPaths, matchingPaths = checkForChanges(dirState, nil)
	if !changed || (len(newPaths)+len(deletedPaths)+len(matchingPaths) != totalFolders) {
		t.Fatal("comparison of changed state with current state does not add up....ouch\n")
	}
	if len(deletedPaths) != 2 {
		t.Fatal(fmt.Sprintf("wrong number of deleted paths. expected 2, got %d....ouch\n", len(deletedPaths)))
	}
	verifyClonedDirTree(t, updatedState)

	// get caught up and recreate them all
	totalFolders -= len(deletedPaths)
	dirState, err = scanDirectoryContents()
	if err != nil {
		panic(err)
	}
	verifyClonedDirTree(t, dirState)

	subDirs = []string{"a", "abd"}
	addFlatSubDirs(t, tmpFolder, subDirs)
	totalFolders += len(subDirs)
	changed, updatedState, newPaths, deletedPaths, matchingPaths = checkForChanges(dirState, nil)
	if !changed || (len(newPaths)+len(deletedPaths)+len(matchingPaths) != totalFolders) {
		t.Fatal("comparison of changed state with current state does not add up....ouch\n")
	}
	if len(newPaths) != 2 {
		t.Fatal(fmt.Sprintf("wrong number of new paths. expected 2, got %d....ouch\n", len(deletedPaths)))
	}
	verifyClonedDirTree(t, updatedState)

	_ = emptyState
}

func verifyClonedDirTree(t *testing.T, orig DirTreeMap) {
	dirState2 := orig.Clone()

	if reflect.DeepEqual(orig, dirState2) == false {
		t.Fatal(fmt.Sprintf("cloned directory tree did not match original (orig, cloned)\n%v\n%v\n", orig, dirState2))
	}
}

func assertEqualsTwoDirTreeMap(t *testing.T, first, second DirTreeMap) {
	if reflect.DeepEqual(first, second) == false {
		t.Fatal(fmt.Sprintf("two directory tree did not match (first, second)\n%v\n%v\n", first, second))
	}
}

func addNestedSubDirs(t *testing.T, baseDir string, subDirs []string) {
	path := baseDir
	for i := range subDirs {
		path += fmt.Sprintf("/%s", subDirs[i])
		//fmt.Println(path)
		err := os.Mkdir(path, os.ModeDir+os.ModePerm)
		if err != nil {
			t.Fatal(err)
		}
	}
}

func addFlatSubDirs(t *testing.T, baseDir string, subDirs []string) {
	for i := range subDirs {
		path := fmt.Sprintf("%s/%s", baseDir, subDirs[i])
		err := os.Mkdir(path, os.ModeDir+os.ModePerm)
		if err != nil {
			t.Fatal(err)
		}
	}

}
