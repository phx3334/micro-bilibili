package util

func CacheMissIDs(hitMap map[uint]bool, ids []uint) []uint {
	var missed []uint
	for _, id := range ids {
		if !hitMap[id] {
			missed = append(missed, id)
		}
	}
	return missed
}
