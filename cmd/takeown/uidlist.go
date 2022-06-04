package main

type UID uint32

type UIDList []UID

func (u UIDList) Has(uid UID) bool {
	if u == nil {
		return false
	}
	for _, x := range u {
		if uid == x {
			return true
		}
	}
	return false
}

// Merge adds the second UIDList to this UIDList, returning a new merged list.
func (a UIDList) Merge(b UIDList) UIDList {
	result := UIDList{}
	if a == nil {
		a = UIDList{}
	}
	if b == nil {
		b = UIDList{}
	}
	added := make(map[UID]bool)
	for _, uid := range append(a, b...) {
		if ok := added[uid]; ok {
			continue
		}
		result = append(result, uid)
		added[uid] = true
	}
	return result
}

// Remove returns a list with all elements of b removed from this UIDList.
func (a UIDList) Remove(b UIDList) UIDList {
	result := UIDList{}
	if a == nil {
		a = UIDList{}
	}
	if b == nil {
		b = UIDList{}
	}
	present := make(map[UID]bool)
	for _, uid := range b {
		present[uid] = true
	}
	for _, uid := range a {
		if ok := present[uid]; ok {
			continue
		}
		result = append(result, uid)
	}
	return result
}

// Equal returns true if both UIDLists are equal.
func (a UIDList) Equal(b UIDList) bool {
	if a == nil {
		a = UIDList{}
	}
	if b == nil {
		b = UIDList{}
	}
	if len(a) != len(b) {
		return false
	}
	equal := true
	for n := range a {
		if a[n] != b[n] {
			equal = false
		}
	}
	return equal
}
