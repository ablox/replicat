// Copyright 2016 Jacob Taylor jacob@ablox.io
// License: Apache2 - http://www.apache.org/licenses/LICENSE-2.0
package main

import (
	"fmt"
	"log"
	"math/rand"
	"time"
	"github.com/ablox/replicat"
)

func main() {
	// Set the flags on the logger to get better accuracy
	log.SetFlags(log.LstdFlags | log.Lmicroseconds | log.Lshortfile)

	fmt.Println("replicat initializing....")
	rand.Seed(int64(time.Now().Nanosecond()))

	SetupCli()

	globalSettings := replicat.GetGlobalSettings()
	replicat.BootstrapAndServe(globalSettings.Nodes[globalSettings.Name].Address)
	fmt.Printf("replicat %s online....\n", globalSettings.Name)
	defer fmt.Println("End of line")

	// keep this process running until it is shut down
	for {
		time.Sleep(time.Second * 60)
	}

}
