// Copyright 2016 Jacob Taylor jacob@ablox.io
// License: Apache2 - http://www.apache.org/licenses/LICENSE-2.0
package main

import (
	"fmt"
	"math/rand"
	"time"
)

func main() {
	fmt.Println("replicat initializing....")
	rand.Seed(int64(time.Now().Nanosecond()))

	SetupCli()
	logOnlyHandler := LogOnlyChangeHandler{}
	tracker := FilesystemTracker{}
	tracker.init(globalSettings.Directory)
	var c ChangeHandler
	c = &logOnlyHandler
	tracker.watchDirectory(&c)

	fmt.Printf("replicat %s online....\n", globalSettings.Name)
	defer fmt.Println("End of line")

	bootstrapAndServe()

	for {
		time.Sleep(time.Millisecond * 500)
	}

}
