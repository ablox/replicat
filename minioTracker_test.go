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
	"testing"
	"github.com/minio/minio-go"
)

func TestMinioSmallObjectCreationAndDeletion(t *testing.T) {
	defer causeFailOnPanic(t)

	outsideFolder := createExtraFolder("outside")
	defer cleanupExtraFolder(outsideFolder)

	tracker := createMinioTracker("")
	defer cleanupMinioTracker(tracker)

	fmt.Printf("Minio initialized with bucket: %s\n", tracker.bucketName)

	objectName := "babySloth"

	// The bucket should already exist at this point
	tracker.printLockable(true)
	targetMonitoredPath := filepath.Join(outsideFolder, objectName)

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

	tracker.CreateObject(tracker.bucketName, objectName, targetMonitoredPath, "text/plain")
	if err != nil {
		panic(err)
	}

	tracker.DeleteObject(tracker.bucketName, objectName)

	// Get the event from Minio

	//if !WaitForStorage(tracker, fileName, true, waitForTrackerFolderExists) {
	//	panic(fmt.Sprintf("%s not found in contents\ncontents: %v\n", fileName, tracker.contents))
	//}
	//
	tracker.printLockable(true)

	// Initialize minio client object.
	minioSDK, err := minio.New(minioAddress, minioAccessKey, minioSecretKey, true)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("tt 1001")

	_, err = minioSDK.FPutObject(tracker.bucketName, objectName, targetMonitoredPath, "text/plain")
	if err != nil {
		panic(err)
	}

	// at this point, we should have a bucket entry named objectName. Wait until Minio gets it.
fmt.Println("tt 1002")

	path, err := joinBucketObjectName(tracker.bucketName, objectName)
	if err != nil {
		panic(err.Error())
	}
	fmt.Println("tt 1003")

	result := WaitForStorage(tracker, objectName, true, waitForTrackerFolderExists)
	if result == false {
		panic(fmt.Sprintf("Object %s was not created. Aborting.\n", path))
	}
	fmt.Println("tt 1004")

	//for true {
	//	fmt.Printf("Yolo: looking for bucket/object: %s/%s\n", tracker.bucketName, objectName)
	//
	//
	//	tracker.fsLock.RLock()
	//	_, exists := tracker.contents[path]
	//	tracker.fsLock.RUnlock()
	//	if exists {
	//		fmt.Println("Found it")
	//		break
	//	}
	//
	//	time.Sleep(time.Millisecond * 15)
	//
	//
	//}

	fmt.Println("Made it past ")
	//fmt.Printf("Moving file \nfrom: %s\n  to: %s\n", targetMonitoredPath, secondMonitoredPath)
	//os.Rename(targetMonitoredPath, secondMonitoredPath)



	//stats, err := os.Stat(secondMonitoredPath)
	//if err != nil {
	//	panic(fmt.Sprintf("failed to move file: %v", err))
	//}
	//fmt.Printf("stats for: %s\n%v\n", targetMonitoredPath, stats)
	//
	//if !WaitForStorage(tracker, fileName, false, waitForTrackerFolderExists) {
	//	panic(fmt.Sprintf("%s found in contents\ncontents: %v\n", fileName, tracker.contents))
	//}
	//
	//if !WaitForStorage(tracker, secondFilename, true, waitForTrackerFolderExists) {
	//	panic(fmt.Sprintf("%s not found in contents\ncontents: %v\n", secondFilename, tracker.contents))
	//}
	//
	//if !WaitForFilesystem(tracker, "", true, waitForEmptyRenamesInProgress) {
	//	panic(fmt.Sprint("6 tracker has renames in progress still"))
	//}

	// check to make sure that there are no invalid directories
	//tracker.validate()
}

//REPLICAT_STATUS_INITIAL_SCAN
//func TestTrackerStatusAndScanInitialFiles(t *testing.T) {
//	defer causeFailOnPanic(t)
//	testTrackerStatusAndScanInitialFiles()
//}

//func TestTrackerTestDual(t *testing.T) {
//	defer causeFailOnPanic(t)
//	trackerTestDual()
//}

/*
func TestTrackerTestSmallFileInSubfolder(t *testing.T) {
	defer causeFailOnPanic(t)
	trackerTestSmallFileInSubfolder()
}

func TestEmptyDirectoryMovesInOutAround(t *testing.T) {
	defer causeFailOnPanic(t)
	trackerTestEmptyDirectoryMovesInOutAround()
}

func TestSmallFileMovesInOutAround(t *testing.T) {
	defer causeFailOnPanic(t)
	trackerTestSmallFileMovesInOutAround()
}



func TestSmallFileCreationAndUpdate(t *testing.T) {
	defer causeFailOnPanic(t)
	trackerTestSmallFileCreationAndUpdate()
}

func TestDirectoryCreation(t *testing.T) {
	defer causeFailOnPanic(t)
	trackerTestDirectoryCreation()
}

func TestDirectoryStorage(t *testing.T) {
	defer causeFailOnPanic(t)
	trackerTestDirectoryStorage()
}

func TestFileChangeTrackerAutoCreateFolderAndCleanup(t *testing.T) {
	defer causeFailOnPanic(t)
	trackerTestFileChangeTrackerAutoCreateFolderAndCleanup()
}

func TestFileChangeTrackerAddFolders(t *testing.T) {
	defer causeFailOnPanic(t)
	trackerTestFileChangeTrackerAddFolders()
}

func TestNestedDirectoryCreation(t *testing.T) {
	defer causeFailOnPanic(t)
	trackerTestNestedDirectoryCreation()
}

func TestBelowThisTestsFailOnUbuntu(t *testing.T) {
	defer causeFailOnPanic(t)
	fmt.Print("would love to figure out why this happens")
}

func TestNestedFastDirectoryCreation(t *testing.T) {
	if os.Getenv("CIRCLECI") == "true" {
		t.Skip("skipping test on circleci")
	}

	defer causeFailOnPanic(t)
	trackerTestNestedFastDirectoryCreation()
}
*/
