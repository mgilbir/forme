package brotli

// compressed reads the prefix codes a meta-block declares and then spends them
// on commands until the stated number of bytes has been produced.
func (d *decoder) compressed(remaining int) error {
	r := d.r
	var b blocks
	for i := 0; i < 3; i++ {
		b.types[i] = readVarLenUint8(r) + 1
		b.recent[i] = [2]int{1, 0}
		b.length[i] = blockLengthCap
		if b.types[i] < 2 {
			// One type, so nothing ever switches and no code is spent saying
			// so. The count above is larger than any meta-block, so the switch
			// below is never reached.
			continue
		}
		// The type alphabet is the types plus the two relative codes.
		var err error
		if b.typeCode[i], err = readCode(r, b.types[i]+2, b.types[i]+2); err != nil {
			return err
		}
		if b.lenCode[i], err = readCode(r, numBlockLenSymbols, numBlockLenSymbols); err != nil {
			return err
		}
		if b.length[i], err = readBlockLength(r, b.lenCode[i]); err != nil {
			return err
		}
	}

	// How distances are coded, §4: how many of the smallest are spelled out
	// directly, and how many low bits of the rest are sent unencoded.
	v := r.take(6)
	npostfix := uint(v & 3)
	ndirect := int(v>>2) << npostfix

	// The context model each literal block type uses, §7.1. There are four, and
	// which one suits depends on what the block holds — UTF-8 text and a table
	// of numbers want different ones.
	modes := make([]byte, b.types[0])
	for i := range modes {
		modes[i] = byte(r.take(2))
	}

	litMap, litTreeCount, err := readContextMap(r, b.types[0]<<6)
	if err != nil {
		return err
	}
	distMap, distTreeCount, err := readContextMap(r, b.types[2]<<2)
	if err != nil {
		return err
	}

	distSize := numDistanceShort + ndirect + (maxDistanceBits << (npostfix + 1))
	litTrees, err := readTrees(r, litTreeCount, numLiteralSymbols)
	if err != nil {
		return err
	}
	cmdTrees, err := readTrees(r, b.types[1], numCommandSymbols)
	if err != nil {
		return err
	}
	distTrees, err := readTrees(r, distTreeCount, distSize)
	if err != nil {
		return err
	}
	distBits, distBase := distanceTable(distSize, ndirect, npostfix)

	// The two bytes before the one being decoded, which choose its code. They
	// are carried in variables rather than read back out of the output because
	// this is the innermost loop in the decoder — but they start from what the
	// output already holds, because a meta-block boundary is not a break in the
	// text and the first literal after one still has two bytes before it.
	p1, p2 := d.previousTwo()

	for remaining > 0 {
		// Past the end of the input, the reader hands out zeroes rather than
		// failing, and this is what stops that from going on for ever. It is
		// not merely a shortcut: a command may legitimately produce no output —
		// a dictionary word that a transform cuts away to nothing — and one
		// read from invented zeroes produces the same nothing every time. With
		// nothing else to stop it, such a stream spins until the block counter
		// wraps, which is sixteen million commands of doing nothing.
		if r.overrun() {
			return errTruncated
		}
		if b.length[1] == 0 {
			if err := b.switchBlock(r, 1); err != nil {
				return err
			}
		}
		b.length[1]--
		sym, err := cmdTrees[b.current[1]].next(r)
		if err != nil {
			return err
		}
		cmd := commandLut[sym]
		insert := cmd.insertOffset + int(r.take(cmd.insertBits))
		copyLen := cmd.copyOffset + int(r.take(cmd.copyBits))
		// Which distance code the copy will use is chosen by how long it is:
		// a short copy is usually near, and giving them separate codes is worth
		// more than it costs.
		distTree := distMap[b.current[2]<<2+cmd.context]

		if insert > 0 {
			at, ok := d.extend(insert)
			if !ok {
				return errTooLarge
			}
			for k := 0; k < insert; k++ {
				if b.length[0] == 0 {
					if err := b.switchBlock(r, 0); err != nil {
						return err
					}
				}
				b.length[0]--
				lut := int(modes[b.current[0]]) << 9
				ctx := contextLookup[lut+int(p1)] | contextLookup[lut+256+int(p2)]
				lit, err := litTrees[litMap[b.current[0]<<6+int(ctx)]].next(r)
				if err != nil {
					return err
				}
				p2, p1 = p1, byte(lit)
				d.out[at+k] = p1
			}
			remaining -= insert
			if remaining <= 0 {
				// The command's copy is not read: the meta-block said how many
				// bytes it holds and they have all been produced.
				break
			}
		}

		// The distance. A command whose code said "the last distance" carries
		// no distance symbol at all, which is the single most common case and
		// the reason it is worth a special code.
		var distance, roll int
		if cmd.repeatDistance {
			roll = 1
			d.distIdx--
			distance = d.distRB[d.distIdx&3]
		} else {
			if b.length[2] == 0 {
				if err := b.switchBlock(r, 2); err != nil {
					return err
				}
				distTree = distMap[b.current[2]<<2+cmd.context]
			}
			b.length[2]--
			code, err := distTrees[distTree].next(r)
			if err != nil {
				return err
			}
			if code < numDistanceShort {
				distance, roll = d.recentDistance(code)
			} else {
				distance = distBase[code] + int(r.take(distBits[code]))<<npostfix
			}
		}

		// How far back the output actually reaches. Anything beyond it is not a
		// mistake but a reference to the static dictionary.
		reach := d.maxBack
		if len(d.out) < reach {
			reach = len(d.out)
		}
		if distance > reach {
			n, err := d.dictionaryWord(distance, reach, copyLen, roll)
			if err != nil {
				return err
			}
			remaining -= n
		} else {
			if distance <= 0 {
				return errDistance
			}
			d.distRB[d.distIdx&3] = distance
			d.distIdx++
			at, ok := d.extend(copyLen)
			if !ok {
				return errTooLarge
			}
			// Byte at a time, because the regions may overlap: a distance of
			// one and a length of ten is how a run of one byte is written, and
			// a block copy would get it wrong.
			from := at - distance
			for k := 0; k < copyLen; k++ {
				d.out[at+k] = d.out[from+k]
			}
			remaining -= copyLen
		}
		p1, p2 = d.previousTwo()
	}
	return nil
}

