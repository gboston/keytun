// ABOUTME: Generates human-readable session codes like "keen-fox-42".
// ABOUTME: Uses embedded wordlists for adjectives and nouns with a random two-digit suffix.
package session

import (
	"fmt"
	"math/rand/v2"
)

var adjectives = []string{
	"aged", "airy", "apt", "arid", "ashy",
	"bold", "brave", "bright", "brisk", "brown",
	"buff", "busy", "calm", "cheap", "clean",
	"clear", "close", "cool", "cozy", "crisp",
	"crude", "curly", "damp", "dark", "deep",
	"deft", "dense", "dim", "drab", "dry",
	"dull", "dusk", "dusty", "eager", "eerie",
	"empty", "even", "fair", "faint", "fast",
	"fine", "firm", "flat", "fond", "free",
	"fresh", "full", "fuzzy", "glad", "gold",
	"good", "grand", "gray", "green", "grim",
	"gruff", "gusty", "happy", "hardy", "hazy",
	"heavy", "hefty", "high", "hollow", "icy",
	"idle", "inky", "ivory", "jolly", "jumpy",
	"just", "keen", "kind", "lanky", "large",
	"late", "lazy", "lean", "light", "live",
	"lofty", "lone", "long", "loud", "lucky",
	"lumpy", "lusty", "meek", "mild", "misty",
	"mixed", "moody", "mossy", "muddy", "murky",
	"musty", "muted", "neat", "nice", "nimble",
	"noble", "noisy", "numb", "odd", "open",
	"pale", "plain", "plump", "proud", "pure",
	"quick", "quiet", "rare", "raw", "ready",
	"rich", "rigid", "ripe", "rocky", "rough",
	"round", "ruddy", "rusty", "safe", "sandy",
	"sharp", "short", "shy", "silky", "slim",
	"slow", "small", "smart", "smoky", "soft",
	"soggy", "solid", "spare", "spiky", "steep",
	"still", "stout", "sturdy", "sunny", "swift",
	"tall", "tame", "thin", "tidy", "tight",
	"tiny", "tough", "true", "tumid", "twixt",
	"vast", "vivid", "warm", "wavy", "weak",
	"wide", "wild", "wispy", "wise", "witty",
	"woody", "wry", "young", "zany", "zesty",
}

var nouns = []string{
	"ant", "ape", "asp", "bass", "bat",
	"bear", "bee", "bird", "boar", "buck",
	"bull", "calf", "carp", "cat", "chub",
	"clam", "cod", "colt", "crab", "crane",
	"crow", "dace", "deer", "dingo", "dog",
	"dove", "drake", "duck", "eel", "elk",
	"fawn", "finch", "fish", "flea", "fly",
	"foal", "fox", "frog", "gnat", "gnu",
	"goat", "grub", "gull", "hare", "hawk",
	"hen", "hog", "ibis", "jay", "kite",
	"koi", "lamb", "lark", "lion", "loon",
	"lynx", "mare", "midge", "mink", "mole",
	"moose", "moth", "mule", "newt", "owl",
	"ox", "perch", "pike", "pony", "pug",
	"pup", "quail", "ram", "rat", "robin",
	"rook", "ruff", "seal", "shrew", "skunk",
	"slug", "snipe", "sole", "sparrow", "sprat",
	"squid", "stag", "stoat", "swan", "tapir",
	"tern", "toad", "trout", "viper", "vole",
	"wasp", "weasel", "whale", "wolf", "worm",
	"wren", "yak",
}

// Generate returns a random session code in the format "adjective-noun-NN"
// where NN is a two-digit number between 10 and 99.
func Generate() string {
	adj := adjectives[rand.IntN(len(adjectives))]
	noun := nouns[rand.IntN(len(nouns))]
	num := rand.IntN(90) + 10 // 10-99
	return fmt.Sprintf("%s-%s-%d", adj, noun, num)
}
