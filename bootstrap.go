// Package replicat is a server for n way synchronization of content (rsync for the cloud).
// More information at: http://replic.at
// Copyright 2016 Jacob Taylor jacob@ablox.io
// License: Apache2 - http://www.apache.org/licenses/LICENSE-2.0
package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/goji/httpauth"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"sync"
	"time"
)

// ReplicatServer is a structure that contains the definition of the servers in a cluster. Each node has a name and this
// node (as determined by globalSettings.name at the moment) also has a StorageTracker interface.
type ReplicatServer struct {
	ClusterKey    string
	Name          string
	Address       string
	Status        string
	CurrentState  DirTreeMap
	PreviousState DirTreeMap
	Lock          sync.Mutex
	storage       StorageTracker
}

// GetStatus - get the current status of the server
func (server *ReplicatServer) GetStatus() string {
	return server.Status
}

// SetStatus - set the current status of the server
func (server *ReplicatServer) SetStatus(status string) {
	// adding an inline check to make sure the server is valid
	// if the server is being shut down or at the end of a unit test)
	// the assignment can crash without this check
	if server != nil {
		server.Status = status
		sendConfigToServer()
	}
}

const (
	REPLICAT_STATUS_INITIAL_SCAN    = "Initial Scan"			// The server is scanning its local storage
	REPLICAT_STATUS_JOINING_CLUSTER = "Joining Cluster"			// The server has started to join the cluster and get up to date
	REPLICAT_STATUS_ONLINE          = "Online"					// The server is up to date and part of the cluster
)

var serverMap = make(map[string]*ReplicatServer)
var serverMapLock sync.RWMutex

var lastConfigPing = time.Now()

// BootstrapAndServe - Start the server
func BootstrapAndServe(address string) {
	//trackerTestDual()
	//trackerTestSmallFileInSubfolder()
	//trackerTestEmptyDirectoryMovesInOutAround()
	//trackerTestFileChangeTrackerAddFolders()
	//trackerTestSmallFileCreationAndRename()
	//trackerTestSmallFileCreationAndUpdate()
	//trackerTestSmallFileMovesInOutAround()
	//trackerTestDirectoryCreation()
	//trackerTestNestedDirectoryCreation()
	//trackerTestDirectoryStorage()
	//trackerTestFileChangeTrackerAutoCreateFolderAndCleanup()
	//trackerTestNestedFastDirectoryCreation()

	// testing code to enable debugger use
	http.Handle("/event/", httpauth.SimpleBasicAuth("replicat", "isthecat")(http.HandlerFunc(eventHandler)))
	http.Handle("/tree/", httpauth.SimpleBasicAuth("replicat", "isthecat")(http.HandlerFunc(folderTreeHandler)))
	http.Handle("/config/", httpauth.SimpleBasicAuth("replicat", "isthecat")(http.HandlerFunc(configHandler)))
	http.Handle("/upload/", httpauth.SimpleBasicAuth("replicat", "isthecat")(http.HandlerFunc(uploadHandler)))

	lsnr, err := net.Listen("tcp4", address)
	if err != nil {
		panic(fmt.Sprintf("Error listening: %v\nAddress: %s\n", err, address))
	}
	fmt.Println("Listening on:", lsnr.Addr().String())

	logOnlyHandler := LogOnlyChangeHandler{}
	tracker := FilesystemTracker{}
	fmt.Printf("Looking up settings for node: %s\n", globalSettings.Name)

	directory := globalSettings.Directory

	fmt.Printf("GlobalSettings directory retrieved for this node: %s\n", directory)
	server := &ReplicatServer{Name: globalSettings.Name, ClusterKey: globalSettings.ClusterKey, Address: lsnr.Addr().String(), storage: &tracker, Status: REPLICAT_STATUS_INITIAL_SCAN}
	serverMap[globalSettings.Name] = server
	tracker.init(directory, server)

	go func(tracker StorageTracker) {
		for true {
			tracker.GetStatistics()
			time.Sleep(30 * time.Second)
		}
	}(&tracker)

	var c ChangeHandler
	c = &logOnlyHandler
	tracker.watchDirectory(&c)

	go func(listener net.Listener) {
		err = http.Serve(listener, nil)
		if err != nil {
			panic(err)
		}
	}(lsnr)

	fmt.Println("Starting config update processor")
	go configUpdateProcessor(configUpdateChannel)
	go keepConfigCurrent()

	if globalSettings.ManagerAddress != "" {
		fmt.Printf("about to send config to server (%s)\nOur address is: (%s)", globalSettings.ManagerAddress, lsnr.Addr())
	}
}

func keepConfigCurrent() {
	for {
		if time.Since(lastConfigPing) > 30*time.Second {
			log.Printf("Manager Contact Overdue, attempting to contact: %s\n", globalSettings.ManagerAddress)
			sendConfigToServer()
		} else {
			fmt.Println("No Update Required")
		}
		time.Sleep(45 * time.Second)
	}
}

