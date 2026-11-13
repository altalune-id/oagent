package extract

// ChildrenRunning reports how many child slots are taken.
func ChildrenRunning() int { return len(childSlots) }
