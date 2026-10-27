package statkey

type Key string

const (
	Points       Key = "points"
	Rebounds     Key = "rebounds"
	Assists      Key = "assists"
	Threes       Key = "threes"
	PassYards    Key = "passYards"
	RushYards    Key = "rushYards"
	RecYards     Key = "recYards"
	Receptions   Key = "receptions"
	PassTDs      Key = "passTDs"
	RushTDs      Key = "rushTDs"
	RecTDs       Key = "recTDs"
	Aces         Key = "aces"
	DoubleFaults Key = "doubleFaults"
	Sets         Key = "sets"
	Kills        Key = "kills"
	Maps         Key = "maps"
	Saves        Key = "saves"
	Passes       Key = "passes"
)

type Values map[Key]float64

var (
	Basketball = []Key{Points, Rebounds, Assists, Threes}
	Gridiron   = []Key{PassYards, RushYards, RecYards, Receptions, PassTDs, RushTDs, RecTDs}
)

func Zeroed(keys []Key) Values {
	values := make(Values, len(keys))
	for _, k := range keys {
		values[k] = 0
	}
	return values
}
