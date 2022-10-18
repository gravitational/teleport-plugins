package main

import "github.com/gravitational/teleport-plugins/access/common"

type MessageSlice []DiscordMsg
type MessageSet map[common.MessageData]struct{}

func (slice MessageSlice) Len() int {
	return len(slice)
}

func (slice MessageSlice) Less(i, j int) bool {
	if slice[i].Channel < slice[j].Channel {
		return true
	}
	return slice[i].DiscordID < slice[j].DiscordID
}

func (slice MessageSlice) Swap(i, j int) {
	slice[i], slice[j] = slice[j], slice[i]
}

func (set MessageSet) Add(msg common.MessageData) {
	set[msg] = struct{}{}
}

func (set MessageSet) Contains(msg common.MessageData) bool {
	_, ok := set[msg]
	return ok
}
