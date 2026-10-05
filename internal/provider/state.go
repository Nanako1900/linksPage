package provider

import "time"

// Stale-after rules: a live or degraded snapshot whose last successful
// fetch is older than StaleAfter(interval) is shown as stale even if the
// refresh job did not record a failure (leader down, job stuck). Applied by
// the page builder (internal/site effectiveState) on every rebuild.
const (
	// staleFactor is how many refresh intervals may pass without success.
	staleFactor = 3
	// minStaleAfter avoids flapping with very short intervals.
	minStaleAfter = 15 * time.Minute
)

// StaleAfter returns the maximum age of live data for a community
// refreshed every interval.
func StaleAfter(interval time.Duration) time.Duration {
	return max(staleFactor*interval, minStaleAfter)
}

// EffectiveState applies the stale-after rule to a stored state. lastOK is
// provider_snapshots.last_ok_at (nil when never successful).
func EffectiveState(s State, lastOK *time.Time, interval time.Duration, now time.Time) State {
	if s != StateLive && s != StateDegraded {
		return s
	}
	if lastOK == nil || now.Sub(*lastOK) > StaleAfter(interval) {
		return StateStale
	}
	return s
}
