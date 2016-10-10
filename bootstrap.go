// Copyright 2016 Jacob Taylor jacob@ablox.io
// License: Apache2 - http://www.apache.org/licenses/LICENSE-2.0
package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/goji/httpauth"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"io"
)

// ReplicatServer is a structure that contains the definition of the servers in a cluster. Each node has a name and this
// node (as determined by globalSettings.name at the moment) also has a StorageTracker interface.
type ReplicatServer struct {
	Cluster       string
	Name          string
	Address       string
	CurrentState  DirTreeMap
	PreviousState DirTreeMap
	Lock          sync.Mutex
	storage       StorageTracker
}

var serverMap = make(map[string]*ReplicatServer)
var serverMapLock sync.RWMutex

func bootstrapAndServe() {
	http.Handle("/event/", httpauth.SimpleBasicAuth("replicat", "isthecat")(http.HandlerFunc(eventHandler)))
	http.Handle("/tree/", httpauth.SimpleBasicAuth("replicat", "isthecat")(http.HandlerFunc(folderTreeHandler)))
	http.Handle("/config/", httpauth.SimpleBasicAuth("replicat", "isthecat")(http.HandlerFunc(configHandler)))
	http.Handle("/upload/", httpauth.SimpleBasicAuth("replicat", "isthecat")(http.HandlerFunc(uploadHandler)))

	lsnr, err := net.Listen("tcp4", ":0")
	if err != nil {
		fmt.Println("Error listening:", err)
		os.Exit(1)
	}
	fmt.Println("Listening on:", lsnr.Addr().String())

	logOnlyHandler := LogOnlyChangeHandler{}
	tracker := FilesystemTracker{}
	tracker.init(globalSettings.Directory)
	var c ChangeHandler
	c = &logOnlyHandler
	tracker.watchDirectory(&c)

	serverMap[globalSettings.Name] = &ReplicatServer{Name: globalSettings.Name, Address: "127.0.0.1:" + strconv.Itoa(lsnr.Addr().(*net.TCPAddr).Port), storage: &tracker}

	go func(lsnr net.Listener) {
		err = http.Serve(lsnr, nil)
		if err != nil {
			panic(err)
		}
	}(lsnr)

	fmt.Println("about to send config to server")
	go sendConfigToServer(lsnr.Addr())
	fmt.Printf("config sent to server with address: %s\n", lsnr.Addr())
}

func sendConfigToServer(addr net.Addr) {
	url := "http://" + globalSettings.BootstrapAddress + "/config/"
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
	fmt.Println("method: ", r.Method)
	if r.Method == "POST" {
		r.ParseMultipartForm(32 << 20)
		file, handler, err := r.FormFile("uploadfile")
		if err != nil {
			fmt.Println(err)
			return
		}
		defer file.Close()
		fmt.Fprint(w, "%v", handler.Header)
		f, err := os.OpenFile(globalSettings.Directory+"/"+handler.Filename, os.O_WRONLY|os.O_CREATE, 0666)
		if err!= nil {
			fmt.Println(err)
			return
		}
		defer f.Close()
		io.Copy(f, file)
	}
}

func configHandler(_ http.ResponseWriter, r *http.Request) {
	log.Println("configHandler called on bootstrap")
	switch r.Method {
	case "POST":
		serverMapLock.Lock()
		defer serverMapLock.Unlock()

		//todo move this to a channel to ensure ordering. It should always be safe to grab the latest one only.
		decoder := json.NewDecoder(r.Body)
		var newServerMap map[string]*ReplicatServer //:= make(map[string]ReplicatServer)
		err := decoder.Decode(&newServerMap)
		if err != nil {
			fmt.Println(err)
		}
		log.Printf("configHandler serverMap read from webcat to: %v\n", newServerMap)

		// find any nodes that have been deleted
		for name, serverData := range serverMap {
			newServerData, exists := newServerMap[name]
			if !exists {
				fmt.Printf("No longer found config for: %s deleting\n", name)
				delete(serverMap, name)
				continue
			}

			if serverData.Address != newServerData.Address || serverData.Name != newServerData.Name || serverData.Cluster != newServerData.Cluster {
				fmt.Printf("Server data is radically changed. Replacing.\nold: %v\nnew: %v\n", &serverData, &newServerData)
				serverMap[name] = newServerData
				fmt.Println("Server data replaced with new server data")
			} else {
				fmt.Printf("Server data has not radically changed. ignoring.\nold: %v\nnew: %v\n", &serverData, &newServerData)
			}
		}

		// find any new nodes
		for name, newServerData := range newServerMap {
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
				}

				fmt.Printf("New server configuration provided. Copying: %s\n", name)
				serverMap[name] = newServerData
			}
		}
	}
}
