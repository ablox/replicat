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
	"encoding/json"
	"errors"
	"fmt"
	"github.com/minio/minio-go"
	log "github.com/sirupsen/logrus"
	"os"
	"sort"
	"strings"
	"sync"
)

// MinioTracker - Track a filesystem and keep it in sync
type MinioTracker struct {
	bucketName string
	contents   map[string]MinioEntry
	setup      bool
	//watcher           *ChangeHandler
	//renamesInProgress map[uint64]renameInformation // map from inode to source/destination of items being moved
	fsLock      sync.RWMutex
	server      *ReplicatServer
	neededFiles map[string]EntryJSON
	stats       TrackerStats
	minioSDK    *minio.Client
	doneCh      chan struct{}
}

var allEvents = []string{
	"s3:ObjectCreated:*",
	"s3:ObjectAccessed:*",
	"s3:ObjectRemoved:*",
}

// Make sure we can adhere to the StorageTracker interface
var _ StorageTracker = (*MinioTracker)(nil)

const (
	//minioLocation  = "https://play.minio.io:9000"
	minioAddress       = "play.minio.io:9000"
	minioPlayAccessKey = "Q3AM3UQ867SPQQA43P2F"
	minioPlaySecretKey = "zuf+tfteSlswRu7BJ86wekitnifILbZam1KYY3TG"
)

//func (handler *MinioTracker) Initialize(directory string, server *ReplicatServer) {
//	log.Println("FilesystemTracker:init")
//	handler.fsLock.Lock()
//	defer handler.fsLock.Unlock()
//	log.Println("FilesystemTracker:/init")
//	defer log.Println("FilesystemTracker://init")
//
//	if handler.setup {
//		return
//	}
//
//	log.Printf("FilesystemTracker:init called with %s", directory)
//	handler.directory = directory
//
//	// Make the channel buffered to ensure no event is dropped. Notify will drop
//	// an event if the receiver is not able to keep up the sending pace.
//	handler.fsEventsChannel = make(chan notify.EventInfo, 10000)
//
//	// Update the path that traffic is served from to be the filesystem canonical path. This will allow the event folders that come in to match what we have.
//	fullPath := validatePath(directory)
//	handler.directory = fullPath
//
//	if fullPath != globalSettings.Directory {
//		log.Printf("Updating serving directory to: %s", fullPath)
//		handler.directory = fullPath
//	}
//
//	handler.renamesInProgress = make(map[uint64]renameInformation, 0)
//
//	log.Println("Setting up filesystemTracker!")
//	handler.printLockable(false)
//
//	log.Println("FilesystemTracker:init starting folder scan looking for initial files")
//	err := handler.scanFolders()
//	if err != nil {
//		panic(err)
//	}
//
//	// Set the status to be done with initial scan
//	server.SetStatus(REPLICAT_STATUS_JOINING_CLUSTER)
//	handler.printLockable(false)
//	handler.setup = true
//}

//// Notification event object metadata.
//type objectMeta struct {
//	Key       string `json:"key"`
//	Size      int64  `json:"size,omitempty"`
//	ETag      string `json:"eTag,omitempty"`
//	VersionID string `json:"versionId,omitempty"`
//	Sequencer string `json:"sequencer"`
//}
//
//// Notification event server specific metadata.
//type eventMeta struct {
//	SchemaVersion   string     `json:"s3SchemaVersion"`
//	ConfigurationID string     `json:"configurationId"`
//	Bucket          bucketMeta `json:"bucket"`
//	Object          objectMeta `json:"object"`
//}
//
//// NotificationEvent represents an Amazon an S3 bucket notification event.
//type NotificationEvent struct {
//	EventVersion      string            `json:"eventVersion"`
//	EventSource       string            `json:"eventSource"`
//	AwsRegion         string            `json:"awsRegion"`
//	EventTime         string            `json:"eventTime"`
//	EventName         string            `json:"eventName"`
//	UserIdentity      identity          `json:"userIdentity"`
//	RequestParameters map[string]string `json:"requestParameters"`
//	ResponseElements  map[string]string `json:"responseElements"`
//	S3                eventMeta         `json:"s3"`
//}

type MinioEntry struct {
	Name      string `json:"name"`
	Size      int64  `json:"size,omitempty"`
	ETag      string `json:"eTag,omitempty"`
	VersionID string `json:"versionId,omitempty"`
	Sequencer string `json:"sequencer"`
	ModTime   string `json:"modTime"`
}

var MINOIO_TRACKER_NOT_IMPLEMENTED error = errors.New("Replicat: Minio tracker has not implemented this function")

func (tracker *MinioTracker) CreatePath(pathName string, isDirectory bool) (err error) {
	return MINOIO_TRACKER_NOT_IMPLEMENTED
}

func (tracker *MinioTracker) watchBucket() {

	for notificationInfo := range tracker.minioSDK.ListenBucketNotification(tracker.bucketName, "", "", allEvents, tracker.doneCh) {
		if notificationInfo.Err != nil {
			panic(notificationInfo.Err)
		}

		for _, record := range notificationInfo.Records {
			log.Printf("%s: %s bucket: %s object: %s", record.EventTime, record.EventName, record.S3.Bucket.Name, record.S3.Object.Key)
			switch record.EventName {
			case minio.ObjectCreatePut:
				newEntry := MinioEntry{Name: record.S3.Object.Key, Size: record.S3.Object.Size, ETag: record.S3.Object.ETag, ModTime: record.EventTime}
				tracker.lock()
				tracker.contents[record.S3.Object.Key] = newEntry
				tracker.unlock()
			case minio.ObjectRemovedDelete:
				tracker.lock()
				_, exists := tracker.contents[record.S3.Object.Key]
				if exists {
					log.Printf("ObjectRemoved: Existing entry found: %s", record.S3.Object.Key)
					delete(tracker.contents, record.S3.Object.Key)
				} else {
					log.Printf("ObjectRemoved: NO Existing entry found: %s", record.S3.Object.Key)
				}

				tracker.unlock()
			}

		}
		if notificationInfo.Err != nil {
			log.Fatalln(notificationInfo.Err)
		}
	}
}

//todo fix up the miniotracker initialization
func (tracker *MinioTracker) Initialize(bucketName string, server *ReplicatServer) (err error) {
	if tracker.setup == true {
		return
	}

	// Initialize minio client object.
	tracker.minioSDK, err = minio.New(minioAddress, minioPlayAccessKey, minioPlaySecretKey, true)
	if err != nil {
		log.Println(err)
		return
	}

	// Create a done channel to control 'ListenBucketNotification' go routine.
	tracker.doneCh = make(chan struct{})
	tracker.bucketName = bucketName

	// Create the contents
	tracker.contents = make(map[string]MinioEntry, 100)

	// If the bucket does not exist, create it.
	exists, err := tracker.minioSDK.BucketExists(bucketName)
	log.Printf("MinioTracker::Initialize: Bucket: %s Exists: %t Error: %v", bucketName, exists, err)
	if err != nil {
		panic(err.Error())
	}

	if exists == true {
		log.Printf("MinioTracker:Initialize starting object scan in bucket: %s", tracker.bucketName)
		go tracker.watchBucket()

		tracker.scanObjects()
	} else {
		err = tracker.minioSDK.MakeBucket(bucketName, "")
		log.Printf("MinioTracker::Initialize: Attempted to make bucket: %s Error: %v", tracker.bucketName, err)
		if err != nil {
			panic(err.Error())
		}

		log.Printf("MinioTracker::Initialize: About to start watching bucket: %s Error: %v", tracker.bucketName, err)
		go tracker.watchBucket()
	}

	log.Printf("MinioTracker::Initialize: About to start watching bucket: %s Error: %v", tracker.bucketName, err)

	// Set the status to be done with initial scan
	server.SetStatus(REPLICAT_STATUS_JOINING_CLUSTER)
	tracker.printLockable(false)
	tracker.setup = true

	return
}

