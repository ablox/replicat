// Copyright 2016 Jacob Taylor jacob@ablox.io
// License: Apache2 - http://www.apache.org/licenses/LICENSE-2.0
package main

import (
	"fmt"
)

func main() {
	fmt.Printf("replicat online....")

	fuseMe()
	defer fmt.Printf("End of line\n")
}


