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
	"math/rand"
	"time"
)

func main() {
	// Set the flags on the logger to get better accuracy
	log.SetFormatter(&log.TextFormatter{})
	//log.SetFlags(log.LstdFlags | log.Lmicroseconds | log.Lshortfile)

	fmt.Println("replicat initializing....")
	rand.Seed(int64(time.Now().Nanosecond()))

	SetupCli()

	globalSettings := GetGlobalSettings()
	BootstrapAndServe(globalSettings.Address)
	fmt.Printf("replicat %s online....", globalSettings.Name)
	defer fmt.Println("End of line")

	// keep this process running until it is shut down
	for {
		time.Sleep(time.Second * 60)
	}

}
