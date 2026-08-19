package brotli

// The insert-and-copy code, RFC 7932 §5.
//
// One symbol carries both halves of a command: how many literals to insert and
// how many bytes to copy. Coding them together rather than apart is worth real
// space, because they are not independent — a long copy usually follows few
// literals — and 704 symbols is small enough to describe cheaply.
//
// The 704 are laid out in eleven blocks of 64, and which block a symbol is in
// says which quarter of the insert range and which quarter of the copy range it
// falls in. Two of the eleven also say the copy uses the last distance again
// without spending a distance symbol on it, which is the commonest case in the
// whole format.

type command struct {
	insertBits   uint
	copyBits     uint
	insertOffset int
	copyOffset   int

	// context selects among four distance codes by how long the copy is: a
	// two-byte copy is almost always nearby and a long one often is not.
	context int

	// repeatDistance is set for the blocks whose commands carry no distance
	// symbol and mean "the same distance as last time".
	repeatDistance bool
}

var commandLut = buildCommandLut()

func buildCommandLut() *[numCommandSymbols]command {
	// How many extra bits each of the 24 insert codes and 24 copy codes has.
	// The ranges they cover follow: each starts where the last one ended.
	insertExtra := [24]uint{
		0, 0, 0, 0, 0, 0, 1, 1, 2, 2, 3, 3,
		4, 4, 5, 5, 6, 7, 8, 9, 10, 12, 14, 24,
	}
	copyExtra := [24]uint{
		0, 0, 0, 0, 0, 0, 0, 0, 1, 1, 2, 2,
		3, 3, 4, 4, 5, 5, 6, 7, 8, 9, 10, 24,
	}
	// Inserting nothing is a command; copying nothing is not, so copies start
	// at two.
	var insertOffset, copyOffset [24]int
	insertOffset[0], copyOffset[0] = 0, 2
	for i := 0; i < 23; i++ {
		insertOffset[i+1] = insertOffset[i] + 1<<insertExtra[i]
		copyOffset[i+1] = copyOffset[i] + 1<<copyExtra[i]
	}

	// Which quarter of each range each block of 64 covers, packed: the low bits
	// pick the copy quarter and the high bits the insert quarter. The first two
	// blocks are the ones whose distance is implied.
	cell := [11]int{0, 1, 0, 1, 8, 9, 2, 16, 10, 17, 18}

	lut := new([numCommandSymbols]command)
	for sym := range lut {
		block := sym >> 6
		pos := cell[block]
		copyCode := ((pos << 3) & 0x18) + (sym & 7)
		insertCode := (pos & 0x18) + ((sym >> 3) & 7)
		at := copyOffset[copyCode]
		ctx := 3
		if at <= 4 {
			ctx = at - 2
		}
		lut[sym] = command{
			insertBits:     insertExtra[insertCode],
			copyBits:       copyExtra[copyCode],
			insertOffset:   insertOffset[insertCode],
			copyOffset:     at,
			context:        ctx,
			repeatDistance: block < 2,
		}
	}
	return lut
}
