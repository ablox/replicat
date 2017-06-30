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
	"fmt"
	log "github.com/sirupsen/logrus"
	"io/ioutil"
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
		os.Stderr.WriteString(fmt.Sprintf("==> Error: %s", err.Error()))
	}
}

func printOutput(outs []byte) {
	if len(outs) > 0 {
		fmt.Printf("==> Output: %s", string(outs))
	}
}
