package main

type MsgSlice []Msg
type MsgSet map[Msg]struct{}

func (slice MsgSlice) Len() int {
	return len(slice)
}

func (slice MsgSlice) Less(i, j int) bool {
	return slice[i].RecipientID < slice[j].RecipientID
}

func (slice MsgSlice) Swap(i, j int) {
	slice[i], slice[j] = slice[j], slice[i]
}

func (set MsgSet) Add(msg Msg) {
	set[msg] = struct{}{}
}

func (set MsgSet) Contains(msg Msg) bool {
	_, ok := set[msg]
	return ok
}
