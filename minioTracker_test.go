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
	"bufio"
	"fmt"
	"github.com/minio/minio-go"
	log "github.com/sirupsen/logrus"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
	//"strings"
	"strings"
	"math/rand"
)

type minioInfo struct {
	command       *exec.Cmd
	monitoredPath string
	bucketName    string
	minioURL      string
	key           string
	secret        string
	address       string
	status        chan string
}



func relayOutput(minio minioInfo, stdout io.ReadCloser) {
	//log.Println("relay: 1")
	scanner := bufio.NewScanner(stdout)
	//log.Println("relay: 2")
	started := false
	minioStartupComplete := "Drive Capacity:"

	for scanner.Scan() {
		//log.Println("relay: 3")
		log.Printf("MINIO - %s", scanner.Text()) // write each line to your log, or anything you need
		//log.Println("relay: 4")

		if !started && len(scanner.Text()) > len(minioStartupComplete) && strings.Compare(scanner.Text()[:len(minioStartupComplete)], minioStartupComplete) == 0 {
			fmt.Printf("We found it!!!! minio is ready: %s\n", scanner.Text())
			started = true
			go updateStatus(minio, "started")
		}
	}
	//log.Println("relay: 5")

	if err := scanner.Err(); err != nil {
		log.Printf("error: %s", err)
	}
	//log.Println("relay: 6")

}

func updateStatus(minio minioInfo, status string) {
	minio.status <- status
}

/* get the minio server ready to be launched. This allows for the creation of the folders and allows for
file manipulation. if lanchNow is true, it will launch immediately
 */
func setupMinioServer(t *testing.T, host, port string, path string, launchNow bool) (minio minioInfo) {
	log.Printf("setupMinioServer called with host: %s and port: %s", host, port)
	minio = minioInfo{}
	minio.status = make(chan string)

	minio.monitoredPath = path
	if path == "" {
		minio.monitoredPath = fmt.Sprintf("replicat_prefix%d", rand.Intn(10000))
	}

	createExtraFolder(minio.monitoredPath)

	log.Printf("Creating new path: %s", minio.monitoredPath)

	minio.address = fmt.Sprintf("%s:%s", host, port)

	return minio
}

func launchMinio(t *testing.T, minio minioInfo) {
	log.Printf("launchMinio called with host: %s and port: %s", minio.address)

	bin, err := exec.LookPath("minio")
	if err != nil {
		t.Fatal(err)
	}

	args := []string{
		"minio",
		"server",
		"--address",
		minio.address,
		minio.monitoredPath,
	}

	os.Setenv("MINIO_ACCESS_KEY", minio.monitoredPath)
	os.Setenv("MINIO_SECRET_KEY", minio.monitoredPath)
	env := os.Environ()

	cmd := exec.Command(bin)
	cmd.Env = env
	cmd.Args = args

	go updateStatus(minio, "starting")

	stdout, err := cmd.StdoutPipe()
	go relayOutput(minio, stdout)

	minio.command = cmd

	log.Printf("About to start minio at: %s\n", bin)
	if err := cmd.Start(); err != nil {
		log.Fatal(err)
	}

	// Wait until minio is started up
	doneStarting := false
	for !doneStarting {
		select {
		case status := <-minio.status:
			log.Printf("minio status updated to %s", status)
			if strings.Compare(status, "started") == 0 {
				log.Printf("minio has started %s", status)
				doneStarting = true
				break
			}
		case <-time.After(time.Second * 10):
			t.Fatalf("minio did not finish starting %s", minio.status)
		}
	}

	//log.Println("Minio Started!!!!!\n. Waiting for you to be done playing")
	//time.Sleep(time.Second * 10)
	log.Println("you are taking too long, I am bored. Bye")
	cmd.Process.Kill()
	log.Println("process down")

	go updateStatus(minio, "stopped")

}


