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
	tracker := new(FilesystemTracker)
	tracker.init()
	defer tracker.cleanup()

	empty := make([]string, 0)
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

	sort.Strings(folderList)
	sort.Strings(folderTemplate)

	if reflect.DeepEqual(folderList, folderTemplate) == false {
		t.Fatal(fmt.Sprintf("Found: %v\nExpected: %v\n", folderList, folderTemplate))
	}

	err := tracker.DeleteFolder(folderTemplate[0])
	if err != nil {
		t.Fatal(err)
	}

	folderList = tracker.ListFolders()
	sort.Strings(folderList)

	folderTemplate = folderTemplate[1:]

	if reflect.DeepEqual(folderList, folderTemplate) == false {
		t.Fatal(fmt.Sprintf("Found: %v\nExpected: %v\n", folderList, folderTemplate))
	}

}

func TestFileChangeTrackerAutoCreateFolderAndCleanup(t *testing.T) {
	tracker := new(FilesystemTracker)
	tracker.init()
	defer tracker.cleanup()

	tmpFolder, err := ioutil.TempDir("", "blank")
	defer os.RemoveAll(tmpFolder)

	tmpFolder = tmpFolder + "R"
	defer os.RemoveAll(tmpFolder)

	logger := &LogOnlyChangeHandler{}
	var loggerInterface ChangeHandler = logger

	tracker.watchDirectory(tmpFolder, &loggerInterface)

	// verify the folder was created
	tmpFolder, err = filepath.EvalSymlinks(tmpFolder)
	if err != nil {
		t.Fatal(err)
	}

	tracker.cleanup()
}

type countingChangeHandler struct {
	folderCreated int
	folderDeleted int
}

func (self *countingChangeHandler) FolderCreated(name string) (err error) {
	fmt.Printf("FolderCreated: %s\n", name)
	self.folderCreated++
	return nil
}

func (self *countingChangeHandler) FolderDeleted(name string) (err error) {
	fmt.Printf("FolderDeleted: %s\n", name)
	self.folderDeleted++
	return nil
}

func TestFileChangeTrackerAddFolders(t *testing.T) {
	tracker := new(FilesystemTracker)
	tracker.init()
	defer tracker.cleanup()

	logHandler := countingChangeHandler{}
	var c ChangeHandler = &logHandler

	tmpFolder, err := ioutil.TempDir("", "blank")
	defer os.RemoveAll(tmpFolder)

	fmt.Println("TestFileChangeTrackerAddFolders: About to call watchDirectory")
	tracker.watchDirectory(tmpFolder, &c)
	fmt.Println("TestFileChangeTrackerAddFolders: Done - About to call watchDirectory")

	numberOfSubFolders := 5

	newFolders := make([]string, 0, numberOfSubFolders)
	for i := 0; i < numberOfSubFolders; i++ {
		path := fmt.Sprintf("%s/a%d", tmpFolder, i)
		newFolders = append(newFolders, path)
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
			fmt.Printf("did not get all folders. Current: %v\n", folderList)
			if cycleCount > 500 {
				t.Fatalf("Did not find enough folders. Got bored of waiting. What was found: %v\n", folderList)
			}
			time.Sleep(time.Millisecond * 5)
		} else {
			fmt.Printf("We have all of our ducks in a row\n")
			break
		}
	}

	// Delete two folders
	fmt.Println(tracker.ListFolders())
	tracker.DeleteFolder("a0")
	//time.Sleep(time.Millisecond * 5)
	tracker.DeleteFolder("a1")

	cycleCount = 0
	expectedSubFolders := numberOfSubFolders - 1 // we have one extra because the current folder has been added automatically
	for {
		cycleCount++
		folderList := tracker.ListFolders()
		if len(folderList) > expectedSubFolders {
			fmt.Printf("Too many subfolders. Current: %v\n", folderList)
			if cycleCount > 10 {
				t.Fatalf("Kept finding too many subfolders. Got bored of waiting. What was found: %v\n", folderList)
			}
			time.Sleep(time.Millisecond * 5)
		} else {
			fmt.Printf("We have all of our ducks in a row again. Yay!\n")
			break
		}
	}

}
