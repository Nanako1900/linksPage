package webui

import (
	"fmt"
	"strings"
	"time"

	"github.com/Nanako1900/linksPage/internal/provider"
	"github.com/Nanako1900/linksPage/internal/site"
)

// Fallback list limits (the React card shows everything).
const (
	maxFallbackChannels = 12
	maxFallbackUsers    = 10
)

// cardView is one community card in the fallback markup.
type cardView struct {
	Slug, State, Card string
	Name, Platform    string
	Description       string
	Icon              *site.ImageView
	Stat              string
	Updated           string
	UpdatedISO        string
	Channels          []string
	Users             string
	QQLabel, QQNumber string
	QR                *qrView
	Contact           *contactView
	Notice            string
	JoinURL, JoinText string
}

type qrView struct {
	URL           string
	Width, Height int
	Alt, Note     string
	Hint          string
}

type contactView struct{ Label, Value string }

func cardFor(pc pageCtx, c site.CommunityView) *cardView {
	v := &cardView{
		Slug:        c.Slug,
		State:       string(c.Live.State),
		Card:        string(c.Card),
		Name:        pc.text(c.Name),
		Description: pc.text(c.Description),
		Icon:        c.Icon,
		Stat:        statLine(pc.txt, c.Live),
		Channels:    channelNames(c.Live.Channels),
		Users:       userSummary(pc.txt, c),
		Notice:      notice(pc, c),
	}
	if p, ok := pc.pub.Platforms[c.Platform]; ok {
		v.Platform = pc.text(p.Name)
	}
	if c.Live.State == provider.StateStale && c.Live.UpdatedAt != nil {
		t := c.Live.UpdatedAt.UTC()
		v.Updated = fmt.Sprintf(pc.txt.UpdatedAt, t.Format("2006-01-02 15:04 UTC"))
		v.UpdatedISO = t.Format(time.RFC3339)
	}
	if c.QQ != nil {
		v.QQLabel, v.QQNumber = pc.txt.GroupNumber, c.QQ.GroupNumber
	}
	v.QR = qrFor(pc, c)
	if c.Contact != nil {
		v.Contact = &contactView{Label: pc.text(c.Contact.Label), Value: c.Contact.Value}
	}
	if c.Live.JoinURL != nil {
		v.JoinURL, v.JoinText = *c.Live.JoinURL, pc.txt.Join
		if c.Card == site.CardQQGroup {
			v.JoinText = pc.txt.JoinGroup
		}
	}
	return v
}

func qrFor(pc pageCtx, c site.CommunityView) *qrView {
	if c.QR == nil {
		return nil
	}
	q := &qrView{
		URL: c.QR.URL, Width: c.QR.Width, Height: c.QR.Height,
		Alt:  fmt.Sprintf(pc.txt.QRAlt, pc.text(c.Name)),
		Note: pc.text(c.QR.Note),
	}
	if c.Card == site.CardWeChatGroup {
		q.Hint = pc.txt.QRHint
	}
	return q
}

// statLine is "● 13 online · 125 members" (the dot always has text).
func statLine(txt uiStrings, l site.LiveView) string {
	switch {
	case l.Online != nil && l.Members != nil:
		return fmt.Sprintf(txt.OnlineMembers, *l.Online, *l.Members)
	case l.Online != nil:
		return fmt.Sprintf(txt.OnlineOnly, *l.Online)
	case l.Members != nil:
		return fmt.Sprintf(txt.MembersOnly, *l.Members)
	}
	return ""
}

func channelNames(chs []site.ChannelView) []string {
	out := make([]string, 0, min(len(chs), maxFallbackChannels))
	for _, ch := range chs[:min(len(chs), maxFallbackChannels)] {
		out = append(out, ch.Name)
	}
	return out
}

// userSummary lists online member names (avatars_names mode only; the
// fallback has no avatars).
func userSummary(txt uiStrings, c site.CommunityView) string {
	if c.MemberDisplay != site.MemberAvatarsNames {
		return ""
	}
	names := make([]string, 0, maxFallbackUsers)
	for _, u := range c.Live.Users {
		if u.Name != nil && len(names) < maxFallbackUsers {
			names = append(names, *u.Name)
		}
	}
	if len(names) == 0 {
		return ""
	}
	out := txt.OnlineList + strings.Join(names, txt.ListSeparator)
	if rest := len(c.Live.Users) - len(names); rest > 0 {
		out += fmt.Sprintf(txt.MoreUsers, rest)
	}
	return out
}

// notice is the card's status text: unavailable text, "invite
// unavailable" when a joinable card has no target, or the pending hint.
func notice(pc pageCtx, c site.CommunityView) string {
	ps := pc.pub.Site
	switch {
	case c.Live.State == provider.StateUnavailable:
		if t := pc.text(c.UnavailableText); t != "" {
			return t
		}
		return copyText(ps.Copy, pc.locale, ps.DefaultLocale, site.CopyCommunityUnavailable, pc.txt)
	case c.Live.JoinURL == nil && c.Card != site.CardQQGroup && c.Card != site.CardWeChatGroup:
		return copyText(ps.Copy, pc.locale, ps.DefaultLocale, site.CopyInviteUnavailable, pc.txt)
	case c.Live.State == provider.StatePending:
		return pc.txt.Pending
	}
	return ""
}
