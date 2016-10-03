// Copyright 2016 Jacob Taylor jacob@ablox.io
// License: Apache2 - http://www.apache.org/licenses/LICENSE-2.0
package main

import (
	"testing"
	"fmt"
	"reflect"
)

func TestDirectoryStorage(t *testing.T) {
	tracker := new(FilesystemTracker)

	empty := make([]string, 0)
	folderList := tracker.ListFolders()

	if reflect.DeepEqual(empty, folderList) == false {
		t.Fatal(fmt.Sprintf("expected empty folder, found something: %v\n", folderList))
	}

	folderTemplate := [...]string{"A", "B", "C"}

	for _, folder := range folderTemplate {
		err := tracker.CreateFolder(folder)
		if err != nil {
			t.Fatal(err)
		}
	}

	folderList = tracker.ListFolders()

	fmt.Printf("first Type: %s, second Type: %s\n", folderList.Type(), folderTemplate.Type())


	folderList2 := make([]string, len(folderList))
	index := 0

	for _, k := range folderTemplate {
		folderList2[index] = k
		index++
	}


	if reflect.DeepEqual(folderList2, folderList) == false {
		t.Fatal(fmt.Sprintf("Expected: %v\nFound: %v\n", folderTemplate, folderList2))
	}


}
