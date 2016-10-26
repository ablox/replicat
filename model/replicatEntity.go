package model

// ReplicatEntityValue - initial take on metadata for stored contents
type ReplicatEntityValue struct {
	hash      string
	timestamp string
}

// ReplicatEntityMap - Initial take on structure for storing the contents
type ReplicatEntityMap struct {
}

var entityMap = make(map[string]ReplicatEntityValue)

func (m *ReplicatEntityMap) add(key string, value ReplicatEntityValue) {
	entityMap[key] = value
}

func (m *ReplicatEntityMap) size() int {
	return len(entityMap)
}
