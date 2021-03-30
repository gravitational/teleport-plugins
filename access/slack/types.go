package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gravitational/trace"
)

type RequestData struct {
	User          string
	Roles         []string
	RequestReason string
}

type SlackDataMessage struct {
	ChannelID string
	Timestamp string
}

type SlackData = []SlackDataMessage

type PluginData struct {
	RequestData
	SlackData
}

func DecodePluginData(dataMap map[string]string) (data PluginData) {
	data.User = dataMap["user"]
	data.Roles = strings.Split(dataMap["roles"], ",")
	data.RequestReason = dataMap["request_reason"]
	if channelID, timestamp := dataMap["channel_id"], dataMap["timestamp"]; channelID != "" && timestamp != "" {
		data.SlackData = append(data.SlackData, SlackDataMessage{ChannelID: channelID, Timestamp: timestamp})
	}
	for _, encodedMsg := range strings.Split(dataMap["messages"], ",") {
		if parts := strings.Split(encodedMsg, "/"); len(parts) == 2 {
			data.SlackData = append(data.SlackData, SlackDataMessage{ChannelID: parts[0], Timestamp: parts[1]})
		}
	}
	return
}

func EncodePluginData(data PluginData) map[string]string {
	var encodedMessages []string
	for _, msg := range data.SlackData {
		encodedMessages = append(encodedMessages, fmt.Sprintf("%s/%s", msg.ChannelID, msg.Timestamp))
	}
	return map[string]string{
		"user":           data.User,
		"roles":          strings.Join(data.Roles, ","),
		"request_reason": data.RequestReason,
		"messages":       strings.Join(encodedMessages, ","),
	}
}

// Slack API types

