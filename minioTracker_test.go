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
	"github.com/minio/minio-go"
	"log"
	"os"
	"path/filepath"
	"testing"
)

func TestMinioSmallObjectCreationAndDeletion(t *testing.T) {
	defer causeFailOnPanic(t)

	outsideFolder := createExtraFolder("outside")
	defer cleanupExtraFolder(outsideFolder)

	tracker := createMinioTracker("", "")
	defer cleanupMinioTracker(tracker)

	fmt.Printf("Minio initialized with bucket: %s\n", tracker.bucketName)

	objectName := "babySloth"

	// The bucket should already exist at this point
	tracker.printLockable(true)
	targetMonitoredPath := filepath.Join(outsideFolder, objectName)

	fmt.Printf("making file: %s\n", targetMonitoredPath)
	file, err := os.Create(targetMonitoredPath)
	if err != nil {
		t.Fatal(err)
	}

	sampleFileContents := "This is the content of the file\n"
	n, err := file.WriteString(sampleFileContents)
	if err != nil {
		t.Fatal(err)
	}
	if n != len(sampleFileContents) {
		t.Fatalf("Contents of file not correct length n: %d len: %d\n", n, len(sampleFileContents))
	}

	err = file.Close()
	if err != nil {
		t.Fatal(err)
	}

	tracker.CreateObject(tracker.bucketName, objectName, targetMonitoredPath, "text/plain")
	if err != nil {
		t.Fatal(err)
	}

	tempTracker := createMinioTracker(tracker.bucketName, "")
	defer cleanupMinioTracker(tempTracker)

	log.Println("About to print tempTracker")
	tempTracker.printLockable(true)
	log.Println("Done - About to print tempTracker")

	objectsList, err := tempTracker.ListFolders(true)
	if err != nil {
		t.Fatal(err)
	}

	for _, x := range objectsList {
		log.Printf("Object name: %s\n", x)
	}

	if len(objectsList) != 1 {
		tempTracker.printLockable(true)
		t.Fatalf("Wrong number of objects found. Expected 1 found %d\n", len(objectsList))
	}

	if objectsList[0] != objectName {
		t.Fatalf("Wrong object name found. Expected %s found %s\n", objectName, objectsList[0])
	}

	tracker.DeleteObject(tracker.bucketName, objectName)

	// Get the event from Minio
	tracker.printLockable(true)

	// Initialize minio client object.
	minioSDK, err := minio.New(minioAddress, minioAccessKey, minioSecretKey, true)
	if err != nil {
		t.Fatal(err)
	}

	_, err = minioSDK.FPutObject(tracker.bucketName, objectName, targetMonitoredPath, "text/plain")
	if err != nil {
		t.Fatal(err)
	}

	// at this point, we should have a bucket entry named objectName. Wait until Minio gets it.

	path, err := joinBucketObjectName(tracker.bucketName, objectName)
	if err != nil {
		t.Fatal(err)
	}

	result := WaitForStorage(tracker, objectName, true, waitForTrackerFolderExists)
	if result == false {
		t.Fatalf("Object %s was not created. Aborting.\n", path)
	}

}

func TestTrackerCatchingExternalWrite(t *testing.T) {
	defer causeFailOnPanic(t)

	outsideFolder := createExtraFolder("outside")
	defer cleanupExtraFolder(outsideFolder)

	tracker := createMinioTracker("", "")
	defer cleanupMinioTracker(tracker)

	fmt.Printf("Minio initialized with bucket: %s\n", tracker.bucketName)

	objectName := "babySloth"

	// The bucket should already exist at this point
	tracker.printLockable(true)
	initialOutput, err := tracker.ListFolders(true)

	if len(initialOutput) != 0 {
		t.Fatalf("Wrong number of contents. Expected: 0, found: %d contents: %s\n", len(initialOutput), initialOutput)
	}

	targetMonitoredPath := filepath.Join(outsideFolder, objectName)

	fmt.Printf("making file: %s\n", targetMonitoredPath)
	file, err := os.Create(targetMonitoredPath)
	if err != nil {
		t.Fatal(err)
	}

	sampleFileContents := "This is the content of the file\n"
	n, err := file.WriteString(sampleFileContents)
	if err != nil {
		t.Fatal(err)
	}
	if n != len(sampleFileContents) {
		t.Fatalf("Contents of file not correct length n: %d len: %d\n", n, len(sampleFileContents))
	}

	err = file.Close()
	if err != nil {
		t.Fatal(err)
	}

	// Initialize minio client object.
	minioSDK, err := minio.New(minioAddress, minioAccessKey, minioSecretKey, true)
	if err != nil {
		t.Fatal(err)
	}

	_, err = minioSDK.FPutObject(tracker.bucketName, objectName, targetMonitoredPath, "text/plain")
	if err != nil {
		t.Fatal(err)
	}

	result := WaitForStorage(tracker, objectName, true, waitForTrackerFolderExists)
	if result == false {
		folders, _ := tracker.ListFolders(true)
		t.Fatalf("Failed to find item: %s, actual contents: %#v\n", objectName, folders)
	}

	//todo remove the object and make sure the tracker does not have it anymore.
	err = minioSDK.RemoveObject(tracker.bucketName, objectName)
	if err != nil {
		t.Fatal(err)
	}

	result = WaitForStorage(tracker, objectName, false, waitForTrackerFolderExists)
	if result == false {
		folders, _ := tracker.ListFolders(true)
		t.Fatalf("Failed to delete item: %s, actual contents: %#v\n", objectName, folders)
	}
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
