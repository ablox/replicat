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
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/goji/httpauth"
	"io"
	"log"
	"math/rand"
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
	// REPLICAT_STATUS_INITIAL_SCAN - The server is scanning its local storage
	REPLICAT_STATUS_INITIAL_SCAN = "Initial Scan"
	// REPLICAT_STATUS_JOINING_CLUSTER - The server has started to join the cluster and get up to date
	REPLICAT_STATUS_JOINING_CLUSTER = "Joining Cluster"
	// REPLICAT_STATUS_ONLINE - The server is up to date and part of the cluster
	REPLICAT_STATUS_ONLINE = "Online"
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

	//exerciseMinio()

	lsnr, err := net.Listen("tcp4", address)
	if err != nil {
		panic(fmt.Sprintf("Error listening: %v\nAddress: %s\n", err, address))
	}
	fmt.Println("Listening on:", lsnr.Addr().String())

	logOnlyHandler := LogOnlyChangeHandler{}
	var tracker StorageTracker = &FilesystemTracker{}

	fmt.Printf("Looking up settings for node: %s\n", globalSettings.Name)

	directory := globalSettings.Directory

	fmt.Printf("GlobalSettings directory retrieved for this node: %s\n", directory)
	server := &ReplicatServer{Name: globalSettings.Name, ClusterKey: globalSettings.ClusterKey, Address: lsnr.Addr().String(), storage: tracker, Status: REPLICAT_STATUS_INITIAL_SCAN}
	serverMap[globalSettings.Name] = server
	tracker.Initialize(directory, server)

	go func(tracker StorageTracker) {
		for true {
			tracker.GetStatistics()
			time.Sleep(30 * time.Second)
		}
	}(tracker)

	var c ChangeHandler
	c = &logOnlyHandler

	switch t := tracker.(type) {
	case *FilesystemTracker:
		t.watchDirectory(&c)
		//func (handler *FilesystemTracker) watchDirectory(watcher *ChangeHandler) {
	}

	go func(listener net.Listener) {
		err = http.Serve(listener, nil)
		if err != nil {
			panic(err)
		}
	}(lsnr)

	fmt.Println("Starting config update processor")
	go configUpdateProcessor(configUpdateChannel)

	if globalSettings.ManagerAddress != "" {
		fmt.Printf("about to send config to server (%s)\nOur address is: (%s)\n", globalSettings.ManagerAddress, lsnr.Addr())
	}

	// we need something to keep us running.
	keepConfigCurrent()
}

const (
	managerOverdueSeconds = 30
	managerCheckSleepTime = 45
)

func keepConfigCurrent() {
	for {
		serverMapLock.RLock()
		ago := time.Since(lastConfigPing)
		serverMapLock.RUnlock()
		if ago > managerOverdueSeconds*time.Second {
			log.Printf("Manager Contact Overdue, attempting to contact: %s\n", globalSettings.ManagerAddress)
			sendConfigToServer()
			//} else {
			//	fmt.Println("No Update Required")
		}
		time.Sleep(time.Duration(rand.Intn(10)-5+managerCheckSleepTime) * time.Second)
	}
}

func sendConfigToServer() {
	// This field will be empty during testing. We have to abort during testing since the manager is not reachable.
	if globalSettings.ManagerAddress == "" {
		return
	}

	url := "https://" + globalSettings.ManagerAddress + "/config/"
	log.Printf("sendConfigToServer: Manager location: %s\n", url)

	server := serverMap[globalSettings.Name]
	jsonStr, _ := json.Marshal(server)
	jsonStr2, _ := json.Marshal(server.storage.GetStatistics())

	byteArraysToMarshal := [][]byte{jsonStr, jsonStr2}
	byteData := bytes.Join(byteArraysToMarshal, []byte{})
	//log.Printf("jasonstr: %s\n", string(byteData))
	jsonData := bytes.NewBuffer(byteData)

	req, err := http.NewRequest("POST", url, jsonData)
	req.Header.Set("Content-Type", "application/json")

	data := []byte(globalSettings.ManagerCredentials)
	authHash := base64.StdEncoding.EncodeToString(data)
	req.Header.Add("Authorization", "Basic "+authHash)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Unable to reach server. Data could be lost: %s\n", err)
		return
	}

	newServerMap, err := extractServerMapFromConfig(resp.Body)
	if err == io.EOF {
		log.Printf("EOF unexpected Config update failed\n")
	} else if err != nil {
		log.Printf("Config update failed due to error: %s\n", err)
	} else {
		configUpdateChannel <- newServerMap
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
	newServerMap, err := extractServerMapFromConfig(r.Body)
	if err != nil {
		fmt.Printf("Config update failed due to error: %s\n", err)
		return
	}
	configUpdateChannel <- newServerMap
}

func extractServerMapFromConfig(sourceReader io.Reader) (newServerMap *map[string]*ReplicatServer, err error) {
	decoder := json.NewDecoder(sourceReader)
	err = decoder.Decode(&newServerMap)
	return
}

func configUpdateProcessor(c chan *map[string]*ReplicatServer) {
	for {
		newServerMap := <-c
		serverMapLock.Lock()

		lastConfigPing = time.Now()
		sendData := false

		// find any nodes that have been deleted and update ones that have changed
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
