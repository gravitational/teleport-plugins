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
