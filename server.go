// Copyright 2016 Jacob Taylor jacob@ablox.io
// License: Apache2 - http://www.apache.org/licenses/LICENSE-2.0
package main

import "time"

type Event struct {
	Source      string
	Name        string
	Message     string
	Time        time.Time
	IsDirectory bool
}

var events = make([]Event, 0, 100)

type FileEvent struct {
	NodeID int32
	Name   string
	Time   time.Time
}
