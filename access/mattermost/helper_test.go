package main

type MattermostPostSlice []Post
type MattermostDataPostSet map[MattermostDataPost]struct{}

func (slice MattermostPostSlice) Len() int {
	return len(slice)
}

func (slice MattermostPostSlice) Less(i, j int) bool {
	if slice[i].ChannelID < slice[j].ChannelID {
		return true
	}
	return slice[i].ID < slice[j].ID
}

func (slice MattermostPostSlice) Swap(i, j int) {
	slice[i], slice[j] = slice[j], slice[i]
}

func (set MattermostDataPostSet) Add(msg MattermostDataPost) {
	set[msg] = struct{}{}
}

func (set MattermostDataPostSet) Contains(msg MattermostDataPost) bool {
	_, ok := set[msg]
	return ok
}
