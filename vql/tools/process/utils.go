package process

func reverse(l []*ProcessEntry) []*ProcessEntry {
	res := make([]*ProcessEntry, 0, len(l))
	for i := len(l) - 1; i >= 0; i-- {
		res = append(res, l[i])
	}

	return res
}

func id_seen(id string, l []*ProcessEntry) bool {
	for _, i := range l {
		if i.RealId == id {
			return true
		}
	}

	return false
}
