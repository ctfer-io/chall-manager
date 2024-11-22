package sdk

import (
	"encoding/binary"
	"math/rand"
	"slices"
)

type options struct {
	lowercase bool
	uppercase bool
	numeric   bool
	special   bool
}

// VariateOption is a functional option to guide the Variate function
// in its behaviors.
type VariateOption interface {
	apply(opts *options)
}

type lowercase bool

var _ VariateOption = (*lowercase)(nil)

func (o lowercase) apply(opts *options) {
	opts.lowercase = bool(o)
}

// WithLowercase enables to set the behavior regarding the variation
// from and to lowercase character.
// Defaults to true.
//
// If set to true, lowercase character is part of the variation possibilities
// in which an alternative will be choosen.
// If set to false, character won't be variated and won't be an alternative.
func WithLowercase(b bool) VariateOption {
	return lowercase(b)
}

type uppercase bool

var _ VariateOption = (*uppercase)(nil)

func (o uppercase) apply(opts *options) {
	opts.uppercase = bool(o)
}

// WithUppercase enables to set the behavior regarding the variation
// from and to uppercase character.
// Defaults to true.
//
// If set to true, uppercase character is part of the variation possibilities
// in which an alternative will be choosen.
// If set to false, character won't be variated and won't be an alternative.
func WithUppercase(b bool) VariateOption {
	return uppercase(b)
}

type numeric bool

var _ VariateOption = (*numeric)(nil)

func (o numeric) apply(opts *options) {
	opts.numeric = bool(o)
}

// WithNumeric enables to set the behavior regarding the variation
// from and to numeric character.
// Default to true.
//
// If set to true, numeric character is part of the variation possibilities
// in which an alternative will be choosen.
// If set to false, character won't be variated and won't be an alternative.
func WithNumeric(b bool) VariateOption {
	return numeric(b)
}

type special bool

var _ VariateOption = (*special)(nil)

func (o special) apply(opts *options) {
	opts.special = bool(o)
}

// WithSpecial enables to set the behavior regarding the variation
// from and to special characters.
// Defaults to false.
//
// If set to true, special characters is part of the variation possibilities
// in which an alternative will be choosen. This can lead to hardly-readable
// variations, possible inconsistencies on printing, or errors if injected in
// configurations. It is nonetheless a good way of improving outcomes randomness.
// Is set to false, character won't be variated and won't be an alternative.
func WithSpecial(b bool) VariateOption {
	return special(b)
}

// Variate consumes the identity as a pseudo-random seed and variate the base.
// Produced value fits printable ASCII extended charset, and is pseudo-random too
// but not cryptographically guaranteed.
func Variate(identity, base string, opts ...VariateOption) string {
	options := &options{
		lowercase: true,
		uppercase: true,
		numeric:   true,
		special:   false,
	}
	for _, opt := range opts {
		opt.apply(options)
	}

	return variate(identity, base, options)
}

func variate(identity, flag string, opts *options) string {
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

	// Variate them all
	vflag := make([]rune, 0, len(flag))
	for i := 0; i < len(flag); i++ {
		vflag = append(vflag, get(prng, rune(flag[i]), opts))
	}
	return string(vflag)
}

const (
	keyLowercase = "l"
	keyUppercase = "u"
	keyNumeric   = "n"
	keySpecial   = "s"
)

func get(prng *rand.Rand, r rune, options *options) rune {
	poss := possibilities(r, options)
	if len(poss) == 0 {
		return r
	}

	// Return a random one in all them (equiprobable)
	idx := prng.Int() % len(poss)
	return poss[idx]
}

func possibilities(r rune, opts *options) []rune {
	// Locate alternative group
	lid := -1
	for i := range dict {
		for _, vs := range dict[i] {
			if slices.Contains(vs, r) {
				lid = i
				break
			}
		}
		if lid != -1 {
			break
		}
	}
	if lid == -1 {
		return nil
	}

	// Build all possibilities
	poss := []rune{}
	if opts.lowercase {
		poss = append(poss, dict[lid][keyLowercase]...)
	}
	if opts.uppercase {
		poss = append(poss, dict[lid][keyUppercase]...)
	}
	if opts.numeric {
		poss = append(poss, dict[lid][keyNumeric]...)
	}
	if opts.special {
		poss = append(poss, dict[lid][keySpecial]...)
	}

	// If alternatives does not contain itself, it should not be variated
	if !slices.Contains(poss, r) {
		return nil
	}
	return poss
}