type Response struct {
	Ok    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

type ChatMsgResponse struct {
	Response
	Channel   string `json:"channel"`
	Timestamp string `json:"ts"`
	Text      string `json:"text"`
}

type Msg struct {
	Type       string      `json:"type,omitempty"`
	Channel    string      `json:"channel,omitempty"`
	User       string      `json:"user,omitempty"`
	Username   string      `json:"username,omitempty"`
	Timestamp  string      `json:"ts,omitempty"`
	Text       string      `json:"text,omitempty"`
	BlockItems []BlockItem `json:"blocks,omitempty"`
}

type RespondMsg struct {
	Msg
	ReplaceOriginal bool `json:"replace_original,omitempty"`
}

type User struct {
	ID      string      `json:"id"`
	Name    string      `json:"name"`
	Profile UserProfile `json:"profile"`
}

type UserProfile struct {
	Email string `json:"email"`
}

// Slack API: blocks

type BlockType string

type Block interface {
	BlockType() BlockType
}

type BlockItem struct{ Block }

func (p *BlockItem) UnmarshalJSON(data []byte) error {
	var itemType struct {
		Type BlockType `json:"type"`
	}
	if err := json.Unmarshal(data, &itemType); err != nil {
		return trace.Wrap(err)
	}
	var block Block
	var err error
	switch itemType.Type {
	case ActionsBlock{}.BlockType():
		var val ActionsBlock
		err = trace.Wrap(json.Unmarshal(data, &val))
		block = val
	case ContextBlock{}.BlockType():
		var val ContextBlock
		err = trace.Wrap(json.Unmarshal(data, &val))
		block = val
	case DividerBlock{}.BlockType():
		var val DividerBlock
		err = trace.Wrap(json.Unmarshal(data, &val))
		block = val
	case FileBlock{}.BlockType():
		var val FileBlock
		err = trace.Wrap(json.Unmarshal(data, &val))
		block = val
	case HeaderBlock{}.BlockType():
		var val HeaderBlock
		err = trace.Wrap(json.Unmarshal(data, &val))
		block = val
	case ImageBlock{}.BlockType():
		var val ImageBlock
		err = trace.Wrap(json.Unmarshal(data, &val))
		block = val
	case InputBlock{}.BlockType():
		var val InputBlock
		err = trace.Wrap(json.Unmarshal(data, &val))
		block = val
	case SectionBlock{}.BlockType():
		var val SectionBlock
		err = trace.Wrap(json.Unmarshal(data, &val))
		block = val
	}
	if err != nil {
		return err
	}
	p.Block = block
	return nil
}

func (p BlockItem) MarshalJSON() ([]byte, error) {
	typeVal := string(p.BlockType())
	switch val := p.Block.(type) {
	case ActionsBlock:
		return json.Marshal(struct {
			Type string `json:"type"`
			ActionsBlock
		}{typeVal, val})
	case ContextBlock:
		return json.Marshal(struct {
			Type string `json:"type"`
			ContextBlock
		}{typeVal, val})
	case DividerBlock:
		return json.Marshal(struct {
			Type string `json:"type"`
			DividerBlock
		}{typeVal, val})
	case FileBlock:
		return json.Marshal(struct {
			Type string `json:"type"`
			FileBlock
		}{typeVal, val})
	case HeaderBlock:
		return json.Marshal(struct {
			Type string `json:"type"`
			HeaderBlock
		}{typeVal, val})
	case ImageBlock:
		return json.Marshal(struct {
			Type string `json:"type"`
			ImageBlock
		}{typeVal, val})
	case InputBlock:
		return json.Marshal(struct {
			Type string `json:"type"`
			InputBlock
		}{typeVal, val})
	case SectionBlock:
		return json.Marshal(struct {
			Type string `json:"type"`
			SectionBlock
		}{typeVal, val})
	default:
		return json.Marshal(val)
	}
}

func NewBlockItem(block Block) BlockItem {
	return BlockItem{block}
}

// Slack API: actions blocks

type ActionsBlock struct {
	Elements []json.RawMessage `json:"elements"`
	BlockID  string            `json:"block_id,omitempty"`
}

func (b ActionsBlock) BlockType() BlockType {
	return BlockType("actions")
}

// Slack API: context blocks

type ContextBlock struct {
	ElementItems []ContextElementItem `json:"elements"`
	BlockID      string               `json:"block_id,omitempty"`
}

func (b ContextBlock) BlockType() BlockType {
	return BlockType("context")
}

type ContextElementType string

type ContextElement interface {
	ContextElementType() ContextElementType
}

type ContextElementItem struct{ ContextElement }

func NewContextElementItem(element ContextElement) ContextElementItem {
	return ContextElementItem{element}
}

func (p *ContextElementItem) UnmarshalJSON(data []byte) error {
	var itemType struct {
		Type ContextElementType `json:"type"`
	}
	if err := json.Unmarshal(data, &itemType); err != nil {
		return trace.Wrap(err)
	}
	var element ContextElement
	var err error
	switch itemType.Type {
	case PlainTextObject{}.ContextElementType():
		var val PlainTextObject
		err = trace.Wrap(json.Unmarshal(data, &val))
		element = val
	case MarkdownObject{}.ContextElementType():
		var val MarkdownObject
		err = trace.Wrap(json.Unmarshal(data, &val))
		element = val
	}
	if err != nil {
		return err
	}
	p.ContextElement = element
	return nil
}

func (p ContextElementItem) MarshalJSON() ([]byte, error) {
	typeVal := string(p.ContextElementType())
	switch val := p.ContextElement.(type) {
	case PlainTextObject:
		return json.Marshal(struct {
			Type string `json:"type"`
			PlainTextObject
		}{typeVal, val})
	case MarkdownObject:
		return json.Marshal(struct {
			Type string `json:"type"`
			MarkdownObject
		}{typeVal, val})
	default:
		return json.Marshal(val)
	}
}

// Slack API: divider blocks

type DividerBlock struct {
	BlockID string `json:"block_id,omitempty"`
}

func (b DividerBlock) BlockType() BlockType {
	return BlockType("divider")
}

// Slack API: file blocks

type FileBlock struct {
	ExternalID string `json:"external_id"`
	Source     string `json:"source"`
	BlockID    string `json:"block_id,omitempty"`
}

func (b FileBlock) BlockType() BlockType {
	return BlockType("file")
}

// Slack API: header blocks

type HeaderBlock struct {
	Text    string `json:"text"`
	BlockID string `json:"block_id,omitempty"`
}

func (b HeaderBlock) BlockType() BlockType {
	return BlockType("header")
}

// Slack API: image blocks

type ImageBlock struct {
	ImageURL string          `json:"image_url"`
	AltText  string          `json:"alt_text,omitempty"`
	Title    json.RawMessage `json:"title,omitempty"`
	BlockID  string          `json:"block_id,omitempty"`
}

func (b ImageBlock) BlockType() BlockType {
	return BlockType("image")
}

// Slack API: input blocks

type InputBlock struct {
	Label          json.RawMessage `json:"label"`
	Element        json.RawMessage `json:"element"`
	DispatchAction bool            `json:"dispatch_action,omitempty"`
	BlockID        string          `json:"block_id,omitempty"`
	Hint           json.RawMessage `json:"hint,omitempty"`
	Optional       bool            `json:"optional,omitempty"`
}

func (b InputBlock) BlockType() BlockType {
	return BlockType("input")
}

// Slack API: section blocks

type SectionBlock struct {
	Text    TextObjectItem   `json:"text,omitempty"`
	BlockID string           `json:"block_id,omitempty"`
	Fields  []TextObjectItem `json:"fields,omitempty"`
}

func (b SectionBlock) BlockType() BlockType {
	return BlockType("section")
}

// Slack API: text objects

type TextObjectType string
type TextObject interface {
	TextObjectType() TextObjectType
	GetText() string
}

type TextObjectItem struct{ TextObject }

func (p *TextObjectItem) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		p.TextObject = nil
		return nil
	}

	var itemType struct {
		Type TextObjectType `json:"type"`
	}
	if err := json.Unmarshal(data, &itemType); err != nil {
		return trace.Wrap(err)
	}
	var object TextObject
	var err error
	switch itemType.Type {
	case PlainTextObject{}.TextObjectType():
		var val PlainTextObject
		err = trace.Wrap(json.Unmarshal(data, &val))
		object = val
	case MarkdownObject{}.TextObjectType():
		var val MarkdownObject
		err = trace.Wrap(json.Unmarshal(data, &val))
		object = val
	}
	if err != nil {
		return trace.Wrap(err)
	}
	p.TextObject = object
	return nil
}

