// Package replicat is a server for n way synchronization of content (rsync for the cloud).
// More information at: http://replic.at
// Copyright 2016 Jacob Taylor jacob@ablox.io
// License: Apache2 - http://www.apache.org/licenses/LICENSE-2.0
package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func testIntegration(t *testing.T) {
	//buildApps()
	startWebcat()
	dirA := startReplicat("nodeA")
	dirB := startReplicat("nodeB")
	//defer os.RemoveAll(dirA) // clean up
	//defer os.RemoveAll(dirB) // clean up

	os.MkdirAll(filepath.Join(dirA, "a/b/c"), os.ModePerm)

	time.Sleep(1 * time.Second)
	fmt.Println(dirA)
	fmt.Println(dirB)

	_, err := ioutil.ReadDir(filepath.Join(dirB, "a/b/c"))
	if err != nil {
		log.Fatal(err)
		os.Exit(1)
	}
}

func startWebcat() {
	go func() {
		err := os.Chdir("../webcat")
		printError(err)
		cmd := exec.Command("go", "run", "main.go")
		output, err := cmd.CombinedOutput()
		printError(err)
		printOutput(output)
	}()
}

func startReplicat(name string) string {
	dir, err := ioutil.TempDir("", "")
	if err != nil {
		log.Fatal(err)
	}

	go func() {
		cmd := exec.Command("go", "run", "main.go", "--directory", dir, "--name", name)
		output, err := cmd.CombinedOutput()
		printError(err)
		printOutput(output)
	}()

	return dir
}

func buildApps() {
	os.Chdir("../webcat")
	cmd := exec.Command("/usr/local/bin/go build -o webcat github.com/ablox/webcat")
	output, err := cmd.CombinedOutput()
	printError(err)
	printOutput(output)
	os.Chdir("../replicat")
	cmd = exec.Command("/usr/local/bin/go build -o replicat github.com/ablox/replicat")
	output, err = cmd.CombinedOutput()
	printError(err)
	printOutput(output)
}

func printError(err error) {
	if err != nil {
		os.Stderr.WriteString(fmt.Sprintf("==> Error: %s\n", err.Error()))
	}
}

func printOutput(outs []byte) {
	if len(outs) > 0 {
		fmt.Printf("==> Output: %s\n", string(outs))
	}
}