var (
	// dict is a collision-free set of character alternatives.
	// all characters fits in printable extended ascii table.
	dict = []map[string][]rune{
		{
			keyLowercase: []rune{'a'},
			keyUppercase: []rune{'A'},
			keyNumeric:   []rune{'4'},
			keySpecial:   []rune{'@', 'ª', 'À', 'Á', 'Â', 'Ã', 'Ä', 'Å', 'à', 'á', 'â', 'ã', 'ä', 'å'},
		}, {
			keyLowercase: []rune{'b'},
			keyUppercase: []rune{'B'},
			keyNumeric:   []rune{'8'},
			keySpecial:   []rune{'ß'},
		}, {
			keyLowercase: []rune{'c'},
			keyUppercase: []rune{'C'},
			keySpecial:   []rune{'(', '¢', '©', 'Ç', 'ç'},
		}, {
			keyLowercase: []rune{'d'},
			keyUppercase: []rune{'D'},
			keySpecial:   []rune{'Ð'},
		}, {
			keyLowercase: []rune{'e'},
			keyUppercase: []rune{'E'},
			keyNumeric:   []rune{'3'},
			keySpecial:   []rune{'€', '&', '£', 'È', 'É', 'Ê', 'Ë', 'è', 'é', 'ê', 'ë'},
		}, {
			keyLowercase: []rune{'f'},
			keyUppercase: []rune{'F'},
			keySpecial:   []rune{'ƒ'},
		}, {
			keyLowercase: []rune{'g'},
			keyUppercase: []rune{'G'},
		}, {
			keyLowercase: []rune{'h'},
			keyUppercase: []rune{'H'},
			keySpecial:   []rune{'#'},
		}, {
			keyLowercase: []rune{'i'},
			keyUppercase: []rune{'I'},
			keyNumeric:   []rune{'1'},
			keySpecial:   []rune{'!', 'Ì', 'Í', 'Î', 'Ï', 'ì', 'í', 'î', 'ï'},
		}, {
			keyLowercase: []rune{'j'},
			keyUppercase: []rune{'J'},
		}, {
			keyLowercase: []rune{'k'},
			keyUppercase: []rune{'K'},
		}, {
			keyLowercase: []rune{'l'},
			keyUppercase: []rune{'L'},
		}, {
			keyLowercase: []rune{'m'},
			keyUppercase: []rune{'M'},
		}, {
			keyLowercase: []rune{'n'},
			keyUppercase: []rune{'N'},
		}, {
			keyLowercase: []rune{'o'},
			keyUppercase: []rune{'O'},
			keyNumeric:   []rune{'0'},
			keySpecial:   []rune{'¤', '°', 'º', 'Ò', 'Ó', 'Ô', 'Õ', 'Ö', 'Ø', 'ø', 'ò', 'ó', 'ô', 'õ', 'ö', 'ð'},
		}, {
			keyLowercase: []rune{'p'},
			keyUppercase: []rune{'P'},
		}, {
			keyLowercase: []rune{'q'},
			keyUppercase: []rune{'Q'},
		}, {
			keyLowercase: []rune{'r'},
			keyUppercase: []rune{'R'},
			keySpecial:   []rune{'®'},
		}, {
			keyLowercase: []rune{'s'},
			keyUppercase: []rune{'S'},
			keyNumeric:   []rune{'5'},
			keySpecial:   []rune{'$', 'š', 'Š', '§'},
		}, {
			keyLowercase: []rune{'t'},
			keyUppercase: []rune{'T'},
			keyNumeric:   []rune{'7'},
			keySpecial:   []rune{'†'},
		}, {
			keyLowercase: []rune{'u'},
			keyUppercase: []rune{'U'},
			keySpecial:   []rune{'µ', 'Ù', 'Ú', 'Û', 'Ü', 'ù', 'ú', 'û', 'ü'},
		}, {
			keyLowercase: []rune{'v'},
			keyUppercase: []rune{'V'},
		}, {
			keyLowercase: []rune{'w'},
			keyUppercase: []rune{'W'},
		}, {
			keyLowercase: []rune{'x'},
			keyUppercase: []rune{'X'},
			keySpecial:   []rune{'×'},
		}, {
			keyLowercase: []rune{'y'},
			keyUppercase: []rune{'Y'},
			keySpecial:   []rune{'Ÿ', '¥', 'Ý', 'ý', 'ÿ'},
		}, {
			keyLowercase: []rune{'z'},
			keyUppercase: []rune{'Z'},
			keySpecial:   []rune{'ž', 'Ž'},
		}, {
			keySpecial: []rune{' ', '-', '_', '~'},
		},
	}
)