// previousTwo is the last two bytes of the output, which are the context a
// literal's prefix code is chosen by. Before there are two, the missing ones
// count as zero, which is what a decoder starting from an empty window sees.
func (d *decoder) previousTwo() (p1, p2 byte) {
	if n := len(d.out); n >= 1 {
		p1 = d.out[n-1]
		if n >= 2 {
			p2 = d.out[n-2]
		}
	}
	return p1, p2
}

// dictionaryWord appends one static-dictionary reference and returns how many
// bytes it added.
func (d *decoder) dictionaryWord(distance, reach, length, roll int) (int, error) {
	if distance > maxAllowedDistance {
		return 0, errDistance
	}
	// A distance past the end of the output is not a distance at all: it names
	// a word in the fixed dictionary and one of the transforms in tables.go.
	// The remembered distances are left alone — the reference is not to
	// anything in this stream, so repeating it would mean nothing — which is
	// what undoes the step the "same distance again" code took above.
	d.distIdx += roll

	before := len(d.out)
	out, err := word(d.out, length, distance-reach-1)
	if err == errEmptyWord {
		// A transform that leaves nothing. RFC 7932 tolerates it far out in the
		// distance range, where no encoder would emit one deliberately, and
		// refuses it where one plainly meant something.
		if distance <= 120 {
			return 0, err
		}
		err = nil
	}
	if err != nil {
		return 0, err
	}
	if len(out) > d.limit {
		return 0, errTooLarge
	}
	d.out = out
	return len(d.out) - before, nil
}

// recentDistance resolves one of the sixteen codes that name a recent distance
// rather than an absolute one, RFC 7932 §4.
//
// Four of them are the four remembered distances; the other twelve are one of
// the two most recent give or take one, two or three, which covers a stride —
// a table of fixed-width records refers back by the record size, and the size
// changes by a byte when a field does.
//
// The second result is one for the code that names the most recent distance,
// and is how many steps the ring buffer was wound back: that code does not
// record a new distance, so the write the copy is about to make must land on
// the slot it came from.
func (d *decoder) recentDistance(code int) (int, int) {
	if code <= 3 {
		roll := 1 >> uint(code)
		distance := d.distRB[(d.distIdx-(code-3))&3]
		d.distIdx -= roll
		return distance, roll
	}
	step, base := 3, code-4
	if code >= 10 {
		step, base = 2, code-10
	}
	// Six four-bit values packed into a word: -1, +1, -2, +2, -3, +3.
	delta := int((0x605142>>uint(4*base))&0xF) - 3
	distance := d.distRB[(d.distIdx+step)&3] + delta
	if distance <= 0 {
		// Past the start of the stream. Naming an impossible distance rather
		// than failing here keeps the check in one place, below.
		distance = maxAllowedDistance + 1
	}
	return distance, 0
}

// readTrees reads n prefix codes over the same alphabet.
func readTrees(r *reader, n, size int) ([]*huffman, error) {
	trees := make([]*huffman, n)
	for i := range trees {
		t, err := readCode(r, size, size)
		if err != nil {
			return nil, err
		}
		trees[i] = t
	}
	return trees, nil
}

// distanceTable works out what each distance symbol means, which depends on the
// two parameters the meta-block chose: how many distances are spelled out
// directly, and how many low bits of the rest ride along unencoded.
func distanceTable(size, ndirect int, npostfix uint) ([]uint, []int) {
	bits := make([]uint, size)
	base := make([]int, size)
	at := numDistanceShort
	for j := 0; j < ndirect && at < size; j++ {
		base[at] = j + 1
		at++
	}
	// Beyond the direct codes the distances double in range every other group,
	// which is what gives them a roughly constant cost in bits per distance.
	group := 1 << npostfix
	n, half := uint(1), 0
	for at < size {
		start := ndirect + ((((2 + half) << n) - 4) << npostfix) + 1
		for j := 0; j < group && at < size; j++ {
			bits[at] = n
			base[at] = start + j
			at++
		}
		n += uint(half)
		half ^= 1
	}
	return bits, base
}
