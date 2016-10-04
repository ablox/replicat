// Copyright 2016 Jacob Taylor jacob@ablox.io
// License: Apache2 - http://www.apache.org/licenses/LICENSE-2.0
package main

import (
	"fmt"
	"reflect"
	"sort"
	"testing"
)

func TestDirectoryStorage(t *testing.T) {
	tracker := new(FilesystemTracker)

	empty := make([]string, 0)
	folderList := tracker.ListFolders()

	if reflect.DeepEqual(empty, folderList) == false {
		t.Fatal(fmt.Sprintf("expected empty folder, found something: %v\n", folderList))
	}

	folderTemplate := []string{"A", "B", "C"}

	for _, folder := range folderTemplate {
		err := tracker.CreateFolder(folder)
		if err != nil {
			t.Fatal(err)
		}
	}

	folderList = tracker.ListFolders()

	sort.Strings(folderList)
	sort.Strings(folderTemplate)

	if reflect.DeepEqual(folderList, folderTemplate) == false {
		t.Fatal(fmt.Sprintf("Found: %v\nExpected: %v\n", folderList, folderTemplate))
	}


	err := tracker.DeleteFolder(folderTemplate[0])
	if err != nil {
		t.Fatal(err)
	}

	folderList = tracker.ListFolders()
	sort.Strings(folderList)

	folderTemplate = folderTemplate[1:]

	if reflect.DeepEqual(folderList, folderTemplate) == false {
		t.Fatal(fmt.Sprintf("Found: %v\nExpected: %v\n", folderList, folderTemplate))
	}



}
