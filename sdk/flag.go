package sdk

import (
	"encoding/binary"
	"math/rand"
	"slices"
)

// vars is the list of all the characters that contains vars.
// if not contained, don't variate the character.
// please don't add vars that are not part of the printable extended ascii table.
var vars = [][]rune{
	{'a', 'A', '4', '@', 'ª', 'À', 'Á', 'Â', 'Ã', 'Ä', 'Å', 'à', 'á', 'â', 'ã', 'ä', 'å'},
	{'b', 'B', '8', 'ß'},
	{'c', 'C', '(', '¢', '©', 'Ç', 'ç'},
	{'d', 'D', 'Ð'},
	{'e', 'E', '€', '&', '£', 'È', 'É', 'Ê', 'Ë', 'è', 'é', 'ê', 'ë', '3'},
	{'f', 'F', 'ƒ'},
	{'g', 'G'},
	{'h', 'H', '#'},
	{'i', 'I', '1', '!', 'Ì', 'Í', 'Î', 'Ï', 'ì', 'í', 'î', 'ï'},
	{'j', 'J'},
	{'k', 'K'},
	{'l', 'L'},
	{'m', 'M'},
	{'n', 'N', 'Ñ', 'ñ'},
	{'o', 'O', '0', '¤', '°', 'º', 'Ò', 'Ó', 'Ô', 'Õ', 'Ö', 'Ø', 'ø', 'ò', 'ó', 'ô', 'õ', 'ö', 'ð'},
	{'p', 'P'},
	{'q', 'Q'},
	{'r', 'R', '®'},
	{'s', 'S', '5', '$', 'š', 'Š', '§'},
	{'t', 'T', '7', '†'},
	{'u', 'U', 'µ', 'Ù', 'Ú', 'Û', 'Ü', 'ù', 'ú', 'û', 'ü'},
	{'v', 'V'},
	{'w', 'W'},
	{'x', 'X', '×'},
	{'y', 'Y', 'Ÿ', '¥', 'Ý', 'ý', 'ÿ'},
	{'z', 'Z', 'ž', 'Ž'},
	{' ', '-', '_', '~'},
}

// VariateFlag builds a PRNG with the given the 8 first characters of the seed (zeros
// are appended if necessary) then travels through all the content and variate each
// character that could be, then returns it. In the end, the result should still be
// understandable by the player thus not altering his soul.
// For instance the flag "super-flag" could become "$uPer-Fl@g" or "sUP&r-fLag".
//
// This process is explainable and reproducible thus enables CTF events to be
// completly reproducibles as required by many organizers.
// Mutated characters have decimal representations over 32 and under 127 (see ASCII
// table for more info), variations remains in this interval.
func VariateFlag(identity string, flag string) string {
	// Append zeros if necessary (should not happen if proper identity provided).
	p := 8 - len(identity)
	if p > 0 {
		for i := 0; i < p; i++ {
			identity += "0"
		}
	}

	// Create PRNG
	sb := []byte(identity)
	s := int64(binary.BigEndian.Uint64(sb))
	prng := rand.New(rand.NewSource(s))

	// Compute variations
	buf := make([]rune, 0, len(flag))
	for _, r := range flag {
		vars := getVars(r)
		// If no variants available, don't do it
		if vars == nil {
			buf = append(buf, r)
			continue
		}
		idx := prng.Int() % len(vars)
		buf = append(buf, vars[idx])
	}
	return string(buf)
}

func getVars(r rune) []rune {
	for _, vars := range vars {
		if slices.Contains(vars, r) {
			return vars
		}
	}
	return nil
}
