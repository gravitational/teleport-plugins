package main

import "github.com/gravitational/teleport-plugins/access/common"

type SlackMessageSlice []SlackMsg
type SlackDataMessageSet map[common.MessageData]struct{}

func (slice SlackMessageSlice) Len() int {
	return len(slice)
}

func (slice SlackMessageSlice) Less(i, j int) bool {
	if slice[i].Channel < slice[j].Channel {
		return true
	}
	return slice[i].Timestamp < slice[j].Timestamp
}

func (slice SlackMessageSlice) Swap(i, j int) {
	slice[i], slice[j] = slice[j], slice[i]
}

func (set SlackDataMessageSet) Add(msg common.MessageData) {
	set[msg] = struct{}{}
}

func (set SlackDataMessageSet) Contains(msg common.MessageData) bool {
	_, ok := set[msg]
	return ok
}