func (tracker *MinioTracker) scanObjects() {
	log.Printf("Scanning objects in bucket: %s", tracker.bucketName)
	doneCh := make(chan struct{})
	defer close(doneCh)

	tracker.fsLock.Lock()
	defer tracker.fsLock.Unlock()

	//type MinioEntry struct {
	//	Key       string `json:"key"`
	//	Size      int64  `json:"size,omitempty"`
	//	ETag      string `json:"eTag,omitempty"`
	//	VersionID string `json:"versionId,omitempty"`
	//	Sequencer string `json:"sequencer"`
	//	ModTime   string `json:"modTime"`
	//}

	// Recursively list all objects in tracker.bucketName
	log.Printf("About to list all of the objects from S3. Current contents:\n%#v", tracker.contents)
	for message := range tracker.minioSDK.ListObjectsV2(tracker.bucketName, "", true, doneCh) {
		log.Println(message)
		var jsonValues []byte
		jsonValues, err := json.Marshal(message)
		if err != nil {
			panic(err.Error())
		}

		log.Printf("Current item is: %s", jsonValues)

		var test MinioEntry
		json.Unmarshal(jsonValues, &test)

		log.Printf("This is the key field from the entry: %s", test.Name)
		log.Printf("Before Setting: %s to %v Length: %d Current contents:\n%#v", test.Name, test, len(tracker.contents), tracker.contents)
		tracker.contents[test.Name] = test
		log.Printf("After  Setting: %s to %v Length: %d Current contents:\n%#v", test.Name, test, len(tracker.contents), tracker.contents)
	}
	log.Printf("About to list all of the objects from S3. Current contents:\n%#v", tracker.contents)

}

func (tracker *MinioTracker) verifyInitialized() (err error) {
	if tracker.setup == false {
		panic("verifyInitialized called when the object was not properly initialized.")
	}

	return
}

//func (tracker *MinioTracker) getEntryJSON(relativePath string) (entry EntryJSON, err error) {
//	type EntryJSON struct {
//		RelativePath string
//		IsDirectory  bool
//		Hash         []byte
//		ModTime      time.Time
//		Size         int64
//		ServerName   string

func (tracker *MinioTracker) CreateObject(bucketName, objectName, sourceFile, contentType string) (err error) {
	//todo rectify this. Filesystem tracker assumes the base directory of the tracker for source file. Not cool.
	//	dir, err := os.Getwd()
	//	log.Printf("Minio Tracker in Create object. sourceFile is: %s cwd is: %s ", sourceFile, dir)
	log.Printf("Minio Tracker in Create object. sourceFile is: %s", sourceFile)

	info, err := os.Stat(sourceFile)
	if err != nil {
		panic(err.Error())
	}

	size, err := tracker.minioSDK.FPutObject(bucketName, objectName, sourceFile, contentType)
	if err != nil {
		panic(err.Error())
	}

	if info.Size() != size {
		log.Println("Size missmatch")
	}

	return
}

//func (tracker *MinioTracker) CreatePath(pathName string, isDirectory bool) (err error) {
//	err = tracker.verifyInitialized()
//	if err != nil {
//		panic(err)
//	}
//
//	if isDirectory == false {
//		panic("Not implemented yet")
//	}
//
//	err = tracker.minioSDK.MakeBucket(pathName, "")
//	if err != nil {
//		log.Println(err)
//		return
//	}
//
//	return
//}

func (tracker *MinioTracker) Rename(sourcePath string, destinationPath string, isDirectory bool) (err error) {

	return
}

func (tracker *MinioTracker) RenameObject(bucketName string, objectName string, sourceObjectName string) (err error) {
	log.Printf("MinioTracker::Rename object dest bucket: %s dest name: %s, source: %s", bucketName, objectName, sourceObjectName)
	err = tracker.verifyInitialized()
	if err != nil {
		panic(err)
	}

	err = tracker.minioSDK.CopyObject(bucketName, objectName, sourceObjectName, minio.NewCopyConditions())
	if err != nil {
		panic(err)
	}

	sourceBucketName, sourceObjectName, err := splitBucketObjectName(sourceObjectName)
	if err != nil {
		panic(err)
	}

	err = tracker.minioSDK.RemoveObject(sourceBucketName, sourceObjectName)
	if err != nil {
		panic(err)
	}

	return
}

// splitBucketObjectNames - Take a bucket followed by a separator and an object name and split it into two components
// Valid bucket names must be at least one character long. Valid path names must be at least one character long
func splitBucketObjectName(bucketObjectName string) (bucketName string, objectName string, err error) {
	// find the first instance of a path separator
	splitResults := strings.SplitN(bucketObjectName, "/", 2)
	if len(splitResults) != 2 || len(splitResults[0]) == 0 || len(splitResults[1]) == 0 {
		err = errors.New(fmt.Sprintf("MinioTracker:splitBucketObjectNames panic Could not separate (%s) bucket info from object name. Source: %s output: %#v", string(os.PathSeparator), bucketObjectName, splitResults))
		return
	}

	bucketName = splitResults[0]
	objectName = splitResults[1]
	return splitResults[0], splitResults[1], err
}

// joinBucketNames - Take a bucket name and object name and combine them into one string
func joinBucketObjectName(bucketName string, objectName string) (bucketObjectName string, err error) {
	if len(bucketName) < 1 || len(objectName) < 1 {
		err = errors.New(fmt.Sprintf("MinioTracker:joinBucketObjectName panic Could not join bucket: %s object: %s", bucketName, objectName))
		return
	}

	bucketObjectName = bucketName + "/" + objectName
	return
}

func (tracker *MinioTracker) DeleteObject(bucket, name string) (err error) {
	log.Printf("MinioTracker::DeleteObject bucket: %s, name: %s", bucket, name)
	err = tracker.verifyInitialized()
	if err != nil {
		panic(err)
	}

	err = tracker.minioSDK.RemoveObject(bucket, name)
	if err != nil {
		panic(err)
	}
	return
}

func (tracker *MinioTracker) DeleteFolder(name string) (err error) {
	err = tracker.verifyInitialized()
	if err != nil {
		panic(err)
	}

	err = tracker.minioSDK.RemoveBucket(name)
	if err != nil {
		log.Println(err)
		return
	}

	// todo add support for this to be either a bucket or an object based on the metadata
	return
}

