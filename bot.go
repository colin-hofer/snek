package snek

const botUnreachable = 1 << 30

type botMove struct {
	input action
	dir   direction
}

type botCandidate struct {
	move    botMove
	start   int
	oldTail int
	newTail int
	grow    bool
}

type botEval struct {
	space            int
	foodDistance     int
	safeFoodDistance int
	tailDistance     int
	exits            int
}

var botMoves = [...]botMove{
	{input: moveUp, dir: dirUp},
	{input: moveRight, dir: dirRight},
	{input: moveDown, dir: dirDown},
	{input: moveLeft, dir: dirLeft},
}

func botAction(g *game) (action, bool) {
	if g == nil || g.over || g.width == 0 || g.height == 0 || g.length == 0 {
		return 0, false
	}

	loopIndex, avoidMask := g.botLoopAvoidMask()
	g.markBotFoods()
	g.markBotBody()

	head := g.headPoint()
	oldTail := g.index(g.tailPoint())

	var actions [len(botMoves)]action
	var scores [len(botMoves)]int
	count := 0
	legalMask := uint8(0)

	for _, move := range botMoves {
		candidate, ok := botCandidateForMove(g, head, oldTail, move)
		if !ok {
			continue
		}

		eval := g.botFlood(candidate)
		actions[count] = move.input
		scores[count] = botScore(g, candidate, eval)
		legalMask |= botActionBit(move.input)
		count++
	}

	if count == 0 {
		return 0, false
	}

	activeAvoidMask := uint8(0)
	if untried := legalMask &^ avoidMask; untried != 0 {
		activeAvoidMask = avoidMask
	}

	best := actions[0]
	bestScore := botLoopAdjustedScore(scores[0], actions[0], activeAvoidMask)
	for i := 1; i < count; i++ {
		score := botLoopAdjustedScore(scores[i], actions[i], activeAvoidMask)
		if score > bestScore {
			best = actions[i]
			bestScore = score
		}
	}

	g.rememberBotLoopAction(loopIndex, best)
	return best, true
}

func botCandidateForMove(g *game, head point, oldTail int, move botMove) (botCandidate, bool) {
	if opposite(move.dir, g.dir) || opposite(move.dir, g.next) {
		return botCandidate{}, false
	}

	start, ok := botStepFromPoint(g, head, move.dir)
	if !ok {
		return botCandidate{}, false
	}

	grow := g.botFood[start] == g.botFoodStamp
	if g.hasIndex(start) && (grow || start != oldTail) {
		return botCandidate{}, false
	}

	newTail := oldTail
	if !grow {
		if g.length > 1 {
			newTail = g.index(g.snake[g.ring(g.length-2)])
		} else {
			newTail = start
		}
	}

	return botCandidate{
		move:    move,
		start:   start,
		oldTail: oldTail,
		newTail: newTail,
		grow:    grow,
	}, true
}

func botScore(g *game, candidate botCandidate, eval botEval) int {
	nextLength := g.length
	if candidate.grow {
		nextLength++
	}

	score := eval.space * 1000
	if eval.tailDistance == botUnreachable {
		score -= 1_000_000
	} else {
		score += 1_000_000 - eval.tailDistance*25
	}

	if eval.exits == 0 {
		score -= 2_000_000
	} else if eval.exits == 1 {
		score -= 250_000
	} else {
		score += eval.exits * 50_000
	}

	if eval.space < nextLength {
		score -= (nextLength - eval.space + 1) * 150_000
	}

	if candidate.grow {
		if eval.exits > 0 && eval.space >= nextLength {
			score += 6_000_000
		} else {
			score -= 2_000_000
		}
	} else if eval.safeFoodDistance != botUnreachable {
		foodScore := 2_000_000 - eval.safeFoodDistance*5000
		if eval.tailDistance == botUnreachable || eval.space < nextLength {
			foodScore /= 8
		}
		score += foodScore
	} else if eval.foodDistance != botUnreachable {
		score += 80_000 - eval.foodDistance*500
	} else {
		score -= 200_000
	}
	if candidate.move.dir == g.dir {
		score += 20
	}
	if candidate.move.dir == g.next {
		score += 10
	}

	return score
}

func botLoopAdjustedScore(score int, input action, avoidMask uint8) int {
	if avoidMask&botActionBit(input) != 0 {
		score -= 10_000_000
	}
	return score
}

