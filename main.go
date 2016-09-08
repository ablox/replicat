// Copyright 2016 Jacob Taylor jacob@ablox.io
// License: Apache2 - http://www.apache.org/licenses/LICENSE-2.0
package main

import (
	"fmt"
	"github.com/rjeczalik/notify"
	"github.com/urfave/cli"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"bytes"
	"io/ioutil"
	"net/http"
	"encoding/base64"
	"encoding/json"
	"github.com/goji/httpauth"
	"github.com/pubnub/go/messaging"
	"regexp"
	"html/template"
)

type ReplicatServer struct {
	Name    		string
	Address 		string
	PublicKey 		string
	LastReceived 	DirTreeMap
	LastSent 		DirTreeMap
}

type ServerMap map[string]ReplicatServer

var GlobalServerMap = ServerMap {
	"NodeA": ReplicatServer{
		Address: ":8001",
	},
	"NodeB": ReplicatServer{
		Address: ":8002",
	},
}

type Event struct {
	Source  string
	Name    string
	Message string
	Time     time.Time
}

var events = make([]Event, 0, 100)

//var events = []Event{
//	Event{Source: "local test", Name: "Cluster initialized"},
//	Event{Source: "local test", Name: "Server 10.1.1.1 joined Cluster"},
//}

// settings for the server
type Settings struct {
	Directory 				string
	ManagerAddress 			string
	ManagerCredentials 		string
	ManagerEnabled 			bool
	Address 				string
	Name 					string
}

type DirTreeMap map[string][]string
//type DirTreeMap map[string][]os.FileInfo


var globalSettings Settings = Settings{
	Directory: "",
	ManagerAddress: "localhost:8080",
	ManagerCredentials: "replicat:isthecat",
	Address: ":8001",
	Name: "",
}

func checkForChanges(basePath string, originalState DirTreeMap) (changed bool, updatedState DirTreeMap, newPaths, deletedPaths, matchingPaths []string)  {
	//  (map[string][]string, error) {
	updatedState, err := createListOfFolders(basePath)
	if err != nil {
		panic(err)
	}

	// Get a list of paths and compare them
	originalPaths := make([]string, 0, len(originalState))
	updatedPaths := make([]string, 0, len(updatedState))

	//todo this should already be sorted. This is very repetitive.
	for key := range originalState {
		originalPaths = append(originalPaths, key)
	}
	sort.Strings(originalPaths)

	for key := range updatedState {
		updatedPaths = append(updatedPaths, key)
	}
	sort.Strings(updatedPaths)
	//todo we should leverage the updated paths to obliviate the need to resort.

	// We now have two sorted lists of strings. Go through the original ones and compare the files
	var originalPosition, updatedPosition int

	deletedPaths = make([]string, 0, 100)
	newPaths = make([]string, 0, 100)
	matchingPaths = make([]string, 0, len(originalPaths))

	//pp := func(name string, stringList []string) {
	//	fmt.Println("***************************")
	//	fmt.Println(name)
	//	fmt.Println("***************************")
	//	for index, value := range stringList {
	//		fmt.Printf("[%3d]: %s\n", index, value)
	//	}
	//	fmt.Println("***************************")
	//}
	//pp("original paths", originalPaths)
	//pp("updated Paths", updatedPaths)

	for {
		//fmt.Printf("Original Position %3d    Updated Position %3d\n", originalPosition, updatedPosition)
		if originalPosition >= len(originalPaths) {
			// all remaining updated paths are new
			newPaths = append(newPaths, updatedPaths[updatedPosition:]...)
			//fmt.Println("Adding remaining paths")
			break
		} else if updatedPosition >= len(updatedPaths) {
			// all remaining original paths are new
			//fmt.Println("Deleting remaining paths")
			deletedPaths = append(deletedPaths, originalPaths[originalPosition:]...)
			break
		} else {
			oldPath := originalPaths[originalPosition]
			updPath := updatedPaths[updatedPosition]
			//fmt.Printf("comparing paths: '%s' and '%s'\n", oldPath, updPath)

			// Start with nothing changed. Base case
			if oldPath == updPath {
				//fmt.Println("match")
				matchingPaths = append(matchingPaths, updatedPaths[updatedPosition])
				updatedPosition++
				originalPosition++
			} else if oldPath > updPath {
				//fmt.Println("adding new path")
				newPaths = append(newPaths, updatedPaths[updatedPosition])
				updatedPosition++
			} else {
				//fmt.Println("Deleting old path")
				deletedPaths = append(deletedPaths, originalPaths[originalPosition])
				originalPosition++
			}
		}
	}

	//fmt.Printf("Path report: new %d, deleted %d, matching %d, original %d, updated %d\n", len(newPaths), len(deletedPaths), len(matchingPaths), len(originalPaths), len(updatedPaths))
	//fmt.Printf("New paths: %v\n", newPaths)
	//fmt.Printf("Deleted paths: %v\n", deletedPaths)

	if len(newPaths) > 0 || len(deletedPaths) > 0 {
		changed = true
	}

	return changed, updatedState, newPaths, deletedPaths, matchingPaths
}

