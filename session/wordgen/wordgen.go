package wordgen

import (
	"crypto/rand"
	"fmt"
	"math/big"
)

// adjectives is a curated list of simple, memorable adjectives
var adjectives = []string{
	"azure", "bold", "calm", "daring", "eager",
	"fleet", "gentle", "happy", "jolly", "kind",
	"lively", "merry", "noble", "proud", "quick",
	"quiet", "rapid", "serene", "swift", "wise",
	"bright", "clever", "cosmic", "crystal", "divine",
	"epic", "fair", "golden", "honest", "humble",
	"iron", "jade", "keen", "lunar", "mystic",
	"omega", "pearl", "royal", "sacred", "silver",
	"solar", "stellar", "stoic", "supreme", "tiger",
	"ultra", "valiant", "vivid", "zealous", "zen",
}

// nouns is a curated list of animals and memorable objects
var nouns = []string{
	"aardvark", "badger", "cheetah", "dolphin", "eagle",
	"falcon", "gazelle", "hawk", "iguana", "jaguar",
	"koala", "leopard", "mantis", "narwhal", "otter",
	"panther", "quail", "raven", "shark", "tiger",
	"urchin", "viper", "walrus", "xerus", "yak",
	"zebra", "bear", "cobra", "dragon", "elk",
	"fox", "giraffe", "heron", "ibex", "jackal",
	"kite", "lynx", "moose", "newt", "owl",
	"panda", "python", "rabbit", "swan", "turtle",
	"unicorn", "vulture", "whale", "wolf", "wren",
}

// Generate creates a random word pair in the format "adjective_noun"
// using cryptographically secure random number generation.
// Returns an empty string on error.
func Generate() string {
	adj, err := selectRandom(adjectives)
	if err != nil {
		return ""
	}

	noun, err := selectRandom(nouns)
	if err != nil {
		return ""
	}

	return fmt.Sprintf("%s_%s", adj, noun)
}

// selectRandom selects a random element from a slice using crypto/rand
func selectRandom(words []string) (string, error) {
	if len(words) == 0 {
		return "", fmt.Errorf("empty word list")
	}

	max := big.NewInt(int64(len(words)))
	n, err := rand.Int(rand.Reader, max)
	if err != nil {
		return "", fmt.Errorf("failed to generate random number: %w", err)
	}

	return words[n.Int64()], nil
}