func (tracker *MinioTracker) ListFolders(getLocks bool) (folderList []string, err error) {
	err = tracker.verifyInitialized()
	if err != nil {
		panic(err)
	}

	folderList = make([]string, 0)
	for k := range tracker.contents {
		folderList = append(folderList, k)
	}

	log.Printf("Items returning from ListFolders: %d\n%#v", len(folderList), folderList)
	return
}

func (tracker *MinioTracker) SendCatalog() {
	return
}

func (tracker *MinioTracker) ProcessCatalog(event Event) {
	return
}

func (tracker *MinioTracker) sendRequestedPaths(pathEntries map[string]EntryJSON, targetServerName string) {
	return
}

func (tracker *MinioTracker) getEntryJSON(relativePath string) (entry EntryJSON, err error) {
	return
}

func (tracker *MinioTracker) GetStatistics() (stats map[string]string) {
	return
}

func (tracker *MinioTracker) IncrementStatistic(name string, delta int, getLocks bool) {
	return
}

//// IncrementStatistic - Increment one of the named statistics on the tracker.
//func (handler *FilesystemTracker) IncrementStatistic(name string, delta int) {
//	go func() {
//		log.Printf("FilesystemTracker:IncrementStatistic(name %s, delta %d)", name, delta)
//		handler.fsLock.Lock()
//		log.Println("FilesystemTracker:/IncrementStatistic")
//		defer handler.fsLock.Unlock()
//		defer log.Println("FilesystemTracker://IncrementStatistic")
//
//		switch name {
//		case TRACKER_TOTAL_FILES:
//			handler.stats.TotalFiles += delta
//			break
//		case TRACKER_TOTAL_FOLDERS:
//			handler.stats.TotalFolders += delta
//			break
//		case TRACKER_FILES_SENT:
//			handler.stats.FilesSent += delta
//			break
//		case TRACKER_FILES_RECEIVED:
//			handler.stats.FilesReceived += delta
//			break
//		case TRACKER_FILES_DELETED:
//			handler.stats.FilesDeleted += delta
//			break
//		case TRACKER_CATALOGS_SENT:
//			handler.stats.CatalogsSent += delta
//			break
//		case TRACKER_CATALOGS_RECEIVED:
//			handler.stats.CatalogsReceived += delta
//			break
//		}
//	}()
//}

func (tracker *MinioTracker) printLockable(lock bool) {
	if lock {
		log.Println("MinioTracker:print")
		tracker.fsLock.RLock()
		log.Println("MinioTracker:/print")
		defer tracker.fsLock.RUnlock()
		defer log.Println("MinioTracker://print")
	}

	folders := make([]string, 0, len(tracker.contents))
	for dir := range tracker.contents {
		folders = append(folders, dir)
	}
	sort.Strings(folders)

	log.Println("~~~~~~~~~~~~~~~~~~~~~~~")
	log.Printf("~~~~%s Minio Tracker report setup(%v)", tracker.bucketName, tracker.setup)
	log.Printf("~~~~contents: %v", tracker.contents)
	log.Printf("~~~~folders: %v", folders)
	log.Println("~~~~~~~~~~~~~~~~~~~~~~~")
}

func (tracker *MinioTracker) rlock() {
	log.Println("MinioTracker:rlock before")
	tracker.fsLock.RLock()
	log.Println("MinioTracker:rlock after")
}

func (tracker *MinioTracker) lock() {
	log.Println("MinioTracker:lock before")
	tracker.fsLock.Lock()
	log.Println("MinioTracker:lock after")
}

func (tracker *MinioTracker) runlock() {
	log.Println("MinioTracker:runlock before")
	tracker.fsLock.RUnlock()
	log.Println("MinioTracker:runlock after")
}

func (tracker *MinioTracker) unlock() {
	log.Println("MinioTracker:unlock before")
	tracker.fsLock.Unlock()
	log.Println("MinioTracker:unlock after")
}

//func (handler *FilesystemTracker) validate() {
//	handler.fsLock.RLock()
//	defer handler.fsLock.RUnlock()
//	log.Println("~~~~~~~~~~~~~~~~~~~~~~~")
//	log.Printf("~~~~%s Tracker validation starting on %d folders", handler.directory, len(handler.contents))
//
//	for name, dir := range handler.contents {
//		if !dir.setup {
//			panic(log.Sprintf("tracker validation failed on directory: %s", name))
//		}
//	}
//
//	log.Println("~~~~~~~~~~~~~~~~~~~~~~~")
//}
//
//// GetStatistics - Return the tracked statistics for the replicat node.
//func (handler *FilesystemTracker) GetStatistics() (stats map[string]string) {
//	stats = make(map[string]string, 7)
//	stats[TRACKER_TOTAL_FILES] = strconv.Itoa(handler.stats.TotalFiles)
//	stats[TRACKER_TOTAL_FOLDERS] = strconv.Itoa(handler.stats.TotalFolders)
//	stats[TRACKER_FILES_SENT] = strconv.Itoa(handler.stats.FilesSent)
//	stats[TRACKER_FILES_RECEIVED] = strconv.Itoa(handler.stats.FilesReceived)
//	stats[TRACKER_FILES_DELETED] = strconv.Itoa(handler.stats.FilesDeleted)
//	stats[TRACKER_CATALOGS_SENT] = strconv.Itoa(handler.stats.CatalogsSent)
//	stats[TRACKER_CATALOGS_RECEIVED] = strconv.Itoa(handler.stats.CatalogsReceived)
//
//	address := serverMap[globalSettings.Name].Address
//
//	// Build a list of the entire cluster. Make that list into a string for printing out later
//	var cluster string
//	serverMapLock.RLock()
//	for _, v := range serverMap {
//		cluster += log.Sprintf("name: %s\taddress: %s", v.Name, v.Address)
//	}
//
//	serverMapLock.RUnlock()
//
//	log.Printf("Address: %s\tFiles: %d\tFolders:%d\tFiles Sent: %d\tReceived %d\tDeleted: %d\tCatalogs Sent: %d\tReceived: %d\n%s", address, handler.stats.TotalFiles, handler.stats.TotalFolders, handler.stats.FilesSent, handler.stats.FilesReceived, handler.stats.FilesDeleted, handler.stats.CatalogsSent, handler.stats.CatalogsReceived, cluster)
//
//	return
//}
//

func (tracker *MinioTracker) cleanupAndDelete() {
	log.Println("MinioTracker:cleanup")
	tracker.fsLock.Lock()
	defer tracker.fsLock.Unlock()
	log.Println("MinioTracker:/cleanup")
	defer log.Println("MinioTracker://cleanup")

	if !tracker.setup {
		panic("cleanup called when not yet setup")
	}

	//todo do we tell Minio that we are no longer watching for changes?
	// delete the bucket we created for tracking
	//todo clean this up to handle multiple buckets
	tracker.DeleteFolder(tracker.bucketName)
}