func main() {
	fmt.Println("replicat initializing....")

	app := cli.NewApp()
	app.Name = "Replicat"
	app.Usage = "rsync for the cloud"
	app.Action = func(c *cli.Context) error {
		globalSettings.Directory = c.GlobalString("directory")
		globalSettings.ManagerAddress = c.GlobalString("manager")
		globalSettings.ManagerCredentials = c.GlobalString("manager_credentials")
		globalSettings.Address = c.GlobalString("address")
		globalSettings.Name = c.GlobalString("name")

		if globalSettings.Directory == "" {
			panic("directory is required to serve files\n")
		}

		if globalSettings.Name == "" {
			panic("Name is currently a required parameter. Name has to be one of the predefined names (e.g. NodeA, NodeB). This will improve.\n")
		}

		return nil
	}

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:   "directory, d",
			Value:  globalSettings.Directory,
			Usage:  "Specify a directory where the files to share are located.",
			EnvVar: "directory, d",
		},
		cli.StringFlag{
			Name:   "manager, m",
			Value:  globalSettings.ManagerAddress,
			Usage:  "Specify a host and port for reaching the manager",
			EnvVar: "manager, m",
		},
		cli.StringFlag{
			Name:   "manager_credentials, mc",
			Value:  globalSettings.ManagerCredentials,
			Usage:  "Specify a usernmae:password for login to the manager",
			EnvVar: "manager_credentials, mc",
		},
		cli.StringFlag{
			Name:   "address, a",
			Value:  globalSettings.Address,
			Usage:  "Specify a listen address for this node. e.g. '127.0.0.1:8000' or ':8000' for where updates are accepted from",
			EnvVar: "address, a",
		},
		cli.StringFlag{
			Name:   "name, n",
			Value:  globalSettings.Name,
			Usage:  "Specify a name for this node. e.g. 'NodeA' or 'NodeB'",
			EnvVar: "name, n",
		},
	}

	app.Run(os.Args)

	listOfFileInfo, err := createListOfFolders(globalSettings.Directory)
	if err != nil {
		log.Fatal(err)
	}

	// Make the channel buffered to ensure no event is dropped. Notify will drop
	// an event if the receiver is not able to keep up the sending pace.
	fsEventsChannel := make(chan notify.EventInfo, 1)

	// Set up a watchpoint listening for events within a directory tree rooted at the specified folder
	if err := notify.Watch(globalSettings.Directory + "/...", fsEventsChannel, notify.All); err != nil {
		log.Fatal(err)
	}
	defer notify.Stop(fsEventsChannel)

	fmt.Printf("replicat %s online....\n", globalSettings.Name)
	defer fmt.Println("End of line")

	totalFiles := 0
	for _, fileInfoList := range listOfFileInfo {
		totalFiles += len(fileInfoList)
	}

	fmt.Printf("Now keeping an eye on %d folders and %d files located under: %s\n", len(listOfFileInfo), totalFiles, globalSettings.Directory)

	go func(c chan notify.EventInfo) {
		for {
			ei := <-c
			sendEvent(&Event{Name: ei.Event().String(), Message: ei.Path()})
			log.Println("Got event:" + ei.Event().String() + ", with Path:" + ei.Path())
		}
	}(fsEventsChannel)

	http.HandleFunc("/view/", makeHandler(viewHandler))
	http.HandleFunc("/edit/", makeHandler(editHandler))
	http.HandleFunc("/save/", makeHandler(saveHandler))

	http.Handle("/home/", httpauth.SimpleBasicAuth("replicat", "isthecat")(http.HandlerFunc(homeHandler)))
	http.Handle("/event/", httpauth.SimpleBasicAuth("replicat", "isthecat")(http.HandlerFunc(eventHandler)))
	http.Handle("/tree/", httpauth.SimpleBasicAuth("replicat", "isthecat")(http.HandlerFunc(folderTreeHandler)))

	fmt.Printf("About to listen on: %s\n", globalSettings.Address)

	go func() {
		err = http.ListenAndServe(globalSettings.Address, nil)
		if err != nil {
			panic(err)
		}

	}()

	for {
		time.Sleep(time.Second * 5)
		changed, updatedState, newPaths, deletedPaths, matchingPaths := checkForChanges(globalSettings.Directory, listOfFileInfo)
		if changed {
			fmt.Println("\nWe have changes, ship it (also updating saved state now that the changes were tracked)")
			fmt.Printf("@Path report: new %d, deleted %d, matching %d, original %d, updated %d\n", len(newPaths), len(deletedPaths), len(matchingPaths), len(listOfFileInfo), len(updatedState))
			fmt.Printf("@New paths: %v\n", newPaths)
			fmt.Printf("@Deleted paths: %v\n", deletedPaths)
			fmt.Println("******************************************************")
			listOfFileInfo = updatedState
		} else {
			fmt.Print(".")
		}

	}
}


