package model

import (
	"fmt"
	"strconv"
	"testing"
)

func TestReplicatEntity(t *testing.T) {
	entityMap := ReplicatEntityMap{}

	for i := 0; i < 10; i++ {
		myValue := ReplicatEntityValue{"a", "b"}
		entityMap.add(strconv.Itoa(i), myValue)
	}
	fmt.Println(entityMap.size())
}
