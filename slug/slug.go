// Package slug generates URL-safe default slugs.
package slug

import (
	"math/rand/v2"
	"slices"
	"strconv"
	"strings"
)

// MaxAttempts bounds generated slugs a caller tries.
const MaxAttempts = 5

const (
	suffixMin = 1000
	suffixMax = 9999
)

//nolint:gochecknoglobals // immutable word list, not runtime state.
var adjectives = [314]string{
	"adventurous", "aerial", "affable", "ageless", "agile", "agreeable", "airy", "alert", "alluring", "alpine",
	"amber", "ambitious", "amiable", "amicable", "ample", "ancient", "angelic", "angular", "animated", "aqua",
	"arctic", "ardent", "aromatic", "artful", "artistic", "astral", "athletic", "atomic", "attentive", "august",
	"austere", "autumn", "azure", "balmy", "bearded", "beige", "benevolent", "biting", "bitter", "blameless",
	"blazing", "blissful", "blithe", "blooming", "blowy", "blue", "blushing", "bohemian", "bold", "bonny",
	"bouncy", "boundless", "brainy", "brash", "brave", "brawny", "breezy", "bright", "brilliant", "brimming",
	"brisk", "bristly", "bronze", "bubbly", "burly", "bustling", "buttery", "buzzing", "cagey", "calm",
	"candid", "canny", "carefree", "cavernous", "cerulean", "chalky", "charming", "cheerful", "cheery", "chic",
	"chilly", "chirpy", "chummy", "chunky", "civic", "classy", "clean", "clement", "clever", "cloudless",
	"cloudy", "clumsy", "coastal", "cobalt", "cogent", "colorful", "colossal", "comfy", "comical", "composed",
	"constant", "content", "coral", "cordial", "cosmic", "courtly", "coy", "cozy", "craggy", "crimson",
	"crinkly", "crisp", "croaky", "crystal", "cuddly", "cultured", "curious", "curly", "curvy", "cushy",
	"cute", "dainty", "dandy", "dapper", "daring", "dauntless", "debonair", "decent", "deciduous", "deep",
	"delicate", "devoted", "dewy", "diligent", "dimpled", "diplomatic", "divine", "dizzy", "docile", "doughty",
	"downy", "dreamy", "drowsy", "dusky", "dusty", "dynamic", "eager", "early", "earnest", "earthy",
	"easy", "easygoing", "ebony", "eclectic", "eerie", "elated", "electric", "elegant", "elusive", "emerald",
	"eminent", "enchanted", "enigmatic", "epic", "eternal", "ethereal", "evergreen", "exotic", "exquisite", "fabled",
	"faint", "fair", "faithful", "fallow", "fanciful", "fancy", "fast", "fearless", "feathery", "feisty",
	"festive", "fiery", "fine", "fluffy", "foggy", "fond", "free", "fresh", "frosty", "frozen",
	"fuzzy", "gentle", "giant", "glossy", "golden", "grand", "grassy", "great", "green", "happy",
	"hardy", "hazy", "hearty", "hidden", "hollow", "honest", "humble", "icy", "indigo", "ivory",
	"jade", "jolly", "jovial", "keen", "kind", "lavish", "leafy", "light", "lime", "lively",
	"lofty", "lucky", "lunar", "lush", "mellow", "merry", "mighty", "mild", "misty", "modern",
	"mossy", "narrow", "neat", "noble", "noisy", "nordic", "olive", "orange", "ornate", "pastel",
	"peachy", "peppy", "plucky", "plush", "polar", "polite", "pretty", "proud", "puffy", "purple",
	"quaint", "quick", "quiet", "rapid", "rare", "ripe", "rosy", "rough", "royal", "rugged",
	"rustic", "sandy", "scarlet", "serene", "sharp", "shiny", "silent", "silky", "silver", "simple",
	"sleepy", "slick", "smart", "smooth", "snowy", "solar", "solid", "sonic", "spacious", "spicy",
	"spiky", "spruce", "spry", "square", "steady", "steel", "stone", "stormy", "strong", "sunny",
	"swift", "tall", "tangy", "teal", "tender", "thorny", "tidy", "tiny", "toasty", "tropical",
	"velvet", "vivid", "warm", "wavy", "white", "wide", "wild", "windy", "wintry", "wise",
	"wispy", "witty", "young", "zesty",
}

