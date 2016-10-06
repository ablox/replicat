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
	"time"
)

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
		err := tracker.CreateFolder(folder)
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
	tmpFolder, err = filepath.EvalSymlinks(tmpFolder)
	if err != nil {
		t.Fatal(err)
	}

	tracker.cleanup()
}

type countingChangeHandler struct {
	FoldersCreated int
	FoldersDeleted int
}

func (self *countingChangeHandler) FolderCreated(name string) (err error) {
	self.FoldersCreated++

	fmt.Printf("countingChangeHandler:FolderCreated: %s (%d)\n", name, self.FoldersCreated)
	return nil
}

func (self *countingChangeHandler) FolderDeleted(name string) (err error) {
	self.FoldersDeleted++

	fmt.Printf("countingChangeHandler:FolderDeleted: %s (%d)\n", name, self.FoldersDeleted)
	return nil
}

func TestFileChangeTrackerAddFolders(t *testing.T) {
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

	numberOfSubFolders := 5

	newFolders := make([]string, 0, numberOfSubFolders)
	for i := 0; i < numberOfSubFolders; i++ {
		path := fmt.Sprintf("%s/a%d", tmpFolder, i)
		newFolders = append(newFolders, path)

		fmt.Printf("Making directory: %s\n", path)

		err = os.Mkdir(path, os.ModeDir+os.ModePerm)
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(time.Millisecond * 10)
	}
	fmt.Printf("done with creating %d different subfolders. :)\n", numberOfSubFolders)

	cycleCount := 0
	for {
		cycleCount++
		folderList := tracker.ListFolders()
		if len(folderList) < numberOfSubFolders {
			//fmt.Printf("did not get all folders. Current: %v\n", folderList)
			if cycleCount > 500 {
				t.Fatalf("Did not find enough folders. Got bored of waiting. What was found: %v\n", folderList)
			}
			time.Sleep(time.Millisecond * 20)
		} else {
			fmt.Printf("We have all of our ducks in a row\n")
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

	expectedCreated := 6
	expectedDeleted := 2

	// wait for the final tally to come through.
	cycleCount = 0
	for {
		cycleCount++

		if logHandler.FoldersCreated != expectedCreated || logHandler.FoldersDeleted != expectedDeleted {
			if cycleCount > 4000 {
				fmt.Printf("Kept finding bad data. Got bored of waiting. What was found: %v\n", tracker.ListFolders())
				break
			}
			time.Sleep(time.Millisecond * 20)
		} else {
			fmt.Println("We have all of our ducks in a row again. Yay!")
			break
		}
	}

	if logHandler.FoldersCreated != expectedCreated || logHandler.FoldersDeleted != expectedDeleted {
		t.Fatalf("Expected/Found created: (%d/%d) deleted: (%d/%d)\n", expectedCreated, logHandler.FoldersCreated, expectedDeleted, logHandler.FoldersDeleted)
	}
}
