package config

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"
)

var (
	usernameRe   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
	providerIDRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)
)

// hashPasswordHint tells operators how to produce a valid hash. The
// `hash-password` subcommand arrives in M2a, so the hint does not point
// at a command that does not exist yet.
const hashPasswordHint = "generate an argon2id PHC string ($argon2id$v=19$m=...,t=...,p=...$salt$hash) with m<=65536, t<=5, p<=2"

func validRole(r Role) bool { return r == RoleOwner || r == RoleEditor }

func validateAuth(a Auth) ([]string, []error) {
	var warns []string
	var errs []error
	errs = append(errs, validateSession(a.Session)...)

	owners := 0
	if a.Local.Enabled {
		w, e, o := validateLocalUsers(a.Local.Users)
		warns, errs, owners = append(warns, w...), append(errs, e...), owners+o
	}
	types, provErrs := validateOAuthProviders(a.OAuth.Providers)
	errs = append(errs, provErrs...)
	w, e, o := validateOAuthAdmins(a.OAuth.Admins, types)
	warns, errs, owners = append(warns, w...), append(errs, e...), owners+o

	admins := len(a.OAuth.Admins)
	if a.Local.Enabled {
		admins += len(a.Local.Users)
	}
	switch {
	case admins == 0:
		warns = append(warns, "no administrators are configured; the public page works but /admin cannot be used (see auth.local.users / auth.oauth.admins)")
	case owners == 0:
		warns = append(warns, "no administrator has the owner role")
	}
	return warns, errs
}

func validateSession(s Session) []error {
	var errs []error
	for key, d := range map[string]int64{
		"idle_timeout": int64(s.IdleTimeout), "absolute_timeout": int64(s.AbsoluteTimeout),
		"attr_max_age": int64(s.AttrMaxAge), "reauth_window": int64(s.ReauthWindow),
	} {
		if d <= 0 {
			errs = append(errs, fmt.Errorf("auth.session.%s: must be a positive duration", key))
		}
	}
	return sortErrors(errs)
}

func validateLocalUsers(users []LocalUser) ([]string, []error, int) {
	var warns []string
	var errs []error
	seen := map[string]bool{}
	owners := 0
	for i, u := range users {
		path := fmt.Sprintf("auth.local.users[%d]", i)
		if !usernameRe.MatchString(u.Username) {
			errs = append(errs, fmt.Errorf("%s.username: must match %s", path, usernameRe.String()))
		} else if key := strings.ToLower(u.Username); seen[key] {
			errs = append(errs, fmt.Errorf("%s.username: duplicate local user", path))
		} else {
			seen[key] = true
		}
		if !validRole(u.Role) {
			errs = append(errs, fmt.Errorf("%s.role: must be owner or editor", path))
		} else if u.Role == RoleOwner {
			owners++
		}
		if !u.PasswordHash.IsSet() {
			continue // reported during file resolution
		}
		params, err := ParseArgon2idPHC(u.PasswordHash.Reveal())
		switch {
		case errors.Is(err, ErrPHCLimits):
			errs = append(errs, fmt.Errorf("%s.password_hash: %w; %s", path, ErrPHCLimits, hashPasswordHint))
		case err != nil:
			errs = append(errs, fmt.Errorf("%s.password_hash: %w; %s", path, ErrPHCFormat, hashPasswordHint))
		case params.BelowRecommended():
			warns = append(warns, fmt.Sprintf("%s.password_hash: argon2id parameters are below the recommended strength; %s", path, hashPasswordHint))
		}
	}
	return warns, errs, owners
}

// validateOAuthProviders returns a map of provider id to type for the
// valid providers.
func validateOAuthProviders(providers []OAuthProvider) (map[string]string, []error) {
	types := map[string]string{}
	var errs []error
	for i, p := range providers {
		path := fmt.Sprintf("auth.oauth.providers[%d]", i)
		switch {
		case !providerIDRe.MatchString(p.ID):
			errs = append(errs, fmt.Errorf("%s.id: must match %s", path, providerIDRe.String()))
		case types[p.ID] != "":
			errs = append(errs, fmt.Errorf("%s.id: duplicate provider id", path))
		default:
			types[p.ID] = p.Type
		}
		switch p.Type {
		case "github", "discord":
		case "oidc":
			if err := validateIssuer(p.Issuer); err != nil {
				errs = append(errs, fmt.Errorf("%s.issuer: %w", path, err))
			}
		default:
			errs = append(errs, fmt.Errorf("%s.type: must be oidc, github or discord", path))
		}
		if p.ClientID == "" {
			errs = append(errs, fmt.Errorf("%s.client_id: required", path))
		}
	}
	return types, errs
}

func validateIssuer(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return errors.New("must be an absolute URL")
	}
	host := u.Hostname()
	if u.Scheme == "https" || (u.Scheme == "http" && (host == "localhost" || host == "127.0.0.1" || host == "::1")) {
		return nil
	}
	return errors.New("must use https")
}

func validateOAuthAdmins(admins []OAuthAdmin, types map[string]string) ([]string, []error, int) {
	var warns []string
	var errs []error
	seen := map[string]bool{}
	owners := 0
	for i, a := range admins {
		path := fmt.Sprintf("auth.oauth.admins[%d]", i)
		ptype, known := types[a.Provider]
		if !known {
			errs = append(errs, fmt.Errorf("%s.provider: does not reference a configured provider id", path))
		}
		kind, value, n := adminMatcher(a)
		if n != 1 {
			errs = append(errs, fmt.Errorf("%s: set exactly one of subject, email or group", path))
		} else {
			key := a.Provider + "\x00" + kind + "\x00" + value
			if seen[key] {
				errs = append(errs, fmt.Errorf("%s: duplicate admin identity", path))
			}
			seen[key] = true
			if kind == "email" && (ptype == "github" || ptype == "discord") {
				warns = append(warns, fmt.Sprintf("%s: email matching for %s is weaker than subject; prefer subject", path, ptype))
			}
		}
		if !validRole(a.Role) {
			errs = append(errs, fmt.Errorf("%s.role: must be owner or editor", path))
		} else if a.Role == RoleOwner {
			owners++
		}
	}
	return warns, errs, owners
}

// adminMatcher returns the configured matcher kind/value and how many
// matchers were set.
func adminMatcher(a OAuthAdmin) (string, string, int) {
	kind, value, n := "", "", 0
	for _, m := range []struct{ k, v string }{{"subject", a.Subject}, {"email", strings.ToLower(a.Email)}, {"group", a.Group}} {
		if m.v != "" {
			kind, value = m.k, m.v
			n++
		}
	}
	return kind, value, n
}

func sortErrors(errs []error) []error {
	out := slices.Clone(errs)
	slices.SortFunc(out, func(a, b error) int { return strings.Compare(a.Error(), b.Error()) })
	return out
}