func (g *game) botFlood(candidate botCandidate) botEval {
	stamp := g.nextBotStamp()
	width := int(g.width)
	cells := int(g.width) * int(g.height)
	queue := g.botQueue[:1]
	queue[0] = candidate.start
	g.botSeen[candidate.start] = stamp

	eval := botEval{
		foodDistance:     botUnreachable,
		safeFoodDistance: botUnreachable,
		tailDistance:     botUnreachable,
		exits:            g.botExitCount(candidate),
	}

	for depth, read := 0, 0; read < len(queue); depth++ {
		end := len(queue)
		for ; read < end; read++ {
			index := queue[read]
			if index == candidate.newTail {
				eval.tailDistance = min(eval.tailDistance, depth)
			}
			if index != candidate.start && g.botFood[index] == g.botFoodStamp {
				eval.foodDistance = min(eval.foodDistance, depth)
			}

			if index >= width {
				next := index - width
				if g.botSeen[next] != stamp && !g.botBlocked(candidate, next, true) {
					g.botSeen[next] = stamp
					queue = append(queue, next)
				}
			}
			if down := index + width; down < cells {
				if g.botSeen[down] != stamp && !g.botBlocked(candidate, down, true) {
					g.botSeen[down] = stamp
					queue = append(queue, down)
				}
			}
			col := index % width
			if col != 0 {
				next := index - 1
				if g.botSeen[next] != stamp && !g.botBlocked(candidate, next, true) {
					g.botSeen[next] = stamp
					queue = append(queue, next)
				}
			}
			if col != width-1 {
				next := index + 1
				if g.botSeen[next] != stamp && !g.botBlocked(candidate, next, true) {
					g.botSeen[next] = stamp
					queue = append(queue, next)
				}
			}
		}
	}

	eval.space = len(queue)
	if candidate.grow {
		eval.safeFoodDistance = 0
	} else {
		eval.safeFoodDistance = g.botSafeFoodDistance(candidate)
	}
	return eval
}

func (g *game) botExitCount(candidate botCandidate) int {
	exits := 0
	for _, move := range botMoves {
		if opposite(move.dir, candidate.move.dir) {
			continue
		}
		next, ok := botStepFromIndex(g, candidate.start, move.dir)
		if ok && !g.botBlocked(candidate, next, true) {
			exits++
		}
	}
	return exits
}

func (g *game) botSafeFoodDistance(candidate botCandidate) int {
	stamp := g.nextBotStamp()
	width := int(g.width)
	cells := int(g.width) * int(g.height)
	queue := g.botQueue[:1]
	queue[0] = candidate.start
	g.botSeen[candidate.start] = stamp

	for depth, read := 0, 0; read < len(queue); depth++ {
		end := len(queue)
		nextDepth := depth + 1
		for ; read < end; read++ {
			index := queue[read]

			if index >= width {
				next := index - width
				if g.botFood[next] == g.botFoodStamp {
					if g.botFoodHasExitAt(candidate, next, index, nextDepth) {
						return nextDepth
					}
				} else if g.botSeen[next] != stamp && g.botFreeAt(candidate, next, nextDepth) {
					g.botSeen[next] = stamp
					queue = append(queue, next)
				}
			}
			if down := index + width; down < cells {
				if g.botFood[down] == g.botFoodStamp {
					if g.botFoodHasExitAt(candidate, down, index, nextDepth) {
						return nextDepth
					}
				} else if g.botSeen[down] != stamp && g.botFreeAt(candidate, down, nextDepth) {
					g.botSeen[down] = stamp
					queue = append(queue, down)
				}
			}
			col := index % width
			if col != 0 {
				next := index - 1
				if g.botFood[next] == g.botFoodStamp {
					if g.botFoodHasExitAt(candidate, next, index, nextDepth) {
						return nextDepth
					}
				} else if g.botSeen[next] != stamp && g.botFreeAt(candidate, next, nextDepth) {
					g.botSeen[next] = stamp
					queue = append(queue, next)
				}
			}
			if col != width-1 {
				next := index + 1
				if g.botFood[next] == g.botFoodStamp {
					if g.botFoodHasExitAt(candidate, next, index, nextDepth) {
						return nextDepth
					}
				} else if g.botSeen[next] != stamp && g.botFreeAt(candidate, next, nextDepth) {
					g.botSeen[next] = stamp
					queue = append(queue, next)
				}
			}
		}
	}

	return botUnreachable
}

func (g *game) botFoodHasExitAt(candidate botCandidate, food, from, depth int) bool {
	for _, move := range botMoves {
		next, ok := botStepFromIndex(g, food, move.dir)
		if ok && next != from && g.botFreeAt(candidate, next, depth) {
			return true
		}
	}
	return false
}

func (g *game) botFreeAt(candidate botCandidate, index, depth int) bool {
	if index == candidate.start {
		return depth >= g.length
	}
	if !candidate.grow && index == candidate.oldTail {
		return true
	}
	if g.botBodySeen[index] != g.botBodyStamp {
		return true
	}

	offset := g.botBodyOrder[index]
	if candidate.grow {
		return depth >= g.length-offset
	}
	if offset == g.length-1 {
		return true
	}
	return depth >= g.length-offset-1
}

