package site

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

// Stored JSON schemas (database columns). Writers (seed, M2 admin API)
// and the page builder share these types; unknown keys are rejected (decode
// with DecodeStrict).

// CommunityDisplay is communities.display.
type CommunityDisplay struct {
	// Name is required (at least one locale).
	Name        LocalizedText `json:"name"`
	Description LocalizedText `json:"description,omitempty"`
	// MemberDisplay defaults to MemberAvatarsNames when empty.
	MemberDisplay MemberDisplay `json:"memberDisplay,omitempty"`
	// NameBlocklist hides members whose name contains any entry
	// (case-insensitive) in avatars_names mode.
	NameBlocklist []string `json:"nameBlocklist,omitempty"`
	// ShowChannels / ShowOnline default to true when nil.
	ShowChannels *bool `json:"showChannels,omitempty"`
	ShowOnline   *bool `json:"showOnline,omitempty"`
	// MemberLimit caps the listed members (default DefaultMemberLimit,
	// max MaxMemberLimit).
	MemberLimit int `json:"memberLimit,omitempty"`
	// Embed enables the Discord iframe facade (discord only).
	Embed bool `json:"embed,omitempty"`
	// QQGroupNumber is required for the qq-group platform (^\d{5,12}$).
	QQGroupNumber string `json:"qqGroupNumber,omitempty"`
	// Contact is the fallback contact (wechat-group, optional elsewhere).
	Contact *ContactView `json:"contact,omitempty"`
	// UnavailableText overrides the unavailable card text.
	UnavailableText LocalizedText `json:"unavailableText,omitempty"`
}

// Member list limits.
const (
	DefaultMemberLimit = 30
	MaxMemberLimit     = 100
)

// HeadingBlockData is page_blocks.data for kind "heading".
type HeadingBlockData struct {
	Text      LocalizedText `json:"text"`
	ShowCount bool          `json:"showCount,omitempty"`
}

// TextBlockData is page_blocks.data for kind "text" (Markdown subset).
type TextBlockData struct {
	Markdown LocalizedText `json:"markdown"`
}

// SocialRowBlockData is page_blocks.data for kind "social_row".
type SocialRowBlockData struct {
	LinkIDs []string `json:"linkIds"`
}

// DecodeStrict decodes JSON into v, rejecting unknown fields and trailing
// data.
func DecodeStrict(data []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("decode: %w", err)
	}
	if dec.More() {
		return errors.New("decode: trailing data")
	}
	return nil
}