func createListOfFolders(basePath string) (DirTreeMap, error) {
	paths := make([]string, 0, 100)
	pendingPaths := make([]string, 0, 100)
	pendingPaths = append(pendingPaths, basePath)
	listOfFileInfo := make(DirTreeMap)

	for len(pendingPaths) > 0 {
		currentPath := pendingPaths[0]
		paths = append(paths, currentPath)
		fileList := make([]string, 0, 100)
		//fileList := make([]os.FileInfo, 0, 100)
		pendingPaths = pendingPaths[1:]

		// Read the directories in the path
		f, err := os.Open(currentPath)
		if err != nil {
			return nil, err
		}
		dirEntries, err := f.Readdir(-1)
		for _, entry := range dirEntries {
			if entry.IsDir() {
				entry.Mode()
				newDirectory := filepath.Join(currentPath, entry.Name())
				//newDirectory := filepath.Join(currentPath, entry)
				pendingPaths = append(pendingPaths, newDirectory)
			} else {
				fileList = append(fileList, entry.Name())
			}
		}
		f.Close()
		if err != nil {
			return nil, err
		}

		sort.Strings(fileList)
		listOfFileInfo[currentPath] = fileList
	}

	//fmt.Printf("Export:\n")
	//for dir, _ := range listOfFileInfo {
	//	fmt.Printf("%s\n", dir)
	//}
	//fmt.Printf("Export done:\n")

	return listOfFileInfo, nil
}

/*
Send the folder tree from this node to another node for comparison
 */
func sendFolderTree(tree *DirTreeMap) {
	// package up the tree
	jsonStr, err := json.Marshal(tree)
	if err != nil {
		panic(err)
	}
	buffer := bytes.NewBuffer(jsonStr)
	client := &http.Client{}

	data := []byte(globalSettings.ManagerCredentials)
	authHash := base64.StdEncoding.EncodeToString(data)

	for _, server := range GlobalServerMap {
		url := "http://" + server.Address + "/tree/"
		fmt.Printf("Posting folder tree to: %s\n", url)

		req, err := http.NewRequest("POST", url, buffer)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Basic " + authHash)

		resp, err := client.Do(req)
		if err != nil {
			panic(err)
		}

		fmt.Println("response Status:", resp.Status)
		fmt.Println("response Headers:", resp.Header)
		body, _ := ioutil.ReadAll(resp.Body)
		fmt.Println("response Body:", string(body))
		resp.Body.Close()
	}
}

func folderTreeHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		listOfFileInfo, err := createListOfFolders(globalSettings.Directory)
		if err != nil {
			log.Fatal(err)
		}
		json.NewEncoder(w).Encode(listOfFileInfo)
		fmt.Printf("Sending tree of size %d back to client\n", len(listOfFileInfo))
	case "POST":
		log.Println("Got POST: ", r.Body)
		decoder := json.NewDecoder(r.Body)

		var tree DirTreeMap
		err := decoder.Decode(&tree)
		if err != nil {
			panic("bad json body")
		}

		fmt.Printf("Received a tree map from: %s\n%s", r.RemoteAddr, tree)
	}
}


func sendEvent(event *Event) {
	url := "http://" + globalSettings.ManagerAddress + "/event/"
	fmt.Printf("Manager location: %s\n", url)

	jsonStr, _ := json.Marshal(event)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonStr))
	req.Header.Set("Content-Type", "application/json")

	data := []byte(globalSettings.ManagerCredentials)
	fmt.Printf("Manager Credentials: %s\n", data)
	authHash := base64.StdEncoding.EncodeToString(data)
	req.Header.Add("Authorization", "Basic " + authHash)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	fmt.Println("response Status:", resp.Status)
	fmt.Println("response Headers:", resp.Header)
	body, _ := ioutil.ReadAll(resp.Body)
	fmt.Println("response Body:", string(body))
}


