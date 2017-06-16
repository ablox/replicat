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
	"log"
	"math/rand"
	"runtime"
	"time"
)

func createMinioTracker(prefix string) (tracker *MinioTracker) {
	rand.Seed(int64(time.Now().Nanosecond()))

	tracker = new(MinioTracker)
	bucketName := generateBucketName(prefix)
	tracker.Initialize(bucketName, nil)

	pc, _, _, _ := runtime.Caller(1)
	details := runtime.FuncForPC(pc)
	startTest(details.Name())

	return tracker
}

func cleanupMinioTracker(tracker StorageTracker) {
	tracker.cleanupAndDelete()

	pc, _, _, _ := runtime.Caller(1)
	details := runtime.FuncForPC(pc)
	endTest(details.Name())
}

func generateBucketName(prefix string) (name string) {
	suffix := rand.Int31n(10000)
	if prefix == "" {
		prefix = "replicat-test"
	}
	name = fmt.Sprintf("%s-%05d", prefix, suffix)
	fmt.Printf("generatedBucketName: %s\n", name)
	return
}

// Quick test if the minio integration to exercise all of the functionality that is working so far.
func exerciseMinio() {
	tracker2 := &MinioTracker{}
	tracker2.Initialize("", nil)

	tempFolder := generateBucketName("replicat-test")

	objectName := "babySloth"
	secondObjectName := "cuteBabySloth"

	err := tracker2.CreatePath(tempFolder, true)
	if err != nil {
		panic(err)
	}

	go watchBucketTest(tracker2, tempFolder, allEvents, tracker2.doneCh)

	folderlist, err := tracker2.ListFolders(false)

	for _, bucket := range folderlist {
		go watchBucketTest(tracker2, bucket, allEvents, tracker2.doneCh)
	}

	found := false
	for _, v := range folderlist {
		if v == tempFolder {
			found = true
			break
		}
	}
	if found == false {
		fmt.Printf("Could not find the directory we should have just created: %s\n", tempFolder)
	}

	// Time to create file!
	err = tracker2.CreateObject(tempFolder, objectName, "/testdata/LargeSampleImages/baby sloth on a rail.jpg", "")
	if err != nil {
		panic(err)
	}

	//Just have list buckets and delete working (buckets and objects). Also have create object from file working.
	//Now it is time to have a little more fun. Finish out the methods and start to add watchers

	//time.Sleep(time.Second * 30)

	err = tracker2.RenameObject(tempFolder, secondObjectName, tempFolder+"/"+objectName)
	if err != nil {
		panic(err)
	}

	//time.Sleep(time.Second * 15)

	err = tracker2.DeleteObject(tempFolder, secondObjectName)
	if err != nil {
		panic(err)
	}

	err = tracker2.DeleteFolder(tempFolder)
	if err != nil {
		panic(err)
	}

	folderlist, err = tracker2.ListFolders(true)

	found = false
	for _, v := range folderlist {
		if v == tempFolder {
			found = true
			break
		}
	}

	if found == true {
		fmt.Printf("We found our folder after it should have been deleted: %s\n", tempFolder)
	}

	fmt.Println("YAY - Made it to the end")

	time.Sleep(time.Duration(60 * time.Second))

	if tracker2 != nil {
		panic("nobody knows the trouble I've seen?\n")
	}

}

func watchBucketTest(tracker2 *MinioTracker, tempFolder string, allEvents []string, doneCh chan struct{}) {
	// Listen for bucket notifications on "mybucket" filtered by prefix, suffix and events.
	for notificationInfo := range tracker2.minioSDK.ListenBucketNotification(tempFolder, "", "", allEvents, doneCh) {
		if notificationInfo.Err != nil {
			log.Fatalln(notificationInfo.Err)
		}
		log.Printf("S3 NOTIFICATION: %s\n", notificationInfo)
	}
}

