// Copyright 2016 Jacob Taylor jacob@ablox.io
// License: Apache2 - http://www.apache.org/licenses/LICENSE-2.0
package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/goji/httpauth"
	"github.com/pubnub/go/messaging"
	"github.com/rjeczalik/notify"
	"github.com/urfave/cli"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

type ReplicatServer struct {
	Name         string
	Address      string
	PublicKey    string
	LastReceived DirTreeMap
	LastSent     DirTreeMap
}

type ServerMap map[string]ReplicatServer

var GlobalServerMap = ServerMap{
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
	Time    time.Time
}

var events = make([]Event, 0, 100)

//var events = []Event{
//	Event{Source: "local test", Name: "Cluster initialized"},
//	Event{Source: "local test", Name: "Server 10.1.1.1 joined Cluster"},
//}

// settings for the server
type Settings struct {
	Directory          string
	ManagerAddress     string
	ManagerCredentials string
	ManagerEnabled     bool
	Address            string
	Name               string
	Peers              string
}

type DirTreeMap map[string][]string

//type DirTreeMap map[string][]os.FileInfo

var globalSettings Settings = Settings{
	Directory:          "",
	ManagerAddress:     "localhost:8080",
	ManagerCredentials: "replicat:isthecat",
	Address:            ":8001",
	Name:               "",
}