type Page struct {
	Title string
	Body  []byte
}

var templates = template.Must(template.ParseFiles("home.html", "edit.html", "view.html"))
var validPath = regexp.MustCompile("^/(edit|save|view|home)/([a-zA-Z0-9]+)$")

func renderTemplate(w http.ResponseWriter, tmpl string, p *Page) {
	err := templates.ExecuteTemplate(w, tmpl + ".html", p)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func makeHandler(fn func(http.ResponseWriter, *http.Request, string)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		m := validPath.FindStringSubmatch(r.URL.Path)
		if m == nil {
			http.NotFound(w, r)
			return
		}
		fn(w, r, m[2])
	}
}

func homeHandler(w http.ResponseWriter, _ *http.Request) {
	renderTemplate(w, "home", nil)
}

func eventHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		json.NewEncoder(w).Encode(events)
	case "POST":
		log.Println("Got POST: ", r.Body)
		decoder := json.NewDecoder(r.Body)
		var event Event
		err := decoder.Decode(&event)
		event.Source = r.RemoteAddr
		if err != nil {
			panic("bad json body")
		}
		log.Println(event.Name)
		events = append([]Event{event}, events...)
	}
}

func viewHandler(w http.ResponseWriter, r *http.Request, title string) {
	p, err := loadPage(title)
	if err != nil {
		http.Redirect(w, r, "/edit/" + title, http.StatusNotFound)
		return
	}
	renderTemplate(w, "view", p)
}

func editHandler(w http.ResponseWriter, r *http.Request, title string) {
	_ = r
	p, err := loadPage(title)
	if err != nil {
		p = &Page{Title: title}
	}
	renderTemplate(w, "edit", p)
}

func saveHandler(w http.ResponseWriter, r *http.Request, title string) {
	body := r.FormValue("body")
	p := &Page{Title: title, Body: []byte(body)}
	err := p.save()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/view/" + title, http.StatusFound)
}

func (p *Page) save() error {
	filename := p.Title + ".txt"
	return ioutil.WriteFile(filename, p.Body, 0600)
}

func loadPage(title string) (*Page, error) {
	filename := title + ".txt"
	body, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	return &Page{Title: title, Body: body}, nil
}

func handlePubNub() {
	fmt.Println("PubNub SDK for go;", messaging.VersionInfo())

	publishKey := "pub-c-fc75596b-c9cf-40c9-844f-f31d7842419c"
	subscribeKey := "sub-c-a76871e8-6692-11e6-879b-0619f8945a4f"
	secretKey := "sec-c-ZWNlNDRlZGYtODIyNi00ZjZhLWE5ZGUtM2FlNmYxNDk1NjQy"

	pubnub := messaging.NewPubnub(publishKey, subscribeKey, secretKey, "", false, "")
	channel := "Channel-ly2qa34uj"

	connected := make(chan struct{})
	msgReceived := make(chan struct{})

	successChannel := make(chan []byte)
	errorChannel := make(chan []byte)

	go pubnub.Subscribe(channel, "", successChannel, false, errorChannel)

	go func() {
		for {
			select {
			case response := <-successChannel:
				var msg []interface{}

				err := json.Unmarshal(response, &msg)
				if err != nil {
					panic(err.Error())
				}

				switch t := msg[0].(type) {
				case float64:
					if strings.Contains(msg[1].(string), "connected") {
						close(connected)
					}
				case []interface{}:
					fmt.Println(string(response))
					close(msgReceived)
				default:
					panic(fmt.Sprintf("Unknown type: %T", t))
				}
			case err := <-errorChannel:
				fmt.Println(string(err))
				return
			case <-messaging.SubscribeTimeout():
				panic("Subscribe timeout")
			}
		}
	}()

	<-connected

	publishSuccessChannel := make(chan []byte)
	publishErrorChannel := make(chan []byte)

	go pubnub.Publish(channel, "Hello from PubnNub Go SDK  haha!",
		publishSuccessChannel, publishErrorChannel)

	go func() {
		select {
		case result := <-publishSuccessChannel:
			fmt.Println(string(result))
		case err := <-publishErrorChannel:
			fmt.Printf("Publish error: %s\n", err)
		case <-messaging.Timeout():
			fmt.Println("Publish timeout")
		}
	}()

	<-msgReceived
}

