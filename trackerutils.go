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

func startTest(name string) {
	event := Event{Name: "startTest", Path: name, Source: globalSettings.Name}
	SendEvent(event, "")
}

func endTest(name string) {
	event := Event{Name: "endTest", Path: name, Source: globalSettings.Name}
	SendEvent(event, "")
}
