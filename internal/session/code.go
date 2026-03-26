// ABOUTME: Generates human-readable session codes like "keen-fox-42".
// ABOUTME: Uses embedded wordlists for adjectives and nouns with a random two-digit suffix.
package session

import (
	"fmt"
	"math/rand/v2"
)

var adjectives = []string{
	"bold", "brave", "bright", "calm", "clean",
	"cool", "crisp", "dark", "deep", "eager",
	"fair", "fast", "fine", "firm", "fond",
	"free", "full", "glad", "gold", "good",
	"gray", "green", "happy", "high", "keen",
	"kind", "late", "lean", "light", "live",
	"long", "loud", "lucky", "mild", "neat",
	"nice", "noble", "odd", "open", "pale",
	"plain", "proud", "pure", "quick", "quiet",
	"rare", "raw", "ready", "rich", "ripe",
	"rough", "round", "safe", "sharp", "short",
	"shy", "slim", "slow", "small", "smart",
	"soft", "solid", "spare", "steep", "still",
	"stout", "swift", "tall", "tame", "thin",
	"tidy", "tight", "tiny", "tough", "true",
	"vast", "vivid", "warm", "weak", "wide",
	"wild", "wise", "witty", "young", "zany",
}

var nouns = []string{
	"ant", "ape", "bat", "bear", "bird",
	"buck", "bull", "calf", "cat", "clam",
	"cod", "colt", "crab", "crow", "deer",
	"dog", "dove", "duck", "eel", "elk",
	"fawn", "fish", "flea", "fly", "foal",
	"fox", "frog", "goat", "gull", "hare",
	"hawk", "hen", "hog", "jay", "kite",
	"lark", "lion", "lynx", "mink", "mole",
	"moth", "mule", "newt", "owl", "ox",
	"paw", "pike", "pony", "pug", "ram",
	"rat", "rook", "seal", "slug", "snag",
	"sole", "stag", "swan", "toad", "trout",
	"vole", "wasp", "worm", "wren", "yak",
}

// Generate returns a random session code in the format "adjective-noun-NN"
// where NN is a two-digit number between 10 and 99.
func Generate() string {
	adj := adjectives[rand.IntN(len(adjectives))]
	noun := nouns[rand.IntN(len(nouns))]
	num := rand.IntN(90) + 10 // 10-99
	return fmt.Sprintf("%s-%s-%d", adj, noun, num)
}
