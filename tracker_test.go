// Copyright 2016 Jacob Taylor jacob@ablox.io
// License: Apache2 - http://www.apache.org/licenses/LICENSE-2.0
package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

func TestEmptyDirectoryMovesInOutAround(t *testing.T) {
	trackerTestEmptyDirectoryMovesInOutAround()
}

func TestNewFileAndRenameFileInside(t *testing.T) {
	trackerTestSmallFileMovesInOutAround()
}

func TestDirectoryCreation(t *testing.T) {
	tmpFolder, err := ioutil.TempDir("", "blank")
	defer os.RemoveAll(tmpFolder)

	tracker := new(FilesystemTracker)
	tracker.init(tmpFolder)
	defer tracker.cleanup()

	testDirectory := tmpFolder + "/" + "newbie"
	before, err := os.Stat(testDirectory)

	os.Mkdir(testDirectory, os.ModeDir+os.ModePerm)

	after, err := os.Stat(testDirectory)

	if err != nil {
		fmt.Println("GACK, directory should exist")
	}

	err = os.Remove(testDirectory)

	_, err = os.Stat(testDirectory)

	if !os.IsNotExist(err) {
		t.Fatal("The folder still exists.....oopps")
	}

	fmt.Printf("TestDirectoryCreation\nbefore: %v\nafter: %v\n", before, after)

}

func TestDirectoryStorage(t *testing.T) {
	tmpFolder, err := ioutil.TempDir("", "blank")
	defer os.RemoveAll(tmpFolder)

	tracker := new(FilesystemTracker)
	tracker.init(tmpFolder)
	defer tracker.cleanup()

	empty := make([]string, 0)
	empty = append(empty, ".")

	folderList := tracker.ListFolders()

	if reflect.DeepEqual(empty, folderList) == false {
		t.Fatal(fmt.Sprintf("expected empty folder, found something: %v\n", folderList))
	}

	folderTemplate := []string{"A", "B", "C"}

	for _, folder := range folderTemplate {
		err := tracker.CreatePath(folder, true)
		if err != nil {
			t.Fatal(err)
		}
	}

	folderList = tracker.ListFolders()

	expectedFolderList := append(folderTemplate, ".")
	sort.Strings(expectedFolderList)
	sort.Strings(folderList)

	if reflect.DeepEqual(expectedFolderList, folderList) == false {
		t.Fatal(fmt.Sprintf("Found: %v\nExpected: %v\n", folderList, expectedFolderList))
	}

	err = tracker.DeleteFolder(folderTemplate[0])
	if err != nil {
		t.Fatal(err)
	}

	folderList = tracker.ListFolders()
	sort.Strings(folderList)

	folderTemplate = folderTemplate[1:]

	expectedFolderList = append(folderTemplate, ".")
	sort.Strings(expectedFolderList)

	if reflect.DeepEqual(folderList, expectedFolderList) == false {
		t.Fatal(fmt.Sprintf("Found: %v\nExpected: %v\n", folderList, expectedFolderList))
	}

}

func TestFileChangeTrackerAutoCreateFolderAndCleanup(t *testing.T) {
	tracker := new(FilesystemTracker)

	tmpFolder, err := ioutil.TempDir("", "blank")
	defer os.RemoveAll(tmpFolder)

	tmpFolder = tmpFolder + "R"
	defer os.RemoveAll(tmpFolder)

	logger := &LogOnlyChangeHandler{}
	var loggerInterface ChangeHandler = logger

	tracker.init(tmpFolder)
	tracker.watchDirectory(&loggerInterface)

	// verify the folder was created
	_, err = filepath.EvalSymlinks(tmpFolder)
	if err != nil {
		t.Fatal(err)
	}

	tracker.cleanup()
}

func TestFileChangeTrackerAddFolders(t *testing.T) {
	trackerTestFileChangeTrackerAddFolders()
}
