package main

import "bazil.org/fuse"

func fuseMe() {
	fuse.Mount("/Users/ray/goProjects/src/github.com/bazil/zipfs/mnt/zipfs_test")

}

