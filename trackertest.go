// Package replicat is a server for n way synchronization of content (rsync for the cloud).
// More information at: http://replic.at
// Copyright 2016 Jacob Taylor jacob@ablox.io
// License: Apache2 - http://www.apache.org/licenses/LICENSE-2.0
package replicat

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"time"
)

func trackerTestEmptyDirectoryMovesInOutAround() {
	monitoredFolder, _ := ioutil.TempDir("", "monitored")
	outsideFolder, _ := ioutil.TempDir("", "outside")
	defer os.RemoveAll(monitoredFolder)
	defer os.RemoveAll(outsideFolder)

	tracker := new(FilesystemTracker)
	server := ReplicatServer{}
	tracker.init(monitoredFolder, &server)
	defer tracker.cleanup()

	testName := "trackerTestEmptyDirectoryMovesInOutAround"
	tracker.startTest(testName)
	defer tracker.endTest(testName)

	logger := &LogOnlyChangeHandler{}
	var loggerInterface ChangeHandler = logger
	tracker.watchDirectory(&loggerInterface)

	tracker.printTracker()
	folderName := "happy"
	originalFolderName := folderName
	targetMonitoredPath := filepath.Join(monitoredFolder, folderName)
	targetOutsidePath := filepath.Join(outsideFolder, folderName)

	fmt.Printf("making folder: %s going to rename it to: %s\n", targetOutsidePath, targetMonitoredPath)
	os.Mkdir(targetOutsidePath, os.ModeDir+os.ModePerm)
	fmt.Printf("About to move file \nfrom: %s\n  to: %s\n", targetOutsidePath, targetMonitoredPath)
	os.Rename(targetOutsidePath, targetMonitoredPath)
	stats, _ := os.Stat(targetMonitoredPath)
	fmt.Printf("stats for: %s\n%v\n", targetMonitoredPath, stats)

	helper := func(tracker *FilesystemTracker, folder string) bool {
		tracker.fsLock.Lock()
		defer tracker.fsLock.Unlock()
		_, exists := tracker.contents[folder]
		fmt.Printf("Checking the value of exists: %v\n", exists)
		return exists
	}

	if !WaitFor(tracker, folderName, true, helper) {
		panic(fmt.Sprintf("%s not found in contents\ncontents: %v\n", folderName, tracker.contents))
	}

	tracker.printTracker()
	if len(tracker.renamesInProgress) > 0 {
		panic(fmt.Sprint("6 tracker has renames in progress still"))
	}

	// check to make sure that there are no invalid directories
	tracker.validate()

	folderName = folderName + "b"
	moveSourcePath := targetMonitoredPath
	moveDestinationPath := monitoredFolder + "/" + folderName
	fmt.Printf("About to move file \nfrom: %s\n  to: %s\n", moveSourcePath, moveDestinationPath)
	os.Rename(moveSourcePath, moveDestinationPath)

	if !WaitFor(tracker, originalFolderName, false, helper) {
		panic(fmt.Sprintf("Still finding originalFolderName %s after rename timeout \ncontents: %v\n", originalFolderName, tracker.contents))
	}

	if !WaitFor(tracker, folderName, true, helper) {
		panic(fmt.Sprintf("%s not found after renamte timout\ncontents: %v\n", folderName, tracker.contents))
	}
	tracker.printTracker()
	if len(tracker.renamesInProgress) > 0 {
		tracker.printTracker()
		panic(fmt.Sprint("11 tracker has renames in progress still"))
	}

	// check to make sure that there are no invalid directories
	tracker.validate()

	moveSourcePath = moveDestinationPath
	moveDestinationPath = targetOutsidePath

	fmt.Printf("About to move file \nfrom: %s\n  to: %s\n", moveSourcePath, moveDestinationPath)
	os.Rename(moveSourcePath, moveDestinationPath)

	if !WaitFor(tracker, folderName, false, helper) {
		fmt.Printf("Tracker contents: %v\n", tracker.contents)
		panic(fmt.Sprintf("%s not cleared from contents\ncontents: %v\n", folderName, tracker.contents))
	}
	tracker.printTracker()
}