func sendConfigToServer() {
	// This field will be empty during testing
	if globalSettings.ManagerAddress == "" {
		return
	}

	url := "http://" + globalSettings.ManagerAddress + "/config/"
	fmt.Printf("Manager location: %s\n", url)

	jsonStr, _ := json.Marshal(serverMap[globalSettings.Name])
	fmt.Printf("jsonStr: %s\n", jsonStr)

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonStr))
	req.Header.Set("Content-Type", "application/json")

	data := []byte(globalSettings.ManagerCredentials)
	authHash := base64.StdEncoding.EncodeToString(data)
	req.Header.Add("Authorization", "Basic "+authHash)

	client := &http.Client{}
	_, err = client.Do(req)
	if err != nil {
		panic(err)
	}
}

func uploadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		r.ParseMultipartForm(32 << 20)
		file, handler, err := r.FormFile("uploadfile")
		if err != nil {
			fmt.Println(err)
			return
		}
		defer file.Close()
		fmt.Fprint(w, handler.Header)

		hash := []byte(r.Form.Get("HASH"))

		storage := serverMap[globalSettings.Name].storage
		local, _ := storage.getEntryJSON(handler.Filename)

		if !bytes.Equal(hash, local.Hash) {
			fullPath := globalSettings.Directory + "/" + handler.Filename
			f, err := os.OpenFile(fullPath, os.O_WRONLY|os.O_CREATE, 0666)

			if err != nil {
				fmt.Println(err)
				return
			}

			bytesWritten, err := io.Copy(f, file)
			if err != nil {
				log.Printf("Error copying file: %s, error(%#v)\n", handler.Filename, err)
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte("500 - Error copying file"))
				f.Close()
				return
			}
			fmt.Printf("Wrote out (%s) bytes (%d)\n", handler.Filename, bytesWritten)
			f.Close()

			// Get the old entry
			entryString := r.Form.Get("entryJSON")
			if entryString != "" {
				var entry EntryJSON
				err := json.Unmarshal([]byte(entryString), &entry)
				if err != nil {
					log.Fatalf("Error copying file (Entry handling): %s, error(%#v)\n", handler.Filename, err)
					w.WriteHeader(http.StatusInternalServerError)
					w.Write([]byte("500 - Error copying file"))
					return
				}

				err = os.Chtimes(fullPath, time.Now(), entry.ModTime)
				if err != nil {
					log.Fatalf("Error copying file (Changing times): %s, error(%#v)\n", handler.Filename, err)
					w.WriteHeader(http.StatusInternalServerError)
					w.Write([]byte("500 - Error copying file"))
					return
				}
			}

		}
	}
}

var configUpdateChannel = make(chan *map[string]*ReplicatServer, 100)

func configHandler(_ http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "POST":
		lastConfigPing = time.Now()
		fmt.Println("configHandler: 3")
		decoder := json.NewDecoder(r.Body)
		fmt.Println("configHandler: 4")
		var newServerMap map[string]*ReplicatServer
		err := decoder.Decode(&newServerMap)
		fmt.Println("configHandler: 5")
		if err != nil {
			fmt.Println(err)
		}

		fmt.Println("configHandler: 6")
		configUpdateChannel <- &newServerMap
		fmt.Println("configHandler: 8")
	}
}

func configUpdateProcessor(c chan *map[string]*ReplicatServer) {
	for {
		newServerMap := <-c
		serverMapLock.Lock()

		sendData := false

		// find any nodes that have been deleted
		for name, serverData := range serverMap {
			newServerData, exists := (*newServerMap)[name]
			if !exists {
				fmt.Printf("No longer found config for: %s deleting\n", name)
				delete(serverMap, name)
				continue
			}

			if serverData.Address != newServerData.Address || serverData.Name != newServerData.Name || serverData.ClusterKey != newServerData.ClusterKey || serverData.Status != serverData.Status {
				fmt.Printf("Server data is changed. Replacing.\nold: %v\nnew: %v\n", &serverData, &newServerData)
				if serverData.Status != REPLICAT_STATUS_JOINING_CLUSTER && newServerData.Status == REPLICAT_STATUS_JOINING_CLUSTER {
					fmt.Printf("Decided to send data to: %s\n", serverData.Name)
					sendData = true
				}
				serverMap[name] = newServerData
				fmt.Println("Server data replaced with new server data")
			}
		}

		// find any new nodes
		for name, newServerData := range *newServerMap {
			_, exists := serverMap[name]
			if !exists {
				fmt.Printf("New server configuration for %s: %v\n", name, newServerData)

				// If this server map is for ourselves, build a list of folder if needed and notify others
				if name == globalSettings.Name {
					listOfFileInfo, err := scanDirectoryContents()
					if err != nil {
						log.Fatal(err)
					}
					newServerData.CurrentState = listOfFileInfo
					// Tell all of our friends that we exist and our current state for them to compare against.
					go func(tree DirTreeMap) {
						sendFolderTree(tree)
					}(listOfFileInfo)
				} else {
					fmt.Printf("New Node Decided to send data to: %s\n", name)
					sendData = true
				}

				fmt.Printf("New server configuration provided. Copying: %s\n", name)
				serverMap[name] = newServerData
			}
		}

		if sendData {
			server := serverMap[globalSettings.Name]
			fmt.Println("about to send existing files")
			server.storage.SendCatalog()
			fmt.Println("done sending existing files")
		}

		serverMapLock.Unlock()
	}
}
