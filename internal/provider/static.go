package provider

// ProviderStatic is the communities.provider value of platforms without a
// live data source (QQ/WeChat groups, Telegram, generic links, custom
// platforms). They are never fetched by the refresh job.
const ProviderStatic = "static"

// StaticState is the card state of a static community: wechat-group cards
// only show a QR code, every other static card is StateStatic.
func StaticState(card string) State {
	if card == CardWeChatGroup {
		return StateQROnly
	}
	return StateStatic
}

// ProviderFor returns the communities.provider value for platform p
// ("discord", "kook" or "static").
func ProviderFor(p Platform) string {
	if p.Provider == "" {
		return ProviderStatic
	}
	return p.Provider
}