//func (handler *FilesystemTracker) watchDirectory(watcher *ChangeHandler) {
//	handler.fsLock.Lock()
//	defer handler.fsLock.Unlock()
//
//	if !handler.setup {
//		panic("FilesystemTracker:watchDirectory called when not yet setup")
//	}
//
//	if handler.watcher != nil {
//		panic("watchDirectory called a second time. Not allowed")
//	}
//
//	handler.watcher = watcher
//
//	go handler.monitorLoop(handler.fsEventsChannel)
//
//	// Set up a watch point listening for events within a directory tree rooted at the specified folder
//	err := notify.Watch(handler.directory+"/...", handler.fsEventsChannel, notify.All)
//	if err != nil {
//		log.Panic(err)
//	}
//}
//
//func validatePath(directory string) (fullPath string) {
//	fullPath, err := filepath.EvalSymlinks(directory)
//	if err != nil {
//		var err2, err3 error
//		// We have an error. If the directory does not exist, then try to create it. Fail if we cannot create it and return the original error
//		if os.IsNotExist(err) {
//			err2 = os.Mkdir(directory, os.ModeDir+os.ModePerm)
//			fullPath, err3 = filepath.EvalSymlinks(directory)
//			if err2 != nil || err3 != nil {
//				panic(log.Sprintf("err: %v\nerr2: %v\nerr3: %v", err, err2, err3))
//			}
//		} else {
//			panic(err)
//		}
//	}
//
//	return
//}
//
//func createPath(pathName string, absolutePathName string) (pathCreated bool, stat os.FileInfo, err error) {
//	for maxCycles := 0; maxCycles < 20 && pathName != ""; maxCycles++ {
//		stat, err = os.Stat(absolutePathName)
//
//		if err == nil {
//			log.Printf("Path existed: %s", absolutePathName)
//			pathCreated = true
//			return
//		}
//
//		// if there is an error, go to create the path
//		log.Printf("Creating path: %s", absolutePathName)
//		err = os.MkdirAll(absolutePathName, os.ModeDir+os.ModePerm)
//
//		// after attempting to create the path, check the err again
//		if err == nil {
//			log.Printf("Path was created or existed: %s", absolutePathName)
//			pathCreated = true
//			stat, err = os.Stat(absolutePathName)
//			log.Printf("os.Stat returned stat: %#v, err: %#v", stat, err)
//			return
//		} else if os.IsExist(err) {
//			log.Printf("Path already exists: %s", absolutePathName)
//			pathCreated = true
//			stat, err = os.Stat(absolutePathName)
//			return
//		}
//
//		log.Printf("Error (%v) encountered creating path, going to try again. Attempt: %d", err, maxCycles)
//		time.Sleep(20 * time.Millisecond)
//	}
//
//	pathCreated = false
//	return pathCreated, stat, err
//}
//
//func (handler *FilesystemTracker) sendRequestedPaths(pathEntries map[string]EntryJSON, targetServerName string) {
//	serverAddress := serverMap[targetServerName].Address
//	currentPath := globalSettings.Directory
//
//	handler.fsLock.RLock()
//	defer handler.fsLock.RUnlock()
//
//	for p, entry := range pathEntries {
//		fullPath := filepath.Join(currentPath, p)
//		realEntry := handler.contents[p]
//		log.Printf("File info for: %s\nProvided: %#v\nFile info for: %s\nStorege : %#v", p, entry, p, realEntry)
//		if !entry.IsDirectory {
//			log.Printf("Requested file information: %s\n%#v", p, entry)
//			go postHelper(p, fullPath, serverAddress, globalSettings.ManagerCredentials)
//		}
//	}
//}
//
//func (handler *FilesystemTracker) getEntryJSON(relativePath string) (EntryJSON, error) {
//	handler.fsLock.RLock()
//	defer handler.fsLock.RUnlock()
//
//	// get the current entry
//	currentEntry, exists := handler.contents[relativePath]
//	if exists == false {
//		return EntryJSON{}, errors.New("File Does Not Exist")
//	}
//
//	if currentEntry.setup == false {
//		return EntryJSON{}, errors.New("File information is unset")
//	}
//
//	result := EntryJSON{RelativePath: relativePath,
//		IsDirectory: currentEntry.IsDir(),
//		Hash:        currentEntry.hash,
//		ModTime:     currentEntry.ModTime(),
//		Size:        currentEntry.Size(),
//		ServerName:  globalSettings.Name}
//
//	return result, nil
//}
//
//// createPath implements the new path/file creation. Locking is done outside this call.
//func (handler *FilesystemTracker) createPath(pathName string, isDirectory bool) (err error) {
//	relativePathName := pathName
//	file := ""
//
//	if !isDirectory {
//		relativePathName, file = filepath.Split(pathName)
//	}
//
//	//log.Printf("Path name before any adjustments (directory: %v)\npath: %s\nfile: %s", isDirectory, relativePathName, file)
//	if len(relativePathName) > 0 && relativePathName[len(relativePathName)-1] == filepath.Separator {
//		log.Println("Stripping out path ending")
//		relativePathName = relativePathName[:len(relativePathName)-1]
//	}
//
//	absolutePathName := filepath.Join(handler.directory, relativePathName)
//	log.Printf("**********\npath was split into \nrelativePathName: %s \nfile: %s \nabsolutePath: %s\n**********", relativePathName, file, absolutePathName)
//
//	pathCreated, stat, err := createPath(pathName, absolutePathName)
//
//	if err != nil && !os.IsExist(err) {
//		panic(log.Sprintf("Error creating folder %s: %v", relativePathName, err))
//	}
//
//	if pathCreated {
//		handler.contents[relativePathName] = *NewDirectoryFromFileInfo(&stat)
//	}
//
//	if !isDirectory {
//		completeAbsoluteFilePath := filepath.Join(handler.directory, pathName)
//		log.Printf("We are creating a file at: %s", completeAbsoluteFilePath)
//
//		// We also need to create the file.
//		for maxCycles := 0; maxCycles < 5; maxCycles++ {
//			stat, err = os.Stat(completeAbsoluteFilePath)
//			log.Printf("Stat call done for\npath: %s\nerr: %v\nstat: %v", completeAbsoluteFilePath, err, stat)
//			var newFile *os.File
//
//			// if there is an error, go to create the file
//			log.Println("before")
//			if err != nil {
//				log.Printf("Creating file: %s", completeAbsoluteFilePath)
//				newFile, err = os.Create(completeAbsoluteFilePath)
//				log.Printf("Attempt to create file finished\n err: %v\n path: %s", err, completeAbsoluteFilePath)
//			}
//			log.Println("after")
//
//			// after attempting to create the file, check the err again
//			if err == nil {
//				newFile.Close()
//				stat, err = os.Stat(completeAbsoluteFilePath)
//				log.Printf("file was created or existed: %s", completeAbsoluteFilePath)
//				break
//			}
//
//			log.Printf("Error (%v) encountered creating file, going to try again. Attempt: %d", err, maxCycles)
//			time.Sleep(20 * time.Millisecond)
//		}
//
//		if err != nil {
//			panic(log.Sprintf("Error creating file %s: %v", completeAbsoluteFilePath, err))
//		}
//
//		handler.contents[relativePathName] = *NewDirectoryFromFileInfo(&stat)
//	}
//
//	return
//}
//
//// CreatePath tells the storage tracker to create a new path
//func (handler *FilesystemTracker) CreatePath(pathName string, isDirectory bool) error {
//	log.Printf("FilesystemTracker:CreatePath called with relativePath: %s isDirectory: %v", pathName, isDirectory)
//	handler.fsLock.Lock()
//	log.Println("FilesystemTracker:/CreatePath")
//	defer handler.fsLock.Unlock()
//	defer log.Println("FilesystemTracker://CreatePath")
//
//	if !handler.setup {
//		panic("FilesystemTracker:CreatePath called when not yet setup")
//	}
//
//	return handler.createPath(pathName, isDirectory)
//}
//
//func (handler *FilesystemTracker) handleCompleteRename(sourcePath string, destinationPath string, isDirectory bool) (err error) {
//
//	// First we do the simple rename where both sides are here.
//	absoluteSourcePath := filepath.Join(handler.directory, sourcePath)
//	absoluteDestinationPath := filepath.Join(handler.directory, destinationPath)
//
//	for maxCycles := 0; maxCycles < 5; maxCycles++ {
//		err = os.Rename(absoluteSourcePath, absoluteDestinationPath)
//
//		// after attempting to create the path, check the err again
//		if err == nil {
//			log.Println("Rename Complete")
//			break
//		}
//
//		log.Printf("Error (%v) encountered moving path, going to try again. Attempt: %d", err, maxCycles)
//		time.Sleep(20 * time.Millisecond)
//	}
//
//	if err != nil {
//		panic(log.Sprintf("Rename failed (%v)!  source: %s dest: %s directory %v", err, sourcePath, destinationPath, isDirectory))
//	}
//
//	return err
//}
//
//// Rename - Rename a file or folder from one location to another
//func (handler *FilesystemTracker) Rename(sourcePath string, destinationPath string, isDirectory bool) (err error) {
//	log.Println("FilesystemTracker:Rename")
//	handler.fsLock.Lock()
//	defer handler.fsLock.Unlock()
//	log.Println("FilesystemTracker:/Rename")
//	defer log.Println("FilesystemTracker://Rename")
//
//	log.Printf("We have a rename event. source: %s dest: %s directory %v", sourcePath, destinationPath, isDirectory)
//
//	if !handler.setup {
//		panic("FilesystemTracker:CreatePath called when not yet setup")
//	}
//
//	// If we have a source and destination, perform the move to mirror the other side
//	if len(sourcePath) > 0 && len(destinationPath) > 0 {
//		log.Println("FilesystemTracker:Rename completing")
//		err = handler.handleCompleteRename(sourcePath, destinationPath, isDirectory)
//	} else if sourcePath == "" {
//		log.Println("FilesystemTracker:Rename creating a new path")
//		// If the file was moved into the monitored folder from nowhere, ...
//		// bypass the locking by using the internal method since we already have the lockings and safety checks
//		err = handler.createPath(destinationPath, isDirectory)
//	} else if destinationPath == "" {
//		log.Println("FilesystemTracker:Rename deleting existing path")
//		// If the file or folder was moved out of the monitored folder, get rid of it.
//		delete(handler.contents, sourcePath)
//		// todo - shortcut the events on this one. Should create a bit of a storm.
//		absolutePathForDeletion := filepath.Join(handler.directory, sourcePath)
//		log.Printf("About to call os.RemoveAll on: %s", absolutePathForDeletion)
//		err = os.RemoveAll(absolutePathForDeletion)
//		if err != nil {
//			panic(log.Sprintf("%v encountered when attempting to os.RemoveAll(%s)", err, absolutePathForDeletion))
//		}
//	} else {
//		panic("Enexpected case encountered in rename")
//	}
//
//	log.Printf("Rename Complete source: %s dest: %s directory %v", sourcePath, destinationPath, isDirectory)
//	return
//}
//
//// DeleteFolder - This storage handler should remove the specified path
//func (handler *FilesystemTracker) DeleteFolder(name string) error {
//	log.Println("FilesystemTracker:DeleteFolder")
//	handler.fsLock.Lock()
//	defer handler.fsLock.Unlock()
//	log.Println("FilesystemTracker:/DeleteFolder")
//	defer log.Println("FilesystemTracker://DeleteFolder")
//
//	if !handler.setup {
//		panic("FilesystemTracker:DeleteFolder called when not yet setup")
//	}
//
//	log.Printf("DeleteFolder: '%s'", name)
//	delete(handler.contents, name)
//	log.Printf("%d after delete of: %s", len(handler.contents), name)
//
//	return nil
//}
//
//// ListFolders - This storage handler should return a list of contained folders.
//func (handler *FilesystemTracker) ListFolders(getLocks bool) (folderList []string) {
//	log.Printf("FilesystemTracker:ListFolders getLocks: %v", getLocks)
//	if getLocks {
//		handler.fsLock.Lock()
//		defer handler.fsLock.Unlock()
//		log.Println("FilesystemTracker:/ListFolders")
//		defer log.Println("FilesystemTracker://ListFolders")
//	}
//
//	if !handler.setup {
//		panic("FilesystemTracker:ListFolders called when not yet setup")
//	}
//
//	folderList = make([]string, len(handler.contents))
//	index := 0
//
//	for k := range handler.contents {
//		folderList[index] = k
//		index++
//	}
//
//	sort.Strings(folderList)
//	return
//}
//
//func extractPaths(handler *FilesystemTracker, ei *notify.EventInfo) (path, fullPath string) {
//	directoryLength := len(handler.directory)
//	fullPath = string((*ei).Path())
//
//	path = fullPath
//	if len(fullPath) >= directoryLength && handler.directory == fullPath[:directoryLength] {
//		if len(fullPath) == directoryLength {
//			return "", ""
//		}
//
//		// update the path to not have this prefix
//		path = fullPath[directoryLength+1:]
//	}
//
//	return
//}
//
//// Monitor the filesystem looking for changes to files we are keeping track of.
//func (handler *FilesystemTracker) monitorLoop(c chan notify.EventInfo) {
//	// filesToIgnore - If you run into one of these files, do not sync it to other side.
//	filesToIgnore := map[string]bool{
//		".DS_Store": true,
//		"Thumbs.db": true,
//	}
//
//	for {
//		ei := <-c
//
//		log.Printf("*****We have an event: %v\nwith Sys: %v\npath: %v\nevent: %v", ei, ei.Sys(), ei.Path(), ei.Event())
//
//		path, fullPath := extractPaths(handler, &ei)
//		// Skip empty paths
//		if path == "" {
//			log.Println("blank path. Ignore!")
//			continue
//		}
//
//		//.DS_Store
//		// split out the filename and check to see if it is on the ignore list.
//		_, testFile := filepath.Split(ei.Path())
//		_, exists := filesToIgnore[testFile]
//		if exists {
//			log.Printf("Ignoring event for file on ignore list: %s - %s", ei.Event(), testFile)
//			continue
//		}
//
//		event := Event{Name: ei.Event().String(), Path: path, Source: globalSettings.Name}
//		log.Printf("Event captured name: %s location: %s, ei.Path(): %s", event.Name, event.Path, ei.Path())
//
//		isDirectory := handler.checkIfDirectory(event, path, fullPath)
//		event.IsDirectory = isDirectory
//		handler.processEvent(event, path, fullPath, true)
//	}
//}
//
//func (handler *FilesystemTracker) checkIfDirectory(event Event, path, fullPath string) bool {
//	log.Println("FilesystemTracker:checkIfDirectory")
//	handler.fsLock.Lock()
//	defer handler.fsLock.Unlock()
//	log.Println("FilesystemTracker:/checkIfDirectory")
//	defer log.Println("FilesystemTracker://checkIfDirectory")
//
//	// Check to see if this was a path we knew about
//	_, isDirectory := handler.contents[path]
//	var iNode uint64
//	// if we have not found it yet, check to see if it can be stated
//	info, err := os.Stat(fullPath)
//	if err == nil {
//		isDirectory = info.IsDir()
//		sysInterface := info.Sys()
//		log.Printf("sysInterface: %v", sysInterface)
//		if sysInterface != nil {
//			foo := sysInterface.(*syscall.Stat_t)
//			iNode = foo.Ino
//		}
//	}
//
//	log.Printf("checkIfDirectory: event raw data: %s with path: %s fullPath: %s isDirectory: %v iNode: %v", event.Name, path, fullPath, isDirectory, iNode)
//
//	return isDirectory
//}
//
//type renameInformation struct {
//	iNode           uint64
//	sourceDirectory *Entry
//	sourcePath      string
//	sourceSet       bool
//	destinationStat os.FileInfo
//	destinationPath string
//	destinationSet  bool
//}
//
//func getiNodeFromStat(stat os.FileInfo) uint64 {
//	log.Printf("getiNodeFromStat called with %#v", stat)
//	if stat == nil {
//		return 0
//	}
//
//	sysInterface := stat.Sys()
//	if sysInterface != nil {
//		foo := sysInterface.(*syscall.Stat_t)
//		return foo.Ino
//	}
//	return 0
//}
//
//const (
//	// TRACKER_RENAME_TIMEOUT - the amount of time to sleep while waiting for the second event in a rename process
//	TRACKER_RENAME_TIMEOUT = time.Millisecond * 250
//)
//
//// WaitForFilesystem - a helper function that will politely wait until the helper argument function evaluates to the value of waitingFor.
//// this is great for waiting for the tracker to catch up to the filesystem in tests, for example.
//func WaitForFilesystem(tracker *FilesystemTracker, folder string, waitingFor bool, helper func(tracker *FilesystemTracker, folder string) bool) bool {
//	roundTrips := 0
//	for {
//		if waitingFor == helper(tracker, folder) {
//			return true
//		}
//
//		if roundTrips > 50 {
//			return false
//		}
//		roundTrips++
//
//		time.Sleep(50 * time.Millisecond)
//	}
//}
//
//// completeRenameIfAbandoned - if there is a rename that was started with a source
//// but has been left pending for more than TRACKER_RENAME_TIMEOUT, complete it (i.e. move the folder away)
//func (handler *FilesystemTracker) completeRenameIfAbandoned(iNode uint64) {
//	time.Sleep(TRACKER_RENAME_TIMEOUT)
//
//	handler.fsLock.Lock()
//	defer handler.fsLock.Unlock()
//
//	inProgress, exists := handler.renamesInProgress[iNode]
//	if !exists {
//		log.Printf("Rename for iNode %d appears to have been completed", iNode)
//		return
//	}
//
//	// We have the inProgress. Clean it up
//	if inProgress.sourceSet {
//		relativePath := inProgress.sourcePath[len(handler.directory)+1:]
//		log.Printf("File at: %s (iNode %d) appears to have been moved away. Removing it", relativePath, iNode)
//		delete(handler.contents, relativePath)
//
//		// tell the other nodes that a rename was done.
//		event := Event{Name: "replicat.Rename", Source: globalSettings.Name, SourcePath: relativePath}
//		SendEvent(event, inProgress.sourcePath)
//	} else if inProgress.destinationSet {
//		log.Printf("directory: %s src: %s dest: %s", handler.directory, inProgress.sourcePath, inProgress.destinationPath)
//		log.Printf("inProgress: %v", handler.renamesInProgress)
//		log.Printf("this inProgress: %v", inProgress)
//
//		//this code failed because of an slice index out of bounds error. It is a reasonable copy with both sides and
//		//yet it was initialized blank. The first event was for the copy and it should have existed. Ahh, it is a file
//		if len(inProgress.destinationPath) < len(handler.directory)+1 {
//			log.Print("About to pop.....")
//		}
//
//		relativePath := inProgress.destinationPath[len(handler.directory)+1:]
//		handler.contents[relativePath] = *NewDirectoryFromFileInfo(&inProgress.destinationStat)
//
//		// tell the other nodes that a rename was done.
//		event := Event{Name: "replicat.Rename", Source: globalSettings.Name, Path: relativePath, ModTime: inProgress.destinationStat.ModTime(),
//			IsDirectory: inProgress.destinationStat.IsDir()}
//		SendEvent(event, inProgress.destinationPath)
//
//		// find the source if the destination is an iNode in our system
//		// This is probably expensive. :) Wait until there is a flag of a cleanup
//		// We also have to watch out for hardlinks if those would share iNodes.
//		//for name, directory := range handler.contents {
//		//	directoryID := getiNodeFromStat(directory)
//		//	if directoryID == iNode {
//		//		log.Println("found the source file, using it!")
//		//		handler.contents[relativePath] = directory
//		//		delete(handler.contents, name)
//		//		return
//		//	}
//		//}
//	} else {
//		log.Printf("FilesystemTracker:completeRenameIfAbandoned In progress item that appears to be not set. iNode %d, inProgress: %#v", iNode, inProgress)
//	}
//
//	delete(handler.renamesInProgress, iNode)
//}
//
//func (handler *FilesystemTracker) handleNotifyRename(event Event, pathName, fullPath string) (err error) {
//	log.Printf("FilesystemTracker:handleNotifyRename: pathname: %s fullPath: %s", pathName, fullPath)
//	// Either the path should exist in the filesystem or it should exist in the stored tree. Find it.
//
//	// check to see if this folder currently exists. If it does, it is the destination
//	tmpDestinationStat, err := os.Stat(fullPath)
//	tmpDestinationSet := false
//	if err == nil {
//		tmpDestinationSet = true
//	}
//
//	// Check to see if we have source information for this folder, if we do, it is the source.
//	tmpSourceDirectory, tmpSourceSet := handler.contents[pathName]
//
//	//todo can it be possible to get real values in error? i.e. can I move to an existing folder and pick up its information accidentally?
//	var iNode uint64
//
//	if tmpDestinationSet {
//		iNode = getiNodeFromStat(tmpDestinationStat)
//	} else if tmpSourceSet && tmpSourceDirectory.setup {
//		//log.Printf("About to get iNodefromStat. Handler: %#v", handler)
//		//log.Printf("tmpSourceDirectory: %#v", tmpSourceDirectory)
//		iNode = getiNodeFromStat(tmpSourceDirectory)
//	}
//
//	inProgress, _ := handler.renamesInProgress[iNode]
//
//	if !inProgress.sourceSet && tmpSourceSet {
//		inProgress.sourceDirectory = &tmpSourceDirectory
//		inProgress.sourcePath = fullPath
//		inProgress.sourceSet = true
//		log.Printf("^^^^^^^Source found, deleting pathName '%s' from contents. Current transfer is: %v", pathName, inProgress)
//		delete(handler.contents, pathName)
//	}
//
//	if !inProgress.destinationSet && tmpDestinationSet {
//		inProgress.destinationStat = tmpDestinationStat
//		inProgress.destinationPath = fullPath
//		inProgress.destinationSet = true
//	}
//
//	log.Printf("^^^^^^^Current transfer is: %#v", inProgress)
//
//	if inProgress.destinationSet && inProgress.sourceSet {
//		log.Printf("directory: %s src: %s dest: %s", handler.directory, inProgress.sourcePath, inProgress.destinationPath)
//
//		relativeDestination := inProgress.destinationPath[len(handler.directory)+1:]
//		relativeSource := inProgress.sourcePath[len(handler.directory)+1:]
//		log.Printf("moving from source: %s (%s) to destination: %s (%s)", inProgress.sourcePath, relativeSource, inProgress.destinationPath, relativeDestination)
//
//		handler.contents[relativeDestination] = *NewDirectoryFromFileInfo(&inProgress.destinationStat)
//		delete(handler.contents, relativeSource)
//		delete(handler.renamesInProgress, iNode)
//
//		// tell the other nodes that a rename was done.
//		event := Event{Name: "replicat.Rename", Path: relativeDestination, Source: globalSettings.Name, SourcePath: relativeSource}
//		// todo - verify relativeDestination is the right thing to send here
//		SendEvent(event, relativeDestination)
//
//	} else {
//		log.Printf("^^^^^^^We do not have both a source and destination - schedule and save under iNode: %d Current transfer is: %#v", iNode, inProgress)
//		inProgress.iNode = iNode
//		handler.renamesInProgress[iNode] = inProgress
//		go handler.completeRenameIfAbandoned(iNode)
//	}
//
//	return
//}
//
//func (handler *FilesystemTracker) handleNotifyCreate(event Event, pathName, fullPath string) (err error) {
//	currentValue, exists := handler.contents[pathName]
//
//	log.Printf("processEvent: About to assign from one path to the next. \n\tOriginal: %v \n\tEvent: %v", currentValue, event)
//	// make sure there is an entry in the DirTreeMap for this folder. Since an empty list will always be returned, we can use that
//	if !exists {
//		info, err := os.Stat(fullPath)
//		if err != nil {
//			log.Printf("Could not get stats on directory %s", fullPath)
//			return TRACKER_ERROR_NO_STATS
//		}
//		directory := NewDirectory()
//		directory.FileInfo = info
//		handler.contents[pathName] = *directory
//	}
//
//	updatedValue, exists := handler.contents[pathName]
//
//	if handler.watcher != nil {
//		if event.IsDirectory {
//			(*handler.watcher).FolderCreated(pathName)
//		} else {
//			(*handler.watcher).FileCreated(pathName)
//		}
//	}
//
//	log.Printf("notify.Create: Updated value for %s: %v (%t)", pathName, updatedValue, exists)
//
//	// sendEvent to manager
//	go SendEvent(event, fullPath)
//
//	return
//}
//
//func (handler *FilesystemTracker) handleNotifyRemove(event Event, pathName, fullPath string) (err error) {
//	_, exists := handler.contents[pathName]
//
//	delete(handler.contents, pathName)
//
//	if handler.watcher != nil && exists {
//		(*handler.watcher).FolderDeleted(pathName)
//	} else {
//		log.Println("In the notify.Remove section but did not see a watcher")
//	}
//
//	go SendEvent(event, "")
//
//	log.Printf("notify.Remove: %s (%t)", pathName, exists)
//	return
//}
//
//func (handler *FilesystemTracker) handleNotifyWrite(event Event, pathName, fullPath string) (err error) {
//	log.Printf("File Write detected: %v", event)
//	if handler.watcher != nil {
//		if event.IsDirectory {
//			(*handler.watcher).FolderUpdated(pathName)
//		} else {
//			(*handler.watcher).FileUpdated(pathName)
//		}
//	}
//
//	go SendEvent(event, fullPath)
//	return
//}
//
////Create pile of folders with other servers offline
////stop C
////Duplicate all of C
////Start C
////Remove all of the duplicates in C
////C says, we do not own this, do not send
////2017/03/06 23:36:20.917030 server.go:88: Original ownership: main.Event{Source:"NodeA", Name:"notify.Create", Path:"trackertest copy.go", SourcePath:"", Time:time.Time{sec:63624468969, nsec:768126848, loc:(*time.Location)(0x45168c0)}, ModTime:time.Time{sec:0, nsec:0, loc:(*time.Location)(0x4512d60)}, IsDirectory:false, NetworkSource:"", RawData:[]uint8(nil)}
////
////let them replicate
//
//func (handler *FilesystemTracker) processEvent(event Event, pathName, fullPath string, lock bool) {
//	log.Println("FilesystemTracker:processEvent")
//	if lock {
//		handler.fsLock.Lock()
//	}
//	log.Println("FilesystemTracker:/processEvent")
//	defer log.Println("FilesystemTracker://processEvent")
//
//	log.Printf("handleFilsystemEvent name: %s pathName: %s", event.Name, pathName)
//	var err error
//
//	switch event.Name {
//	case "notify.Create":
//		err = handler.handleNotifyCreate(event, pathName, fullPath)
//	case "notify.Remove":
//		err = handler.handleNotifyRemove(event, pathName, fullPath)
//	case "notify.Rename":
//		err = handler.handleNotifyRename(event, pathName, fullPath)
//	case "notify.Write":
//		err = handler.handleNotifyWrite(event, pathName, fullPath)
//	default:
//		// do not send the event if we do not recognize it
//		log.Printf("%s: %s not known, skipping (%v)", event.Name, pathName, event)
//	}
//
//	if err != nil {
//		log.Printf("Error encountered when processing: %v", err)
//	}
//
//	if lock {
//		handler.fsLock.Unlock()
//		log.Println("FilesystemTracker:/+processEvent 2")
//	}
//}
//