func (g *game) botBlocked(candidate botCandidate, index int, allowTail bool) bool {
	if index == candidate.start {
		return false
	}
	if !candidate.grow && index == candidate.oldTail {
		return false
	}
	if allowTail && !candidate.grow && index == candidate.newTail {
		return false
	}
	return g.hasIndex(index)
}

func botStepFromPoint(g *game, p point, dir direction) (int, bool) {
	x := int(p.x) + int(dir.x)
	y := int(p.y) + int(dir.y)
	if x < 0 || x >= int(g.width) || y < 0 || y >= int(g.height) {
		return 0, false
	}
	return y*int(g.width) + x, true
}

func botStepFromIndex(g *game, index int, dir direction) (int, bool) {
	width := int(g.width)
	switch dir {
	case dirUp:
		if index < width {
			return 0, false
		}
		return index - width, true
	case dirDown:
		next := index + width
		if next >= int(g.width)*int(g.height) {
			return 0, false
		}
		return next, true
	case dirLeft:
		if index%width == 0 {
			return 0, false
		}
		return index - 1, true
	case dirRight:
		if index%width == width-1 {
			return 0, false
		}
		return index + 1, true
	default:
		return 0, false
	}
}

func (g *game) markBotFoods() {
	g.botFoodStamp++
	if g.botFoodStamp == 0 {
		clear(g.botFood)
		g.botFoodStamp = 1
	}

	for i := 0; i < g.foods; i++ {
		g.botFood[g.index(g.food[i])] = g.botFoodStamp
	}
}

func (g *game) markBotBody() {
	g.botBodyStamp++
	if g.botBodyStamp == 0 {
		clear(g.botBodySeen)
		g.botBodyStamp = 1
	}

	for offset := 0; offset < g.length; offset++ {
		index := g.index(g.snake[g.ring(offset)])
		g.botBodySeen[index] = g.botBodyStamp
		g.botBodyOrder[index] = offset
	}
}

func (g *game) botLoopAvoidMask() (int, uint8) {
	if g.score != g.botLoopScore {
		g.botLoopScore = g.score
		g.botLoopLen = 0
		g.botLoopNext = 0
	}

	hash := g.botStateHash()
	for i := 0; i < g.botLoopLen; i++ {
		if g.botLoopHash[i] == hash {
			return i, g.botLoopMask[i]
		}
	}

	if len(g.botLoopHash) == 0 {
		return -1, 0
	}
	if g.botLoopLen < len(g.botLoopHash) {
		i := g.botLoopLen
		g.botLoopLen++
		g.botLoopHash[i] = hash
		g.botLoopMask[i] = 0
		return i, 0
	}

	i := g.botLoopNext
	g.botLoopNext++
	if g.botLoopNext >= len(g.botLoopHash) {
		g.botLoopNext = 0
	}
	g.botLoopHash[i] = hash
	g.botLoopMask[i] = 0
	return i, 0
}

func (g *game) rememberBotLoopAction(index int, input action) {
	if index >= 0 {
		g.botLoopMask[index] |= botActionBit(input)
	}
}

func (g *game) botStateHash() uint64 {
	h := uint64(1469598103934665603)
	h = botHashMix(h, uint64(g.width))
	h = botHashMix(h, uint64(g.height))
	h = botHashMix(h, uint64(uint8(g.dir.x))<<8|uint64(uint8(g.dir.y)))
	h = botHashMix(h, uint64(uint8(g.next.x))<<8|uint64(uint8(g.next.y)))
	h = botHashMix(h, uint64(g.length))
	for i := 0; i < g.foods; i++ {
		h = botHashMix(h, uint64(g.index(g.food[i]))+1)
	}
	for i := 0; i < g.length; i++ {
		h = botHashMix(h, uint64(g.index(g.snake[g.ring(i)]))+1)
	}
	return h
}

func botHashMix(h, v uint64) uint64 {
	h ^= v + 0x9e3779b97f4a7c15 + (h << 6) + (h >> 2)
	return h
}

func botActionBit(input action) uint8 {
	switch input {
	case moveUp:
		return 1 << 0
	case moveDown:
		return 1 << 1
	case moveLeft:
		return 1 << 2
	case moveRight:
		return 1 << 3
	default:
		return 0
	}
}

func (g *game) nextBotStamp() uint32 {
	g.botStamp++
	if g.botStamp == 0 {
		clear(g.botSeen)
		g.botStamp = 1
	}
	return g.botStamp
}