func trackerTestFileChangeTrackerAddFolders() {
	logHandler := countingChangeHandler{}
	var c ChangeHandler = &logHandler

	tmpFolder, err := ioutil.TempDir("", "blank")
	defer os.RemoveAll(tmpFolder)

	fmt.Println("TestFileChangeTrackerAddFolders: About to call watchDirectory")
	tracker := new(FilesystemTracker)
	server := ReplicatServer{}
	tracker.init(tmpFolder, &server)
	defer tracker.cleanup()

	testName := "trackerTestFileChangeTrackerAddFolders"
	tracker.startTest(testName)
	defer tracker.endTest(testName)

	tracker.watchDirectory(&c)
	fmt.Println("TestFileChangeTrackerAddFolders: Done - About to call watchDirectory")

	numberOfSubFolders := 10

	for i := 0; i < numberOfSubFolders; i++ {
		path := fmt.Sprintf("%s/a%d", tmpFolder, i)

		err = os.Mkdir(path, os.ModeDir+os.ModePerm)
		if err != nil {
			panic(err)
		}
	}
	fmt.Printf("done with creating %d different subfolders. :)\n", numberOfSubFolders)

	// todo - convert to waitfor
	cycleCount := 0
	for {
		cycleCount++
		folderList := tracker.ListFolders()
		if len(folderList) < numberOfSubFolders {
			//fmt.Printf("did not get all folders. Current: %v\n", folderList)
			if cycleCount > 20 {
				panic(fmt.Sprintf("Did not find enough folders. Got bored of waiting. What was found: %v\n", folderList))
			}
			time.Sleep(time.Millisecond * 50)
		} else {
			fmt.Println("We have all of our ducks in a row")
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

	expectedCreated := numberOfSubFolders
	expectedDeleted := 2
	created := 0
	updated := 0
	deleted := 0

	tracker.printTracker()

	// wait for the final tally to come through.
	cycleCount = 0
	for {
		cycleCount++

		created, deleted, updated = logHandler.GetFolderStats()
		if created != expectedCreated || deleted != expectedDeleted {
			if cycleCount > 20 || created > expectedCreated || deleted > expectedDeleted {
				tracker.printTracker()
				panic(fmt.Sprintf("Expected/Found created: (%d/%d) deleted: (%d/%d)\n", expectedCreated, created, expectedDeleted, deleted))
			}
			time.Sleep(time.Millisecond * 50)
		} else {
			fmt.Println("We have all of our ducks in a row again. Yay!")
			break
		}
	}

	if created != expectedCreated || deleted != expectedDeleted {
		panic(fmt.Sprintf("Expected/Found created: (%d/%d) deleted: (%d/%d) updated: %d\n", expectedCreated, created, expectedDeleted, deleted, updated))
	}

	tracker.fsLock.Lock()
	rootDirectory, exists := tracker.contents["."]
	tracker.fsLock.Unlock()

	if !exists {
		fmt.Println("root directory fileinfo is nil")
	} else {
		fmt.Printf("Root directory %v\n", rootDirectory)
		fmt.Printf("Root directory named: %s and has size %d\n", rootDirectory.Name(), rootDirectory.Size())
	}
}

func trackerTestSmallFileCreationAndRename() {
	monitoredFolder, _ := ioutil.TempDir("", "monitored")
	outsideFolder, _ := ioutil.TempDir("", "outside")
	defer os.RemoveAll(monitoredFolder)
	defer os.RemoveAll(outsideFolder)

	tracker := new(FilesystemTracker)
	server := ReplicatServer{}
	tracker.init(monitoredFolder, &server)
	defer tracker.cleanup()

	testName := "trackerTestSmallFileCreationAndRename"
	tracker.startTest(testName)
	defer tracker.endTest(testName)

	logger := &LogOnlyChangeHandler{}
	var loggerInterface ChangeHandler = logger
	tracker.watchDirectory(&loggerInterface)

	tracker.printTracker()

	fileName := "happy.txt"
	secondFilename := "behappy.txt"
	targetMonitoredPath := filepath.Join(monitoredFolder, fileName)
	secondMonitoredPath := filepath.Join(monitoredFolder, secondFilename)

	fmt.Printf("making file: %s\n", targetMonitoredPath)
	file, err := os.Create(targetMonitoredPath)
	if err != nil {
		panic(err)
	}

	sampleFileContents := "This is the content of the file\n"
	n, err := file.WriteString(sampleFileContents)
	if err != nil {
		panic(err)
	}
	if n != len(sampleFileContents) {
		panic(fmt.Sprintf("Contents of file not correct length n: %d len: %d\n", n, len(sampleFileContents)))
	}

	err = file.Close()
	if err != nil {
		panic(err)
	}

	helper := func(tracker *FilesystemTracker, path string) bool {
		tracker.fsLock.Lock()
		defer tracker.fsLock.Unlock()

		entry, exists := tracker.contents[path]
		if !exists {
			return false
		}

		fmt.Printf("entry is: %#v\npath: %s\n", entry, path)

		return true
	}

	if !WaitFor(tracker, fileName, true, helper) {
		panic(fmt.Sprintf("%s not found in contents\ncontents: %v\n", fileName, tracker.contents))
	}

	tracker.printTracker()

	fmt.Printf("Moving file \nfrom: %s\n  to: %s\n", targetMonitoredPath, secondMonitoredPath)
	os.Rename(targetMonitoredPath, secondMonitoredPath)

	stats, err := os.Stat(secondMonitoredPath)
	if err != nil {
		panic(fmt.Sprintf("failed to move file: %v", err))
	}
	fmt.Printf("stats for: %s\n%v\n", targetMonitoredPath, stats)

	if !WaitFor(tracker, fileName, false, helper) {
		panic(fmt.Sprintf("%s found in contents\ncontents: %v\n", fileName, tracker.contents))
	}

	if !WaitFor(tracker, secondFilename, true, helper) {
		panic(fmt.Sprintf("%s not found in contents\ncontents: %v\n", secondFilename, tracker.contents))
	}

	if len(tracker.renamesInProgress) > 0 {
		panic(fmt.Sprint("6 tracker has renames in progress still"))
	}

	// check to make sure that there are no invalid directories
	tracker.validate()
}

func trackerTestSmallFileCreationAndUpdate() {
	monitoredFolder, _ := ioutil.TempDir("", "monitored")
	defer os.RemoveAll(monitoredFolder)

	tracker := new(FilesystemTracker)
	server := ReplicatServer{}
	tracker.init(monitoredFolder, &server)
	defer tracker.cleanup()

	testName := "trackerTestSmallFileCreationAndUpdate"
	tracker.startTest(testName)
	defer tracker.endTest(testName)

	logger := &countingChangeHandler{}
	var loggerInterface ChangeHandler = logger
	tracker.watchDirectory(&loggerInterface)

	tracker.printTracker()

	fileName := "happy.txt"
	targetMonitoredPath := filepath.Join(monitoredFolder, fileName)

	fmt.Printf("making file: %s\n", targetMonitoredPath)
	file, err := os.Create(targetMonitoredPath)
	if err != nil {
		panic(err)
	}

	sampleFileContents := "This is the content of the file\n"
	n, err := file.WriteString(sampleFileContents)
	if err != nil {
		panic(err)
	}
	if n != len(sampleFileContents) {
		panic(fmt.Sprintf("Contents of file not correct length n: %d len: %d\n", n, len(sampleFileContents)))
	}

	err = file.Close()
	if err != nil {
		panic(err)
	}

	helper := func(tracker *FilesystemTracker, path string) bool {
		tracker.fsLock.Lock()
		defer tracker.fsLock.Unlock()
		entry, exists := tracker.contents[path]
		if !exists {
			return false
		}

		fmt.Printf("entry is: %#v\npath: %s\n", entry, path)

		return true
	}

	if !WaitFor(tracker, fileName, true, helper) {
		panic(fmt.Sprintf("%s not found in contents\ncontents: %v\n", fileName, tracker.contents))
	}

	tracker.printTracker()
	tracker.validate()

	// Open the file and make a change to make sure the write event is tracked and sent
	fmt.Printf("opening file to modify: %s\n", targetMonitoredPath)
	file, err = os.OpenFile(targetMonitoredPath, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		panic(err)
	}

	_, err = file.WriteString("And we have a second line!!!!\n")
	if err != nil {
		panic(err)
	}

	fmt.Println("we just wrote the second line...")
	file.Close()
	fmt.Println("file closed...")

	time.Sleep(50 * time.Millisecond)
	created, deleted, updated := logger.GetFileStats()
	if created != 1 || updated != 2 {
		panic(fmt.Sprintf("Expected created 1 updated 2. Actual Values: created %d updated %d deleted %d\n", created, updated, deleted))
	}

}

func testTrackerStatusAndScanInitialFiles() {
	monitoredFolder, _ := ioutil.TempDir("", "monitored")
	defer os.RemoveAll(monitoredFolder)

	tracker := new(FilesystemTracker)
	server := ReplicatServer{}
	tracker.init(monitoredFolder, &server)
	defer tracker.cleanup()

	//if tracker.
	panic("probably should do something")

	// todo complete this test. The folder needs to be there and the single file need to be there.
}

func trackerTestSmallFileInSubfolder() {
	monitoredFolder, _ := ioutil.TempDir("", "monitored")
	defer os.RemoveAll(monitoredFolder)

	tracker := new(FilesystemTracker)
	server := ReplicatServer{}
	tracker.init(monitoredFolder, &server)
	defer tracker.cleanup()

	testName := "trackerTestSmallFileInSubfolder"
	tracker.startTest(testName)
	defer tracker.endTest(testName)

	logger := &countingChangeHandler{}
	var loggerInterface ChangeHandler = logger
	tracker.watchDirectory(&loggerInterface)

	tracker.printTracker()

	fileName := filepath.Join("subfolder", "happy.txt")
	fmt.Printf("about to create nested folder: %s\n", fileName)

	// create the subfolder and the file underneath it.
	tracker.CreatePath(fileName, false)

	time.Sleep(50 * time.Millisecond)
	tracker.printTracker()
	tracker.validate()

	// todo complete this test. The folder needs to be there and the single file need to be there.
}

func trackerTestSmallFileMovesInOutAround() {
	monitoredFolder, _ := ioutil.TempDir("", "monitored")
	outsideFolder, _ := ioutil.TempDir("", "outside")
	defer os.RemoveAll(monitoredFolder)
	defer os.RemoveAll(outsideFolder)

	tracker := new(FilesystemTracker)
	server := ReplicatServer{}
	tracker.init(monitoredFolder, &server)
	defer tracker.cleanup()

	testName := "trackerTestSmallFileMovesInOutAround"
	tracker.startTest(testName)
	defer tracker.endTest(testName)

	logger := &LogOnlyChangeHandler{}
	var loggerInterface ChangeHandler = logger
	tracker.watchDirectory(&loggerInterface)

	tracker.printTracker()

	fileName := "happy"
	targetMonitoredPath := filepath.Join(monitoredFolder, fileName)
	targetOutsidePath := filepath.Join(outsideFolder, fileName)

	fmt.Printf("making file: %s\n", targetOutsidePath)
	file, err := os.Create(targetOutsidePath)
	if err != nil {
		panic(err)
	}

	sampleFileContents := "This is the content of the file\n"
	n, err := file.WriteString(sampleFileContents)
	if err != nil {
		panic(err)
	}
	if n != len(sampleFileContents) {
		panic(fmt.Sprintf("Contents of file not correct length n: %d len: %d\n", n, len(sampleFileContents)))
	}

	err = file.Close()
	if err != nil {
		panic(err)
	}

	fmt.Printf("making file: %s going to rename it to: %s\n", targetOutsidePath, targetMonitoredPath)
	os.Rename(targetOutsidePath, targetMonitoredPath)

	stats, _ := os.Stat(targetMonitoredPath)
	fmt.Printf("stats for: %s\n%v\n", targetMonitoredPath, stats)

	fmt.Println("Hit the end of this test.....needs more! Files are currently being treated as directories.....stop stat....\n\n\n\n\n\n\ngaaak")
}

func trackerTestDirectoryCreation() {
	tmpFolder, _ := ioutil.TempDir("", "blank")
	defer os.RemoveAll(tmpFolder)

	tracker := new(FilesystemTracker)
	server := ReplicatServer{}
	tracker.init(tmpFolder, &server)
	defer tracker.cleanup()

	testName := "trackerTestDirectoryCreation"
	tracker.startTest(testName)
	defer tracker.endTest(testName)

	testDirectory := tmpFolder + "/newbie"
	before, err := os.Stat(testDirectory)

	os.Mkdir(testDirectory, os.ModeDir+os.ModePerm)

	after, err := os.Stat(testDirectory)

	if err != nil {
		panic("GACK, directory should exist")
	}

	err = os.Remove(testDirectory)

	_, err = os.Stat(testDirectory)

	if !os.IsNotExist(err) {
		panic("The folder still exists.....oopps")
	}

	fmt.Printf("TestDirectoryCreation\nbefore: %v\nafter: %v\n", before, after)
}

func trackerTestNestedDirectoryCreation() {
	// create monitored a/b/c/d/e/f
	logHandler := countingChangeHandler{}
	var c ChangeHandler = &logHandler

	// check to see if they are all in the contents
	tmpFolder, _ := ioutil.TempDir("", "blank")
	defer os.RemoveAll(tmpFolder)

	tracker := new(FilesystemTracker)
	server := ReplicatServer{}
	tracker.init(tmpFolder, &server)
	tracker.watchDirectory(&c)
	defer tracker.cleanup()

	testName := "trackerTestNestedDirectoryCreation"
	tracker.startTest(testName)
	defer tracker.endTest(testName)

	os.Mkdir(tmpFolder+"/a", os.ModeDir+os.ModePerm)
	time.Sleep(10 * time.Millisecond)
	os.Mkdir(tmpFolder+"/a/b", os.ModeDir+os.ModePerm)
	time.Sleep(10 * time.Millisecond)
	os.Mkdir(tmpFolder+"/a/b/c", os.ModeDir+os.ModePerm)
	time.Sleep(10 * time.Millisecond)
	os.Mkdir(tmpFolder+"/a/b/c/d", os.ModeDir+os.ModePerm)
	time.Sleep(10 * time.Millisecond)
	os.Mkdir(tmpFolder+"/a/b/c/d/e", os.ModeDir+os.ModePerm)
	time.Sleep(10 * time.Millisecond)
	os.Mkdir(tmpFolder+"/a/b/c/d/e/f", os.ModeDir+os.ModePerm)
	time.Sleep(10 * time.Millisecond)

	expectedCreated := 6
	expectedDeleted := 0
	created := 0
	updated := 0
	deleted := 0

	// wait for the final tally to come through.
	cycleCount := 0
	for {
		cycleCount++

		created, updated, deleted = logHandler.GetFolderStats()
		if created == expectedCreated && deleted == expectedDeleted {
			fmt.Println("We have all of our ducks in a row again. Yay!")
			break
		}

		fmt.Printf("We have made another round: Expected/Found created: (%d/%d) deleted: (%d/%d)\n", expectedCreated, created, expectedDeleted, deleted)
		if cycleCount > 20 {
			tracker.printTracker()
			panic(fmt.Sprintf("Expected/Found created: (%d/%d) deleted: (%d/%d) updated: %d\n", expectedCreated, created, expectedDeleted, deleted, updated))
		}
		time.Sleep(time.Millisecond * 50)
	}

	if logHandler.FoldersCreated != expectedCreated || logHandler.FoldersDeleted != expectedDeleted {
		panic(fmt.Sprintf("Expected/Found created: (%d/%d) deleted: (%d/%d)\n", expectedCreated, logHandler.FoldersCreated, expectedDeleted, logHandler.FoldersDeleted))
	}

	tracker.printTracker()
}

func trackerTestNestedFastDirectoryCreation() {
	// create monitored a/b/c/d/e/f
	logHandler := countingChangeHandler{}
	var c ChangeHandler = &logHandler

	// check to see if they are all in the contents
	tmpFolder, _ := ioutil.TempDir("", "blank")
	defer os.RemoveAll(tmpFolder)

	tracker := new(FilesystemTracker)
	server := ReplicatServer{}
	tracker.init(tmpFolder, &server)
	tracker.watchDirectory(&c)
	defer tracker.cleanup()

	testName := "trackerTestNestedFastDirectoryCreation"
	tracker.startTest(testName)
	defer tracker.endTest(testName)

	nestedRelativePath := "a/b/c/d/e/f"
	fullPath := tmpFolder + "/" + nestedRelativePath

	os.MkdirAll(fullPath, os.ModeDir+os.ModePerm)

	expectedCreated := 6
	expectedDeleted := 0
	created := 0
	updated := 0
	deleted := 0

	// wait for the final tally to come through.
	cycleCount := 0
	for {
		cycleCount++

		created, updated, deleted = logHandler.GetFolderStats()
		if created == expectedCreated && deleted == expectedDeleted {
			fmt.Println("We have all of our ducks in a row again. Yay!")
			break
		}

		fmt.Printf("We have made another round: Expected/Found created: (%d/%d) deleted: (%d/%d)\n", expectedCreated, created, expectedDeleted, deleted)
		if cycleCount > 20 {
			tracker.printTracker()
			panic(fmt.Sprintf("Expected/Found created: (%d/%d) deleted: (%d/%d)\n", expectedCreated, created, expectedDeleted, deleted))
		}
		time.Sleep(time.Millisecond * 50)
	}

	if created != expectedCreated || deleted != expectedDeleted {
		panic(fmt.Sprintf("Expected/Found created: (%d/%d) deleted: (%d/%d) updated: %d\n", expectedCreated, logHandler.FoldersCreated, expectedDeleted, logHandler.FoldersDeleted, updated))
	}

	tracker.printTracker()
}

func trackerTestDirectoryStorage() {
	tmpFolder, err := ioutil.TempDir("", "blank")
	defer os.RemoveAll(tmpFolder)

	tracker := new(FilesystemTracker)
	server := ReplicatServer{}
	tracker.init(tmpFolder, &server)
	defer tracker.cleanup()

	testName := "trackerTestDirectoryStorage"
	tracker.startTest(testName)
	defer tracker.endTest(testName)

	empty := make([]string, 0)

	folderList := tracker.ListFolders()

	fmt.Printf("empty: %v\nlist: %v\n", empty, folderList)

	if reflect.DeepEqual(empty, folderList) == false {
		panic(fmt.Sprintf("expected empty folder, found something: %v\n", folderList))
	}

	folderTemplate := []string{"A", "B", "C"}

	for _, folder := range folderTemplate {
		err := tracker.CreatePath(folder, true)
		if err != nil {
			panic(err)
		}
	}

	folderList = tracker.ListFolders()
	sort.Strings(folderList)

	sort.Strings(folderTemplate)

	if reflect.DeepEqual(folderTemplate, folderList) == false {
		panic(fmt.Sprintf("Found: %v\nExpected: %v\n", folderList, folderTemplate))
	}

	err = tracker.DeleteFolder(folderTemplate[0])
	if err != nil {
		panic(err)
	}

	folderList = tracker.ListFolders()
	sort.Strings(folderList)

	folderTemplate = folderTemplate[1:]
	sort.Strings(folderTemplate)

	if reflect.DeepEqual(folderList, folderTemplate) == false {
		panic(fmt.Sprintf("Found: %v\nExpected: %v\n", folderList, folderTemplate))
	}

}

func trackerTestFileChangeTrackerAutoCreateFolderAndCleanup() {
	tracker := new(FilesystemTracker)

	tmpFolder, err := ioutil.TempDir("", "blank")
	defer os.RemoveAll(tmpFolder)

	tmpFolder = tmpFolder + "R"
	defer os.RemoveAll(tmpFolder)

	logger := &LogOnlyChangeHandler{}
	var loggerInterface ChangeHandler = logger

	server := ReplicatServer{}
	tracker.init(tmpFolder, &server)
	tracker.watchDirectory(&loggerInterface)

	// verify the folder was created
	_, err = filepath.EvalSymlinks(tmpFolder)
	if err != nil {
		panic(err)
	}

	tracker.cleanup()
}
