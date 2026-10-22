package steer

// DisclosureVersion is the version of what the consent screen and the
// disclosure notices say steering does. A note reaches the agent only when
// consent.json holds a steer_disclosure at or above it (consent.Current,
// Technical spec 7.2.3), so a bump holds every note until the user has been
// shown the new text, and a downgrade keeps the consent a newer build
// recorded. Pinned by TestTRU48_DisclosureVersionIsPinned.
const DisclosureVersion = 2
