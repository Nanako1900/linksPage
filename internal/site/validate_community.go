package site

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Nanako1900/linksPage/internal/provider"
)

var cardKinds = map[CardKind]bool{CardDiscord: true, CardKOOK: true, CardQQGroup: true, CardWeChatGroup: true, CardStatic: true}

var memberDisplays = map[MemberDisplay]bool{MemberHidden: true, MemberAvatars: true, MemberAvatarsNames: true}

func (p *PublicPage) validateCommunity(id string, c CommunityView) error {
	switch {
	case c.ID != id || !uuidRe.MatchString(id) || !slugPatternRe.MatchString(c.Slug):
		return errors.New("invalid id or slug")
	case c.Provider != "discord" && c.Provider != "kook" && c.Provider != "static":
		return fmt.Errorf("unknown provider %q", c.Provider)
	case !cardKinds[c.Card] || !memberDisplays[c.MemberDisplay]:
		return errors.New("invalid card or memberDisplay")
	case len(c.Name) == 0 || c.Description == nil || c.UnavailableText == nil:
		return errors.New("name is required; description and unavailableText must not be null")
	case c.SharePath != PathCommunity+c.Slug:
		return errors.New("sharePath must be /c/{slug}")
	case c.InviteURL != nil && !strings.HasPrefix(*c.InviteURL, "https://"):
		return errors.New("inviteUrl must be https")
	}
	if _, ok := p.Platforms[c.Platform]; !ok {
		return errors.New("platform not in platforms")
	}
	if c.Icon != nil && validateImage(c.Icon, uploadPathRe) != nil && validateImage(c.Icon, proxyPathRe) != nil {
		return errors.New("icon must be a /media/u or /media/p image")
	}
	if err := validateCardExtras(c); err != nil {
		return err
	}
	return validateLive(c)
}

func validateCardExtras(c CommunityView) error {
	if c.Embed != nil && (c.Card != CardDiscord || c.Embed.Kind != "discord" || !guildEmbedSrcR.MatchString(c.Embed.Src)) {
		return errors.New("embed is only valid for discord with a widget src")
	}
	if (c.QQ != nil) != (c.Card == CardQQGroup) || (c.QQ != nil && !qqGroupRe.MatchString(c.QQ.GroupNumber)) {
		return errors.New("qq must be set exactly for qq-group cards with a 5–12 digit number")
	}
	if c.Card == CardWeChatGroup && c.QR == nil {
		return errors.New("wechat-group cards need a qr")
	}
	if c.QR != nil {
		img := ImageView{URL: c.QR.URL, Width: c.QR.Width, Height: c.QR.Height}
		if validateImage(&img, qrPathRe) != nil || c.QR.Note == nil {
			return errors.New("qr must be /media/q/{uuid} with size and a non-null note")
		}
	}
	if c.Contact != nil && (c.Contact.Value == "" || len(c.Contact.Label) == 0) {
		return errors.New("contact needs a label and a value")
	}
	return nil
}

func validateLive(c CommunityView) error {
	l := c.Live
	switch {
	case !l.State.Valid():
		return fmt.Errorf("live.state %q unknown", l.State)
	case l.Channels == nil || l.Users == nil:
		return errors.New("live.channels and live.users must not be null")
	case (l.Online == nil) != (l.OnlineSource == nil):
		return errors.New("live.onlineSource must be null exactly when online is null")
	case l.OnlineSource != nil && *l.OnlineSource != provider.SourceInvite &&
		*l.OnlineSource != provider.SourceWidget && *l.OnlineSource != provider.SourceBadge:
		return errors.New("live.onlineSource unknown")
	case l.JoinURL != nil && *l.JoinURL != PathGo+c.Slug:
		return errors.New("live.joinUrl must be /go/{slug}")
	case l.JoinURL != nil && (l.State == provider.StateUnavailable || c.Card == CardWeChatGroup):
		return errors.New("live.joinUrl must be null for unavailable and wechat-group cards")
	}
	return validateUsers(c.MemberDisplay, l.Users)
}

func validateUsers(mode MemberDisplay, users []UserView) error {
	if mode == MemberHidden && len(users) > 0 {
		return errors.New("live.users must be empty when memberDisplay is hidden")
	}
	for i, u := range users {
		switch {
		case mode == MemberAvatars && u.Name != nil:
			return fmt.Errorf("live.users[%d].name must be null in avatars mode", i)
		case mode == MemberAvatarsNames && u.Name == nil:
			return fmt.Errorf("live.users[%d].name is required in avatars_names mode", i)
		case u.Status != "online" && u.Status != "idle" && u.Status != "dnd":
			return fmt.Errorf("live.users[%d].status unknown", i)
		case u.AvatarURL != nil && !proxyPathRe.MatchString(*u.AvatarURL):
			return fmt.Errorf("live.users[%d].avatarUrl must be a /media/p path", i)
		}
	}
	return nil
}
