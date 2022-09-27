/*
Copyright 2022 Gravitational, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

type DiscordResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message,omitempty"`
}

type ChatMsgResponse struct {
	DiscordResponse
	Channel   string `json:"channel"`
	Text      string `json:"text"`
	DiscordID string `json:"id"`
}

type Msg struct {
	Type      string `json:"type,omitempty"`
	Channel   string `json:"channel,omitempty"`
	User      string `json:"user,omitempty"`
	Username  string `json:"username,omitempty"`
	Timestamp string `json:"ts,omitempty"`
	ThreadTs  string `json:"thread_ts,omitempty"`
}

type DiscordMsg struct {
	Msg
	Text string `json:"content,omitempty"`
}