/*
func createExtraFolder(prefix string) (name string) {
	name, _ = ioutil.TempDir("", prefix)
	return
}

func cleanupExtraFolder(name string) {
	os.RemoveAll(name)
}

func waitForTrackerFolderExists(tracker StorageTracker, folder string) bool {
	tracker.rlock()
	defer tracker.runlock()

	folders, err := tracker.ListFolders(false)
	if err != nil {
		panic("error when listing folders.")
	}

	for _, oneFolder := range folders {
		if oneFolder == folder {
			return true
		}
	}

	return false
}

func waitForEmptyRenamesInProgress(tracker *FilesystemTracker, _ string) bool {
	tracker.fsLock.Lock()
	defer tracker.fsLock.Unlock()
	return len(tracker.renamesInProgress) == 0
}

func waitForTrackerFolderCount(tracker StorageTracker, numberOfFolders string) bool {
	folderCount, _ := strconv.Atoi(numberOfFolders)
	folderList, err := tracker.ListFolders(true)
	if err != nil {
		panic("error when listing folders.")
	}

	trackerFolderCount := len(folderList)
	fmt.Printf("* looking for: %d currently have: %d\n", folderCount, trackerFolderCount)
	return trackerFolderCount == folderCount
}

func trackerTestEmptyDirectoryMovesInOutAround() {
	outsideFolder := createExtraFolder("outside")
	defer cleanupExtraFolder(outsideFolder)

	tracker := createMinioTracker("monitored")
	defer cleanupMinioTracker(tracker)
	monitoredFolder := tracker.directory

	logger := &LogOnlyChangeHandler{}
	var loggerInterface ChangeHandler = logger
	tracker.watchDirectory(&loggerInterface)
	tracker.printLockable(true)
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

	if !WaitForStorage(tracker, folderName, true, waitForTrackerFolderExists) {
		panic(fmt.Sprintf("%s not found in contents\ncontents: %v\n", folderName, tracker.contents))
	}

	tracker.printLockable(true)
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

	if !WaitForStorage(tracker, originalFolderName, false, waitForTrackerFolderExists) {
		panic(fmt.Sprintf("Still finding originalFolderName %s after rename timeout \ncontents: %v\n", originalFolderName, tracker.contents))
	}

	if !WaitForStorage(tracker, folderName, true, waitForTrackerFolderExists) {
		panic(fmt.Sprintf("%s not found after renamte timout\ncontents: %v\n", folderName, tracker.contents))
	}
	tracker.printLockable(true)

	if !WaitForFilesystem(tracker, folderName, true, waitForEmptyRenamesInProgress) {
		tracker.printLockable(true)
		panic(fmt.Sprint("11 tracker has renames in progress still"))
	}

	// check to make sure that there are no invalid directories
	tracker.validate()

	moveSourcePath = moveDestinationPath
	moveDestinationPath = targetOutsidePath

	fmt.Printf("About to move file \nfrom: %s\n  to: %s\n", moveSourcePath, moveDestinationPath)
	os.Rename(moveSourcePath, moveDestinationPath)

	if !WaitForStorage(tracker, folderName, false, waitForTrackerFolderExists) {
		fmt.Printf("Tracker contents: %v\n", tracker.contents)
		panic(fmt.Sprintf("%s not cleared from contents\ncontents: %v\n", folderName, tracker.contents))
	}
	tracker.printLockable(true)
}

func trackerTestFileChangeTrackerAddFolders() {
	logHandler := countingChangeHandler{}
	var c ChangeHandler = &logHandler

	tracker := createMinioTracker("monitored")
	defer cleanupMinioTracker(tracker)
	tmpFolder := tracker.directory

	tracker.watchDirectory(&c)
	fmt.Printf("TestFileChangeTrackerAddFolders: Done - watchDirectory. Tracker is now watching: %s\n", tracker.directory)

	numberOfSubFolders := 10

	for i := 0; i < numberOfSubFolders; i++ {
		path := fmt.Sprintf("%s/a%d", tmpFolder, i)
		fmt.Printf("os.Mkdir: %s\n", path)
		err := os.Mkdir(path, os.ModeDir+os.ModePerm)
		if err != nil {
			panic(err)
		}
	}
	fmt.Printf("done with creating %d different subfolders. \n", numberOfSubFolders)

	folderSeeking := strconv.Itoa(numberOfSubFolders)
	fmt.Printf("***************>>>>>>>>>>> folder seeking: %s num: %d\n", folderSeeking, numberOfSubFolders)
	if !WaitForStorage(tracker, folderSeeking, true, waitForTrackerFolderCount) {
		folders, _ := tracker.ListFolders(true)
		panic(fmt.Sprintf("did not find enough subfolders. Looking for: %d found: %d", numberOfSubFolders, len(folders)))
	}

	//todo figure out why this sleep is needed.
	time.Sleep(time.Millisecond * 500)

	folder1 := fmt.Sprintf("%s/a0", tmpFolder)
	folder2 := fmt.Sprintf("%s/a1", tmpFolder)
	fmt.Printf("about to delete two folders \n%s\n%s\n", folder1, folder2)
	// Delete two folders
	fmt.Println(tracker.ListFolders(true))
	os.Remove(folder1)
	os.Remove(folder2)
	fmt.Printf("deleted two folders \n%s\n%s\n", folder1, folder2)

	expectedCreated := numberOfSubFolders
	expectedDeleted := 2
	created := 0
	updated := 0
	deleted := 0

	tracker.printLockable(true)

	// wait for the final tally to come through.
	cycleCount := 0
	for {
		cycleCount++

		created, deleted, updated = logHandler.GetFolderStats()
		if created != expectedCreated || deleted != expectedDeleted {
			if cycleCount > 20 || created > expectedCreated || deleted > expectedDeleted {
				tracker.printLockable(true)
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


func trackerTestSmallFileCreationAndUpdate() {
	outsideFolder := createExtraFolder("outside")
	defer cleanupExtraFolder(outsideFolder)

	tracker := createMinioTracker("monitored")
	defer cleanupMinioTracker(tracker)
	monitoredFolder := tracker.directory

	logger := &countingChangeHandler{}
	var loggerInterface ChangeHandler = logger
	tracker.watchDirectory(&loggerInterface)

	tracker.printLockable(true)

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

	if !WaitForStorage(tracker, fileName, true, waitForTrackerFolderExists) {
		panic(fmt.Sprintf("%s not found in contents\ncontents: %v\n", fileName, tracker.contents))
	}

	tracker.printLockable(true)
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

func trackerTestSmallFileInSubfolder() {
	outsideFolder := createExtraFolder("outside")
	defer cleanupExtraFolder(outsideFolder)

	tracker := createMinioTracker("monitored")
	defer cleanupMinioTracker(tracker)

	logger := &countingChangeHandler{}
	var loggerInterface ChangeHandler = logger
	tracker.watchDirectory(&loggerInterface)

	tracker.printLockable(true)

	fileName := filepath.Join("subfolder", "happy.txt")
	fmt.Printf("about to create nested folder: %s\n", fileName)

	// create the subfolder and the file underneath it.
	tracker.CreatePath(fileName, false)

	time.Sleep(50 * time.Millisecond)
	tracker.printLockable(true)
	tracker.validate()

	// todo complete this test. The folder needs to be there and the single file need to be there.
}

func trackerTestSmallFileMovesInOutAround() {
	outsideFolder := createExtraFolder("outside")
	defer cleanupExtraFolder(outsideFolder)

	tracker := createMinioTracker("monitored")
	defer cleanupMinioTracker(tracker)
	monitoredFolder := tracker.directory

	logger := &LogOnlyChangeHandler{}
	var loggerInterface ChangeHandler = logger
	tracker.watchDirectory(&loggerInterface)

	tracker.printLockable(true)

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
	outsideFolder := createExtraFolder("outside")
	defer cleanupExtraFolder(outsideFolder)

	tracker := createMinioTracker("monitored")
	defer cleanupMinioTracker(tracker)
	monitoredFolder := tracker.directory

	testDirectory := monitoredFolder + "/newbie"
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

	outsideFolder := createExtraFolder("outside")
	defer cleanupExtraFolder(outsideFolder)

	tracker := createMinioTracker("monitored")
	defer cleanupMinioTracker(tracker)
	monitoredFolder := tracker.directory
	tracker.watchDirectory(&c)

	os.Mkdir(monitoredFolder+"/a", os.ModeDir+os.ModePerm)
	time.Sleep(10 * time.Millisecond)
	os.Mkdir(monitoredFolder+"/a/b", os.ModeDir+os.ModePerm)
	time.Sleep(10 * time.Millisecond)
	os.Mkdir(monitoredFolder+"/a/b/c", os.ModeDir+os.ModePerm)
	time.Sleep(10 * time.Millisecond)
	os.Mkdir(monitoredFolder+"/a/b/c/d", os.ModeDir+os.ModePerm)
	time.Sleep(10 * time.Millisecond)
	os.Mkdir(monitoredFolder+"/a/b/c/d/e", os.ModeDir+os.ModePerm)
	time.Sleep(10 * time.Millisecond)
	os.Mkdir(monitoredFolder+"/a/b/c/d/e/f", os.ModeDir+os.ModePerm)
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
			tracker.printLockable(true)
			panic(fmt.Sprintf("Expected/Found created: (%d/%d) deleted: (%d/%d) updated: %d\n", expectedCreated, created, expectedDeleted, deleted, updated))
		}
		time.Sleep(time.Millisecond * 50)
	}

	if logHandler.FoldersCreated != expectedCreated || logHandler.FoldersDeleted != expectedDeleted {
		panic(fmt.Sprintf("Expected/Found created: (%d/%d) deleted: (%d/%d)\n", expectedCreated, logHandler.FoldersCreated, expectedDeleted, logHandler.FoldersDeleted))
	}

	tracker.printLockable(true)
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
	tracker.Initialize(tmpFolder, &server)
	tracker.watchDirectory(&c)
	defer tracker.cleanupAndDelete()

	testName := "trackerTestNestedFastDirectoryCreation"
	startTest(testName)
	defer endTest(testName)

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
			tracker.printLockable(true)
			panic(fmt.Sprintf("Expected/Found created: (%d/%d) deleted: (%d/%d)\n", expectedCreated, created, expectedDeleted, deleted))
		}
		time.Sleep(time.Millisecond * 50)
	}

	if created != expectedCreated || deleted != expectedDeleted {
		panic(fmt.Sprintf("Expected/Found created: (%d/%d) deleted: (%d/%d) updated: %d\n", expectedCreated, logHandler.FoldersCreated, expectedDeleted, logHandler.FoldersDeleted, updated))
	}

	tracker.printLockable(true)
}

func trackerTestDirectoryStorage() {
	tracker := createMinioTracker("monitored")
	defer cleanupMinioTracker(tracker)

	empty := make([]string, 0)

	folderList, err := tracker.ListFolders(true)
	if err != nil {
		panic(err)
	}

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

	folderList, err = tracker.ListFolders(true)
	if err != nil {
		panic(err)
	}
	sort.Strings(folderList)

	sort.Strings(folderTemplate)

	if reflect.DeepEqual(folderTemplate, folderList) == false {
		panic(fmt.Sprintf("Found: %v\nExpected: %v\n", folderList, folderTemplate))
	}

	err = tracker.DeleteFolder(folderTemplate[0])
	if err != nil {
		panic(err)
	}

	folderList, err = tracker.ListFolders(true)
	if err != nil {
		panic(err)
	}
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
	tracker.Initialize(tmpFolder, &server)
	tracker.watchDirectory(&loggerInterface)

	// verify the folder was created
	_, err = filepath.EvalSymlinks(tmpFolder)
	if err != nil {
		panic(err)
	}

	tracker.cleanupAndDelete()
}

*/