//nolint:gochecknoglobals // immutable word list, not runtime state.
var nouns = [313]string{
	"abbey", "acacia", "acorn", "acre", "aeon", "aisle", "alcove", "alder", "algae", "alley",
	"alloy", "almond", "aloe", "alpaca", "altar", "amber", "amulet", "anchor", "anemone", "angel",
	"angle", "anise", "antler", "anvil", "apex", "apiary", "apple", "apricot", "apron", "aqueduct",
	"arbor", "arcade", "arch", "arena", "arrow", "aspen", "asteroid", "atlas", "aurora", "aviary",
	"axis", "badger", "badlands", "bagel", "bamboo", "banner", "barley", "barn", "barrier", "basin",
	"bastion", "beach", "beacon", "beam", "belfry", "bell", "bench", "berry", "billow", "birch",
	"biscuit", "bison", "blaze", "blizzard", "bloom", "blossom", "bluff", "bonsai", "border", "bough",
	"boulder", "bouquet", "bramble", "branch", "brass", "breeze", "brick", "bridge", "brink", "brook",
	"bubble", "bunting", "burrow", "bustle", "butte", "buzzard", "byway", "cabin", "cabinet", "cactus",
	"cairn", "caldera", "cameo", "canopy", "canton", "canvas", "canyon", "cape", "caper", "caravel",
	"cardinal", "carnival", "cascade", "cashew", "castle", "cave", "cavern", "cedar", "chalet", "charm",
	"chestnut", "chime", "chimney", "chive", "circuit", "citadel", "citrus", "clarion", "cleft", "cliff",
	"climate", "cloud", "clove", "clover", "coast", "cobbler", "cocoon", "comet", "compass", "conduit",
	"contour", "coppice", "coral", "cord", "cornet", "corral", "cosmos", "cottage", "cotton", "courtyard",
	"cove", "crag", "crane", "crater", "crayon", "creek", "crescent", "crest", "crevice", "crocus",
	"crown", "crumb", "cuckoo", "culvert", "cupola", "current", "cypress", "dale", "dapple", "dawn",
	"dell", "delta", "den", "desert", "dike", "dinghy", "dock", "dolphin", "dome", "drift",
	"drumlin", "dryad", "dune", "dungeon", "dusk", "eagle", "earth", "easel", "eddy", "ember",
	"falcon", "feather", "fern", "field", "finch", "fjord", "flame", "flint", "flower", "fog",
	"forest", "fossil", "fox", "frost", "garden", "gate", "geyser", "glacier", "glade", "glen",
	"granite", "grape", "grass", "grove", "gully", "harbor", "haven", "hawk", "heather", "hedge",
	"hill", "hollow", "horizon", "inlet", "island", "isle", "ivy", "jade", "jungle", "juniper",
	"kelp", "lagoon", "lake", "lantern", "lark", "laurel", "leaf", "ledge", "lily", "linden",
	"lotus", "maple", "marble", "marsh", "meadow", "mesa", "meteor", "mill", "mint", "mist",
	"moon", "moss", "mountain", "myrtle", "nest", "north", "oak", "oasis", "ocean", "orbit",
	"orchid", "otter", "palm", "peak", "pebble", "pine", "plateau", "pond", "poplar", "prairie",
	"prism", "quartz", "quill", "raven", "reef", "ridge", "river", "robin", "rock", "root",
	"rose", "sage", "sapling", "sea", "shell", "shore", "shrub", "sky", "slope", "snow",
	"sparrow", "spring", "sprout", "spruce", "star", "steam", "stone", "storm", "stream", "summit",
	"sun", "swan", "thicket", "thistle", "thunder", "tide", "timber", "trail", "tree", "tulip",
	"tundra", "valley", "vine", "violet", "volcano", "walnut", "wave", "willow", "wind", "wolf",
	"wood", "wren", "zenith",
}

// Generate returns a slug like "wispy-frost-4821".
func Generate() string {
	var b strings.Builder
	b.WriteString(adjectives[rand.IntN(len(adjectives))])
	b.WriteByte('-')
	b.WriteString(nouns[rand.IntN(len(nouns))])
	b.WriteByte('-')
	b.WriteString(strconv.Itoa(suffixMin + rand.IntN(suffixMax-suffixMin+1)))
	return b.String()
}

// Combinations reports how many slugs Generate can produce.
func Combinations() int {
	return len(adjectives) * len(nouns) * (suffixMax - suffixMin + 1)
}

// IsValid reports whether s is a generated "<adjective>-<noun>-<nnnn>" slug.
func IsValid(s string) bool {
	parts := strings.Split(s, "-")
	if len(parts) != 3 {
		return false
	}
	if !slices.Contains(adjectives[:], parts[0]) || !slices.Contains(nouns[:], parts[1]) {
		return false
	}
	n, err := strconv.Atoi(parts[2])
	return err == nil && n >= suffixMin && n <= suffixMax
}

// Retry calls create until a generated slug is not taken.
func Retry[T any](attempts int, taken func(error) bool, create func(candidate string) (T, error)) (T, error) {
	var zero T
	var last error
	for range max(attempts, 1) {
		v, err := create(Generate())
		if err == nil {
			return v, nil
		}
		if !taken(err) {
			return zero, err
		}
		last = err
	}
	return zero, last
}