//
//// EntryJSON - a JSON friendly version of the entry object. It does not have a native filesystem object inside of it.
//type EntryJSON struct {
//	RelativePath string
//	IsDirectory  bool
//	Hash         []byte
//	ModTime      time.Time
//	Size         int64
//	ServerName   string
//}
//
//// SendCatalog - Send our catalog out for other nodes to compare. This needs to be called with handler.fsLock engaged
//func (handler *FilesystemTracker) SendCatalog() {
//	log.Printf("FileSystemTracker ScanFolders - end - Found %d items", len(handler.contents))
//
//	handler.stats.CatalogsSent++
//	// transfer the normal contents structure to the JSON friendly version
//	//rawData := make([]EntryJSON, 0, len(handler.contents))
//	rawData := make([]EntryJSON, 0, len(handler.contents))
//
//	for k, v := range handler.contents {
//		entry := EntryJSON{RelativePath: k, IsDirectory: v.IsDir(), Hash: v.hash, ModTime: v.ModTime(), Size: v.Size()}
//		log.Printf("Packing up: %s=%#v", k, entry)
//		rawData = append(rawData, entry)
//	}
//
//	jsonData, err := json.Marshal(rawData)
//	if err != nil {
//		panic(err)
//	}
//	log.Printf("Analyzed all files in the folder and the total size is: %d", len(jsonData))
//
//	event := Event{
//		Name:          "replicat.Catalog",
//		Source:        globalSettings.Name,
//		Time:          time.Now(),
//		NetworkSource: globalSettings.Name,
//		RawData:       jsonData,
//	}
//
//	log.Printf("About to directly send out full catalog event with: %v", event)
//	sendCatalogToManagerAndSiblings(event)
//	log.Println("catalog sent")
//}
//
//// ProcessCatalog - handle a catalog passed from another replicat node
//func (handler *FilesystemTracker) ProcessCatalog(event Event) {
//	log.Printf("FilesystemTracker ProcessCatalog from Server: %s", event.Source)
//
//	handler.stats.CatalogsReceived++
//	remoteServer := event.Source
//	// pull the directory tree from the payload
//	remoteContents := make([]EntryJSON, 0)
//	err := json.Unmarshal(event.RawData, &remoteContents)
//	if err != nil {
//		panic(err)
//	}
//	log.Printf("Done unmarshalling event. len %d", len(remoteContents))
//
//	// Let's go through the other side's files and see if any of them are more up to date than what we have.
//	handler.fsLock.Lock()
//
//	log.Printf("Data retrieved from: %s\n%#v", event.Source, remoteContents)
//
//	if handler.neededFiles == nil {
//		handler.neededFiles = make(map[string]EntryJSON)
//	}
//
//	for _, remoteEntry := range remoteContents {
//		// Get the path out
//		path := remoteEntry.RelativePath
//
//		// check for a local value
//		local, exists := handler.contents[path]
//
//		// Request transfer of the file if we do not have a local copy already
//		transfer := !exists
//
//		log.Printf("Considering: %s\nexists: %v\nremote: %#v\nlocal:  %#v", path, exists, remoteEntry, local)
//		// Make missing directories immediately so we have a place to put the files
//		if !exists && remoteEntry.IsDirectory {
//			log.Printf("ProcessCatalog(%s) %s\nIs a directory, creating now", remoteEntry.ServerName, path)
//			// Make a missing directory
//			handler.createPath(path, true)
//			// Skip to the next entry
//			continue
//		}
//
//		if !transfer {
//			log.Printf("ProcessCatalog(%s) %s\remote: %v\nlocal:  %v", remoteEntry.ServerName, path, remoteEntry, local)
//			log.Printf("Comparing times for (%s) remote: %v local: %v", path, remoteEntry.ModTime, local.ModTime())
//			if local.hash == nil || local.ModTime().Before(remoteEntry.ModTime) {
//				transfer = true
//			}
//		}
//
//		hashSame := bytes.Equal(remoteEntry.Hash, local.hash)
//		log.Printf("Done considering(%s) transfer is: %t", path, transfer)
//		//todo should we do something if the transfer is set to true yet the hash is the same?
//
//		// If the hashes differ, we need to do something -- unless the other side is the older one
//		//if !transfer && !hashSame {
//		//	panic(log.Sprintf("We have a problem. file: %s\nremoteHash: %s\nlocalHash:  %s", path, remoteEntry.Hash, local.hash))
//		//}
//
//		if transfer {
//			currentEntry := handler.neededFiles[path]
//			log.Printf("We have decided to request transfer of this file: %s\nCurrent: %#v\nRemote: %#v", path, currentEntry, remoteEntry)
//
//			// If the current modification time is before the remote modification time, use the remote one
//			useNew := currentEntry.ModTime.Before(remoteEntry.ModTime)
//			log.Printf("Use New: %v HashSame: %v", useNew, hashSame)
//
//			// If the hash and time are the same, randomly decide which one to use
//			if !useNew && hashSame {
//				useNew = rand.Intn(1) == 1
//			}
//
//			// If we are going to use the new file, update the information
//			if useNew {
//				log.Println("decided to use new")
//				currentEntry.ModTime = remoteEntry.ModTime
//				currentEntry.Hash = remoteEntry.Hash
//				currentEntry.Size = remoteEntry.Size
//				currentEntry.ServerName = remoteServer
//
//				log.Printf("About to save %s \n%#v to neededFiles \n%#v", path, currentEntry, handler.neededFiles)
//				handler.neededFiles[path] = currentEntry
//			} else {
//				log.Println("decided to use old")
//
//			}
//		}
//	}
//
//	if len(handler.neededFiles) > 0 {
//		handler.server.SetStatus(REPLICAT_STATUS_JOINING_CLUSTER)
//		handler.requestNeededFiles()
//	} else {
//		handler.server.SetStatus(REPLICAT_STATUS_ONLINE)
//	}
//
//	handler.fsLock.Unlock()
//}
//
//// send out the actual requests for needed files when necessary. Call when inside of a lock!
//func (handler *FilesystemTracker) requestNeededFiles() {
//	// Collect the files needed for each server.
//	log.Printf("start collecting what we need from each server %#v", handler.neededFiles)
//	filesToFetch := make(map[string]map[string]EntryJSON)
//
//	for path, entry := range handler.neededFiles {
//		log.Printf("%s: %s (%#v)", entry.ServerName, path, entry)
//		server := entry.ServerName
//		fileMap := filesToFetch[server]
//		if fileMap == nil {
//			fileMap = make(map[string]EntryJSON)
//		}
//		fileMap[path] = entry
//		filesToFetch[server] = fileMap
//	}
//
//	for server, fileMap := range filesToFetch {
//		log.Printf("Files needed from: %s", server)
//		for filename, entry := range fileMap {
//			log.Printf("\t%s(%d) - %v", filename, entry.Size, entry.Hash)
//		}
//		handler.SendRequestForFiles(server, fileMap)
//	}
//	log.Println("done collecting what we need from each server")
//
//}
//
//// SendRequestForFiles - Request files you need from another Replicat This needs to be called with handler.fsLock engaged
//func (handler *FilesystemTracker) SendRequestForFiles(server string, fileMap map[string]EntryJSON) {
//	log.Printf("FileSystemTracker SendRequestForFiles - end - Found %d items", len(handler.contents))
//
//	jsonData, err := json.Marshal(fileMap)
//	if err != nil {
//		panic(err)
//	}
//	log.Printf("Files needed from %s: %d    Data: %d", server, len(fileMap), len(jsonData))
//
//	event := Event{
//		Name:          "replicat.FileRequest",
//		Source:        globalSettings.Name,
//		Time:          time.Now(),
//		NetworkSource: globalSettings.Name,
//		RawData:       jsonData,
//	}
//
//	log.Printf("About to directly send out request for files from %s: %v", server, event)
//	sendFileRequestToServer(server, event)
//	log.Println("request sent")
//}
