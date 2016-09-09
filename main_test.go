// Copyright 2016 Jacob Taylor jacob@ablox.io
// License: Apache2 - http://www.apache.org/licenses/LICENSE-2.0
package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"
)

func TestDirectoryScan(t *testing.T) {
	tmpFolder, err := ioutil.TempDir("", "blank")
	defer os.RemoveAll(tmpFolder)
	emptyState, err := createListOfFolders(tmpFolder)
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
	dirState, err := createListOfFolders(tmpFolder)
	if len(dirState) != totalFolders {
		t.Fatalf("Unexpected Number of items in state. Expected %d, found %d\n", totalFolders, len(dirState))
		t.Fail()
	}

	changed, updatedState, newPaths, deletedPaths, matchingPaths := checkForChanges(tmpFolder, dirState)
	if changed || (len(newPaths)+len(deletedPaths)+len(matchingPaths) != totalFolders) {
		t.Fatal("comparision of current state with current state did not result in empty....ouch\n")
		t.Fail()
	}

	assertEqualsTwoDirTreeMap(t, dirState, updatedState)

	//changed, updatedState, newPaths, deletedPaths, matchingPaths := checkForChanges(globalSettings.Directory, emptyState)

	// todo make some changes and verify that it is working correctly.
	// add paths
	// a, b, c, d, e, ab,abc,abd
	// delete ab and make sure it is the only one deleted
	// delete the start and end ones
	// recreate them

	_ = emptyState
}

func assertEqualsTwoDirTreeMap(t *testing.T, first, second DirTreeMap) {
	if len(first) != len(second) {
		t.Fatal("inconsistent tree lengths")
		t.Fail()
	}
	//todo continue to do the rest of the tests
}