func TestInitialStartupScan( t *testing.T) {
	startTest("TestInitialStartupScan")
	defer endTest("TestInitialStartupScan")

	minio := setupMinioServer(t, "localhost", "8888", "", false)

	// create a new file in the monitored folder.
	fileName := "happy.txt"
	targetMonitoredPath := filepath.Join(minio.monitoredPath, fileName)

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

	if !WaitForStorage(minio, fileName, true, waitForTrackerFolderExists) {
		panic(fmt.Sprintf("%s not found in contents\ncontents: %v\n", fileName, minio.contents))
	}

}


func stopMinioServer(t *testing.T, info minioInfo) {

}

func TestMinioSmallObjectCreationAndDeletion(t *testing.T) {
	minioSmallServer := setupMinioServer(t, "localhost", "8888", "", true)

	t.Fail()
	t.Fatalf("down the rabbit hole we go\n%#v\n", minioSmallServer)

	defer causeFailOnPanic(t)

	outsideFolder := createExtraFolder("outside")
	defer cleanupExtraFolder(outsideFolder)

	tracker := createMinioTracker("", "")
	defer cleanupMinioTracker(tracker)

	fmt.Printf("Minio initialized with bucket: %s", tracker.bucketName)

	objectName := "babySloth"

	// The bucket should already exist at this point
	tracker.printLockable(true)
	targetMonitoredPath := filepath.Join(outsideFolder, objectName)

	fmt.Printf("making file: %s", targetMonitoredPath)
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
		t.Fatalf("Contents of file not correct length n: %d len: %d", n, len(sampleFileContents))
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
		log.Printf("Object name: %s", x)
	}

	if len(objectsList) != 1 {
		tempTracker.printLockable(true)
		t.Fatalf("Wrong number of objects found. Expected 1 found %d", len(objectsList))
	}

	if objectsList[0] != objectName {
		t.Fatalf("Wrong object name found. Expected %s found %s", objectName, objectsList[0])
	}

	tracker.DeleteObject(tracker.bucketName, objectName)

	// Get the event from Minio
	tracker.printLockable(true)

	// Initialize minio client object.
	minioSDK, err := minio.New(minioAddress, minioPlayAccessKey, minioPlaySecretKey, true)
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
		t.Fatalf("Object %s was not created. Aborting.", path)
	}

}

func TestTrackerCatchingExternalWrite(t *testing.T) {
	defer causeFailOnPanic(t)

	outsideFolder := createExtraFolder("outside")
	defer cleanupExtraFolder(outsideFolder)

	tracker := createMinioTracker("", "")
	defer cleanupMinioTracker(tracker)

	fmt.Printf("Minio initialized with bucket: %s", tracker.bucketName)

	objectName := "babySloth"

	// The bucket should already exist at this point
	tracker.printLockable(true)
	initialOutput, err := tracker.ListFolders(true)

	if len(initialOutput) != 0 {
		t.Fatalf("Wrong number of contents. Expected: 0, found: %d contents: %s", len(initialOutput), initialOutput)
	}

	targetMonitoredPath := filepath.Join(outsideFolder, objectName)

	fmt.Printf("making file: %s", targetMonitoredPath)
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
		t.Fatalf("Contents of file not correct length n: %d len: %d", n, len(sampleFileContents))
	}

	err = file.Close()
	if err != nil {
		t.Fatal(err)
	}

	// Initialize minio client object.
	minioSDK, err := minio.New(minioAddress, minioPlayAccessKey, minioPlaySecretKey, true)
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
		t.Fatalf("Failed to find item: %s, actual contents: %#v", objectName, folders)
	}

	//todo remove the object and make sure the tracker does not have it anymore.
	err = minioSDK.RemoveObject(tracker.bucketName, objectName)
	if err != nil {
		t.Fatal(err)
	}

	result = WaitForStorage(tracker, objectName, false, waitForTrackerFolderExists)
	if result == false {
		folders, _ := tracker.ListFolders(true)
		t.Fatalf("Failed to delete item: %s, actual contents: %#v", objectName, folders)
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
