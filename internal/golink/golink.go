// Package golink implements GET /go/{slug}: target resolution (doc 5.6)
// and the server-side guide pages used when JavaScript did not run
// (doc 5.7). /go never returns 429 and, in M1, never counts clicks (see
// ClickHook).
package golink

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/Nanako1900/linksPage/internal/provider"
	"github.com/Nanako1900/linksPage/internal/site"
	"github.com/Nanako1900/linksPage/internal/uaclass"
)

// ErrNotFound is returned by Source when no community or link has the slug.
var ErrNotFound = errors.New("golink: slug not found")

// SlugRe is the /go/{slug} (and /c/{slug}) slug syntax.
var SlugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

// Action is what /go/{slug} responds with.
type Action string

// Actions (see the decision table in docs/m1/contract.md).
const (
	// ActionRedirect: 302 to Decision.URL with Cache-Control: no-store.
	ActionRedirect Action = "redirect"
	// ActionOpenInBrowser: 200 guide page "tap ··· → open in browser" with
	// a copy-link button (the absolute /go URL), never a redirect.
	ActionOpenInBrowser Action = "open-in-browser"
	// ActionQQGroup: 200 page with the QQ group number (copy) and the QR
	// code when present (WeChat blocks qm.qq.com).
	ActionQQGroup Action = "qq-group"
	// ActionQRCode: 200 page with the QR code, note and fallback contact
	// (wechat-group).
	ActionQRCode Action = "qr-code"
	// ActionUnavailable: 200 "invite unavailable" page listing the other
	// communities; never redirects to a link known to be invalid.
	ActionUnavailable Action = "unavailable"
	// ActionNotFound: 404 page.
	ActionNotFound Action = "not-found"
)

// Decision is the resolver output.
type Decision struct {
	Action Action
	// URL is the redirect target (ActionRedirect only).
	URL string
	// CommunityID is set for community slugs ("" for links).
	CommunityID string
	// Slug echoes the request slug.
	Slug string
	// Community is the resolved community (nil for links and not-found);
	// guide pages render from it when the community is not on the page.
	Community *CommunityTarget
}

// CommunityTarget is what the resolver needs about a community
// (dbq.GetGoCommunity, published communities only, joined with the platform catalog).
type CommunityTarget struct {
	ID       string
	Slug     string
	Platform string
	// PlatformName is the localized platform name (may be empty).
	PlatformName site.LocalizedText
	// NeedsExternalBrowser comes from the platform.
	NeedsExternalBrowser bool
	// Join feeds provider.JoinTarget (shared with the page builder).
	Join provider.JoinInput
	// HasQR reports whether a QR code exists (its id for /media/q).
	HasQR bool
	QRID  string
	// QQGroupNumber is set for qq-group cards.
	QQGroupNumber string
	// Name is the admin display name (communities.display.name).
	Name site.LocalizedText
	// Contact is the optional fallback contact.
	Contact *site.ContactView
	// UnavailableText overrides the unavailable card text ({} = default).
	UnavailableText site.LocalizedText
}

// LinkTarget is a links row.
type LinkTarget struct {
	ID   string
	Slug string
	URL  string
}

// Source loads targets by slug; both return ErrNotFound when missing.
type Source interface {
	Community(ctx context.Context, slug string) (CommunityTarget, error)
	Link(ctx context.Context, slug string) (LinkTarget, error)
}

// Resolver applies the doc 5.6 target order and the doc 5.7 UA rules.
type Resolver struct {
	src Source
	now func() time.Time
}

// NewResolver returns a resolver; now defaults to time.Now.
func NewResolver(src Source, now func() time.Time) *Resolver {
	if now == nil {
		now = time.Now
	}
	return &Resolver{src: src, now: now}
}

// Resolve decides the response for slug and the visitor's UA class. Only
// storage failures are returned as errors; unknown or malformed slugs
// yield ActionNotFound.
func (r *Resolver) Resolve(ctx context.Context, slug string, ua uaclass.Class) (Decision, error) {
	notFound := Decision{Action: ActionNotFound, Slug: slug}
	if !SlugRe.MatchString(slug) {
		return notFound, nil
	}
	c, err := r.src.Community(ctx, slug)
	if err == nil {
		return r.decideCommunity(slug, c, ua), nil
	}
	if !errors.Is(err, ErrNotFound) {
		return Decision{}, fmt.Errorf("golink: load community: %w", err)
	}
	l, err := r.src.Link(ctx, slug)
	if errors.Is(err, ErrNotFound) {
		return notFound, nil
	}
	if err != nil {
		return Decision{}, fmt.Errorf("golink: load link: %w", err)
	}
	target, ok := safeRedirect(l.URL, linkSchemes)
	if !ok {
		return notFound, nil
	}
	return Decision{Action: ActionRedirect, URL: target, Slug: slug}, nil
}

// decideCommunity applies rows 3–7 of the contract decision table.
func (r *Resolver) decideCommunity(slug string, c CommunityTarget, ua uaclass.Class) Decision {
	d := Decision{Slug: slug, CommunityID: c.ID, Community: &c}
	if c.Join.Card == provider.CardWeChatGroup {
		d.Action = ActionQRCode
		return d
	}
	target := JoinURL(c, r.now())
	isQQ := c.Join.Card == provider.CardQQGroup
	switch {
	case isQQ && ua.InWeChat:
		d.Action = ActionQQGroup
	case target == "" && isQQ:
		d.Action = ActionQQGroup
	case target == "":
		d.Action = ActionUnavailable
	case c.NeedsExternalBrowser && ua.InApp():
		d.Action = ActionOpenInBrowser
	default:
		d.Action = ActionRedirect
		d.URL = target
	}
	return d
}
