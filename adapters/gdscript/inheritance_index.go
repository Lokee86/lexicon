package main

func (f *factSet) indexParent(source, target string) {
	if f.parentByOwnerID == nil {
		f.parentByOwnerID = make(map[string][]string)
	}
	if f.childByOwnerID == nil {
		f.childByOwnerID = make(map[string][]string)
	}
	for _, existing := range f.parentByOwnerID[source] {
		if existing == target {
			return
		}
	}
	f.parentByOwnerID[source] = append(f.parentByOwnerID[source], target)
	f.childByOwnerID[target] = append(f.childByOwnerID[target], source)
}

func (f *factSet) ensureChildIndex() {
	if f.childByOwnerID != nil {
		return
	}
	f.childByOwnerID = make(map[string][]string)
	for child, parents := range f.parentByOwnerID {
		for _, parent := range parents {
			f.childByOwnerID[parent] = append(f.childByOwnerID[parent], child)
		}
	}
}