func (p TextObjectItem) MarshalJSON() ([]byte, error) {
	typeVal := string(p.TextObjectType())
	switch val := p.TextObject.(type) {
	case PlainTextObject:
		return json.Marshal(struct {
			Type string `json:"type"`
			PlainTextObject
		}{typeVal, val})
	case MarkdownObject:
		return json.Marshal(struct {
			Type string `json:"type"`
			MarkdownObject
		}{typeVal, val})
	default:
		return json.Marshal(val)
	}
}

func NewTextObjectItem(object TextObject) TextObjectItem {
	return TextObjectItem{object}
}

type PlainTextObject struct {
	Text  string `json:"text"`
	Emoji bool   `json:"emoji,omitempty"`
}

func (t PlainTextObject) TextObjectType() TextObjectType {
	return TextObjectType("plain_text")
}

func (t PlainTextObject) ContextElementType() ContextElementType {
	return ContextElementType("plain_text")
}

func (t PlainTextObject) GetText() string {
	return t.Text
}

type MarkdownObject struct {
	Text     string `json:"text"`
	Verbatim bool   `json:"verbatim,omitempty"`
}

func (t MarkdownObject) TextObjectType() TextObjectType {
	return TextObjectType("mrkdwn")
}

func (t MarkdownObject) ContextElementType() ContextElementType {
	return ContextElementType("mrkdwn")
}

func (t MarkdownObject) GetText() string {
	return t.Text
}
