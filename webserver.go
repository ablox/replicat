package main

import (
	"net/http"
	"io/ioutil"
	"html/template"
	"regexp"
	"fmt"
	"github.com/pubnub/go/messaging"
	"strings"
	"encoding/json"
)

type Page struct {
	Title string
	Body  []byte
}

var templates = template.Must(template.ParseFiles("edit.html", "view.html"))
var validPath = regexp.MustCompile("^/(edit|save|view)/([a-zA-Z0-9]+)$")

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

func viewHandler(w http.ResponseWriter, r *http.Request, title string) {
	p, err := loadPage(title)
	if err != nil {
		http.Redirect(w, r, "/edit/" + title, http.StatusNotFound)
		return
	}
	renderTemplate(w, "view", p)
}

func editHandler(w http.ResponseWriter, r *http.Request, title string) {
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

func main() {
	fmt.Println("PubNub SDK for go;", messaging.VersionInfo())
	pubnub := messaging.NewPubnub("pub-c-fc75596b-c9cf-40c9-844f-f31d7842419c", "sub-c-a76871e8-6692-11e6-879b-0619f8945a4f", "sec-c-ZWNlNDRlZGYtODIyNi00ZjZhLWE5ZGUtM2FlNmYxNDk1NjQy", "", false, "")
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

	http.HandleFunc("/view/", makeHandler(viewHandler))
	http.HandleFunc("/edit/", makeHandler(editHandler))
	http.HandleFunc("/save/", makeHandler(saveHandler))
	http.ListenAndServe(":8080", nil)
}