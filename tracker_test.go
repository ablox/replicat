// Copyright 2016 Jacob Taylor jacob@ablox.io
// License: Apache2 - http://www.apache.org/licenses/LICENSE-2.0
package main

import "testing"

func TestEmptyDirectoryMovesInOutAround(t *testing.T) {
	trackerTestEmptyDirectoryMovesInOutAround()
}

func TestSmallFileMovesInOutAround(t *testing.T) {
	trackerTestSmallFileMovesInOutAround()
}

func TestSmallFileCreationAndRename(t *testing.T) {
	trackerTestSmallFileCreationAndRename()
}

func TestDirectoryCreation(t *testing.T) {
	trackerTestDirectoryCreation()
}

func TestNestedDirectoryCreation(t *testing.T) {
	trackerTestNestedDirectoryCreation()
}

func TestDirectoryStorage(t *testing.T) {
	trackerTestDirectoryStorage()
}

func TestFileChangeTrackerAutoCreateFolderAndCleanup(t *testing.T) {
	trackerTestFileChangeTrackerAutoCreateFolderAndCleanup()
}

func TestFileChangeTrackerAddFolders(t *testing.T) {
	trackerTestFileChangeTrackerAddFolders()
}