func checkForChanges(basePath string, originalState DirTreeMap) (changed bool, updatedState DirTreeMap, newPaths, deletedPaths, matchingPaths []string) {
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
		globalSettings.Peers = c.GlobalString("peers")

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
		cli.StringFlag{
			Name:   "peers, p",
			Value:  globalSettings.Peers,
			Usage:  "Specify peers for this node. e.g. 1.2.3.4:8001,2.2.2.2",
			EnvVar: "peers, p",
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

	// Set up a watch point listening for events within a directory tree rooted at the specified folder
	if err := notify.Watch(globalSettings.Directory+"/...", fsEventsChannel, notify.All); err != nil {
		log.Fatal(err)
	}
	defer notify.Stop(fsEventsChannel)

	fmt.Printf("replicat %s online....\n", globalSettings.Name)
	defer fmt.Println("End of line")

	globalSettings.Address = GlobalServerMap[globalSettings.Name].Address

	totalFiles := 0
	for _, fileInfoList := range listOfFileInfo {
		totalFiles += len(fileInfoList)
	}

	fmt.Printf("Now keeping an eye on %d folders and %d files located under: %s\n", len(listOfFileInfo), totalFiles, globalSettings.Directory)

	go func(c chan notify.EventInfo) {
		for {
			ei := <-c

			// sendEvent to manager (if it's available)
			sendEvent(&Event{Name: ei.Event().String(), Message: ei.Path()}, globalSettings.ManagerAddress, globalSettings.ManagerCredentials)

			// sendEvent to peers (if any)
			for _, address := range strings.Split(globalSettings.Peers, ",") {
				sendEvent(&Event{Name: ei.Event().String(), Message: ei.Path()}, address, globalSettings.ManagerCredentials)

			}

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

			// Post the changes to the other side.
			sendFolderTree(listOfFileInfo)

		} else {
			fmt.Print(".")
		}
	}
}

func createListOfFolders(basePath string) (DirTreeMap, error) {
	//paths := make([]string, 0, 100)
	pendingPaths := make([]string, 0, 100)
	pendingPaths = append(pendingPaths, basePath)
	listOfFileInfo := make(DirTreeMap)

	for len(pendingPaths) > 0 {
		currentPath := pendingPaths[0]
		// Strip off of the base path before adding it to the list of folders
		//paths = append(paths, currentPath[len(globalSettings.Directory)+1:])
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
func sendFolderTree(initialTree DirTreeMap) {
	var tree = make(DirTreeMap)
	prefixLength := len(globalSettings.Directory) + 1
	for key, value := range initialTree {
		if key == globalSettings.Directory {
			continue
		}
		fmt.Printf("key is: '%s', length is: %d\n", key, prefixLength)
		key = key[prefixLength:]
		tree[key] = value
	}

	// package up the tree
	jsonStr, err := json.Marshal(tree)
	if err != nil {
		panic(err)
	}
	buffer := bytes.NewBuffer(jsonStr)
	client := &http.Client{}

	data := []byte(globalSettings.ManagerCredentials)
	authHash := base64.StdEncoding.EncodeToString(data)

	for name, server := range GlobalServerMap {
		if name == globalSettings.Name {
			fmt.Printf("skipping %s\n", name)
			continue
		}
		fmt.Printf("sending tree update to: %s\n", name)

		url := "http://" + server.Address + "/tree/"
		fmt.Printf("Posting folder tree to: %s\n", url)

		req, err := http.NewRequest("POST", url, buffer)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Basic "+authHash)

		resp, err := client.Do(req)
		if err != nil {
			fmt.Printf("we encountered an error!\n%s\n", err)
			//panic(err)
			continue
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

		var remoteTreePreTranslate DirTreeMap
		err := decoder.Decode(&remoteTreePreTranslate)
		if err != nil {
			panic("bad json body")
		}

		var remoteTree = make(DirTreeMap)
		for key, value := range remoteTreePreTranslate {
			fmt.Printf("key is: '%s'\n", key)
			key = fmt.Sprintf("%s/%s", globalSettings.Directory, key)
			remoteTree[key] = value
		}

		fmt.Printf("Received a tree map from: %s\n%s\n", r.RemoteAddr, remoteTree)

		// let's compare to the current one we have.
		_, _, newPaths, deletedPaths, _ := checkForChanges(globalSettings.Directory, remoteTree)

		// update the paths to be consistent. Remember, everything is backwards since this is comparing the other side to us instead of two successive states
		// deletion will show up as a new path.

		// add folders that were created
		fmt.Printf("about to check for new folders: %s\n", deletedPaths)
		if len(deletedPaths) > 0 {
			fmt.Println("Paths were added on the other side. Create them here")
			for _, newPathName := range deletedPaths {
				fmt.Printf("pathname is: %s\n", newPathName)
				//if newPathName == "" {
				//	continue
				//}
				//newFullPath := fmt.Sprintf("%s/%s", globalSettings.Directory, newPathName)
				//fmt.Printf("new full path is: %s\n", newFullPath)
				err = os.Mkdir(newPathName, os.ModeDir+os.ModePerm)
				//todo figure out how to catch a path exists error.
				//if err != nil && err != os.ErrExist {
				//	panic(err)
				//}
			}
		}

		fmt.Printf("about to check for deletions: %s\n", newPaths)
		// check for folders that were deleted
		if len(newPaths) > 0 {
			fmt.Println("Paths were deleted on the other side. Delete them here")
			// Reverse sort the paths so the most specific is first. This allows us to get away without a recursive delete
			sort.Sort(sort.Reverse(sort.StringSlice(newPaths)))
			for _, pathName := range newPaths {
				if pathName == "" || globalSettings.Directory == "" || pathName == globalSettings.Directory {
					fmt.Sprintf("Trying to delete invalid path: %s\n", pathName)
					panic("Path information is not right. Do not delete")
				}

				err = os.Remove(pathName)
				if err != nil {
					panic(err)
				}
			}
		}




	}
}

func sendEvent(event *Event, address string, credentials string) {
	//url := "http://" + globalSettings.ManagerAddress + "/event/"
	url := "http://" + address + "/event/"
	fmt.Printf("Manager location: %s\n", url)

	jsonStr, _ := json.Marshal(event)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonStr))
	req.Header.Set("Content-Type", "application/json")

	//data := []byte(globalSettings.ManagerCredentials)
	data := []byte(credentials)
	fmt.Printf("Manager Credentials: %s\n", data)
	authHash := base64.StdEncoding.EncodeToString(data)
	req.Header.Add("Authorization", "Basic "+authHash)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Println(err)
	} else {
		defer resp.Body.Close()

		fmt.Println("response Status:", resp.Status)
		fmt.Println("response Headers:", resp.Header)
		body, _ := ioutil.ReadAll(resp.Body)
		fmt.Println("response Body:", string(body))
	}
}

type Page struct {
	Title string
	Body  []byte
}

var templates = template.Must(template.ParseFiles("home.html", "edit.html", "view.html"))
var validPath = regexp.MustCompile("^/(edit|save|view|home)/([a-zA-Z0-9]+)$")

func renderTemplate(w http.ResponseWriter, tmpl string, p *Page) {
	err := templates.ExecuteTemplate(w, tmpl+".html", p)
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
		http.Redirect(w, r, "/edit/"+title, http.StatusNotFound)
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
	http.Redirect(w, r, "/view/"+title, http.StatusFound)
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
