package model

type ReplicatEntityValue struct {
	hash      string
	timestamp string
}

type ReplicatEntityMap struct {
}

var entityMap = make(map[string]ReplicatEntityValue)

func (m *ReplicatEntityMap) add(key string, value ReplicatEntityValue) {
	entityMap[key] = value
}

func (m *ReplicatEntityMap) size() int {
	return len(entityMap)
}
