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

import "time"

// WaitFor - a helper function that will politely wait until the helper argument function evaluates to the value of waitingFor.
// this is great for waiting for the tracker to catch up to the filesystem in tests, for example.
func WaitFor(tracker *FilesystemTracker, folder string, waitingFor bool, helper func(tracker *FilesystemTracker, folder string) bool) bool {
	roundTrips := 0
	for {
		if waitingFor == helper(tracker, folder) {
			return true
		}

		if roundTrips > 50 {
			return false
		}
		roundTrips++

		time.Sleep(50 * time.Millisecond)
	}
}

// WaitForStorage - a helper function that will politely wait until the helper argument function evaluates to the value of waitingFor.
// this is great for waiting for a StorageTracker to catch up to the monitored system in tests, for example.
func WaitForStorage(tracker StorageTracker, folder string, waitingFor bool, helper func(tracker StorageTracker, folder string) bool) bool {
	roundTrips := 0
	for {
		if waitingFor == helper(tracker, folder) {
			return true
		}

		if roundTrips > 50 {
			return false
		}
		roundTrips++

		time.Sleep(50 * time.Millisecond)
	}
}

// startTest - log the fact that a test is starting and the test's name
func startTest(name string) {
	event := Event{Name: "startTest", Path: name, Source: globalSettings.Name}
	SendEvent(event, "")
}

// endTest - log the fact that a test is ending and the test's name
func endTest(name string) {
	event := Event{Name: "endTest", Path: name, Source: globalSettings.Name}
	SendEvent(event, "")
}
