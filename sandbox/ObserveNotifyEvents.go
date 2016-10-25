package main

import (
	"fmt"
	"github.com/rjeczalik/notify"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

var done = make(chan bool)
var testDirectory = "foo_" + strconv.FormatInt(time.Now().UnixNano(), 10)
var testDirPath1 = filepath.Join(testDirectory, "mydir1")
var testTextFilePath1 = filepath.Join(testDirectory, "abc.txt")
var testTextFilePath2 = filepath.Join(testDirectory, "def.txt")

func main() {
	log.Println("Observe Notify Events")
	log.Println("Create new test directory... ")
	os.Mkdir(testDirectory, os.ModePerm)

	go monitor()
	time.Sleep(time.Second * 1)

	// Test case
	log.Println("Create new file " + testTextFilePath1)
	f, err := os.Create(testTextFilePath1)
	if err != nil {
		log.Fatal(err)
	}
	f.Close()
	for !<-done {
		time.Sleep(time.Second * 1)
	}

	// Test case
	log.Println("Create new directory " + testDirPath1)
	err = os.Mkdir(testDirPath1, os.ModeDir|os.ModePerm)
	if err != nil {
		log.Fatal(err)
	}
	for !<-done {
		time.Sleep(time.Second * 1)
	}

	// Test case
	log.Println("dump text to new file " + testTextFilePath2)
	d1 := []byte("hello\ngo\n")
	err = ioutil.WriteFile(testTextFilePath2, d1, 0644)
	if err != nil {
		log.Fatal(err)
	}
	for !<-done {
		time.Sleep(time.Second * 1)
	}

	// Test case
	log.Println("append text to file " + testTextFilePath1)
	f, err = os.OpenFile(testTextFilePath1, os.O_APPEND|os.O_WRONLY, os.ModeAppend)
	if err != nil {
		log.Fatal(err)
	}
	d2 := []byte("hello\ngo\n")
	n2, err := f.Write(d2)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("wrote %d bytes\n", n2)
	f.Close()
	for !<-done {
		time.Sleep(time.Second * 1)
	}

	os.RemoveAll(testDirectory)
}

func monitor() {
	c := make(chan notify.EventInfo, 1000)

	for {
		if err := notify.Watch(testDirectory, c, notify.All); err != nil {
			log.Fatal(err)
		}
		defer notify.Stop(c)

		ei := <-c
		log.Println(ei.Event())
		done <- true
	}
}
