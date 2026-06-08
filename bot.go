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
	cycleAdvance     int
	cycleClearance   int
	cycleSafe        bool
	cycleNext        bool
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
	g.ensureBotState()

	loopIndex, avoidMask := g.botLoopAvoidMask()
	g.markBotBody()

	head := g.headPoint()
	oldTail := g.tailIndex()
	if move, ok := g.botDirectFoodAction(head, oldTail); ok {
		g.rememberBotLoopAction(loopIndex, move)
		return move, true
	}

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
		score := botScore(g, candidate, eval)
		actions[count] = move.input
		scores[count] = score
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

	grow := g.hasFoodIndex(start)
	if g.hasIndex(start) && (grow || start != oldTail) {
		return botCandidate{}, false
	}

	newTail := oldTail
	if !grow {
		if g.length > 1 {
			newTail = int(g.snake[g.ring(g.length-2)])
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
	if eval.cycleSafe {
		score += min(eval.cycleClearance, nextLength*3) * 12_000
		if eval.cycleNext {
			score += 120_000
		}
	} else {
		score -= 1_500_000
	}
	if eval.cycleAdvance > nextLength {
		score -= (eval.cycleAdvance - nextLength) * 2_000
	}

	if candidate.grow {
		if eval.exits > 0 && eval.space >= nextLength {
			score += 6_000_000
		} else {
			score -= 2_000_000
		}
	} else if eval.safeFoodDistance != botUnreachable {
		foodScore := 2_000_000 - eval.safeFoodDistance*5000
		if eval.tailDistance == botUnreachable || eval.space < nextLength || !eval.cycleSafe {
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

func (g *game) botDirectFoodAction(head point, oldTail int) (action, bool) {
	if g.foods != 1 {
		return 0, false
	}

	food := g.pointAt(int(g.food[0]))
	if food == head {
		return 0, false
	}

	var moves [len(botMoves)]botMove
	count := 0
	add := func(move botMove) {
		if !botMoveToward(head, food, move.dir) {
			return
		}
		for i := 0; i < count; i++ {
			if moves[i].input == move.input {
				return
			}
		}
		moves[count] = move
		count++
	}

	for _, move := range botMoves {
		if move.dir == g.dir {
			add(move)
			break
		}
	}

	dx := int(food.x) - int(head.x)
	dy := int(food.y) - int(head.y)
	if botAbs(dx) >= botAbs(dy) {
		add(botHorizontalMove(dx))
		add(botVerticalMove(dy))
	} else {
		add(botVerticalMove(dy))
		add(botHorizontalMove(dx))
	}

	for i := 0; i < count; i++ {
		candidate, ok := botCandidateForMove(g, head, oldTail, moves[i])
		if !ok {
			continue
		}
		if g.botDirectMoveSafe(candidate, g.botFlood(candidate)) {
			return moves[i].input, true
		}
	}
	return 0, false
}

func (g *game) botDirectMoveSafe(candidate botCandidate, eval botEval) bool {
	nextLength := g.length
	if candidate.grow {
		nextLength++
	}
	if eval.exits == 0 || eval.space < nextLength {
		return false
	}
	if g.length <= initialLength(g.width)+1 {
		return true
	}
	if g.width%2 != 0 && g.height%2 != 0 {
		return false
	}
	return eval.cycleSafe && eval.tailDistance != botUnreachable
}

func botMoveToward(head, food point, dir direction) bool {
	switch dir {
	case dirUp:
		return food.y < head.y
	case dirDown:
		return food.y > head.y
	case dirLeft:
		return food.x < head.x
	case dirRight:
		return food.x > head.x
	default:
		return false
	}
}

func botHorizontalMove(dx int) botMove {
	if dx < 0 {
		return botMove{input: moveLeft, dir: dirLeft}
	}
	return botMove{input: moveRight, dir: dirRight}
}

func botVerticalMove(dy int) botMove {
	if dy < 0 {
		return botMove{input: moveUp, dir: dirUp}
	}
	return botMove{input: moveDown, dir: dirDown}
}

func botAbs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func botLoopAdjustedScore(score int, input action, avoidMask uint8) int {
	if avoidMask&botActionBit(input) != 0 {
		score -= 10_000_000
	}
	return score
}

func (g *game) botCycleEval(candidate botCandidate) (advance, clearance int, safe, next bool) {
	cells := int(g.width) * int(g.height)
	if cells <= 1 {
		return 0, 0, true, true
	}
	if g.width%2 != 0 && g.height%2 != 0 {
		return 1, cells - g.length, true, false
	}

	headOrder := g.botCycleOrder(g.headIndex())
	nextOrder := g.botCycleOrder(candidate.start)
	tailOrder := g.botCycleOrder(candidate.oldTail)
	advance = botCycleDistance(headOrder, nextOrder, cells)
	clearance = botCycleDistance(nextOrder, tailOrder, cells)
	tailAdvance := botCycleDistance(headOrder, tailOrder, cells)
	next = advance == 1
	if candidate.grow {
		safe = advance > 0 && advance < tailAdvance
	} else {
		safe = advance > 0 && advance <= tailAdvance
	}
	return advance, clearance, safe, next
}

func (g *game) botCycleOrder(index int) int {
	width := int(g.width)
	height := int(g.height)
	x := index % width
	y := index / width
	if height%2 == 0 {
		return botCycleOrderEvenHeight(width, height, x, y)
	}
	return botCycleOrderEvenHeight(height, width, y, x)
}

func botCycleOrderEvenHeight(width, height, x, y int) int {
	if y == 0 {
		return x
	}
	if x == 0 {
		return width*height - y
	}

	row := y - 1
	cols := width - 1
	if row%2 == 0 {
		return width + row*cols + width - 1 - x
	}
	return width + row*cols + x - 1
}

func botCycleDistance(from, to, cells int) int {
	distance := to - from
	if distance <= 0 {
		distance += cells
	}
	return distance
}

func (g *game) botFlood(candidate botCandidate) botEval {
	stamp := g.nextBotStamp()
	width := int(g.width)
	cells := int(g.width) * int(g.height)
	queue := g.botQueue[:1]
	queue[0] = cellIndex(candidate.start)
	g.botSeen[candidate.start] = stamp

	eval := botEval{
		foodDistance:     botUnreachable,
		safeFoodDistance: botUnreachable,
		tailDistance:     botUnreachable,
		exits:            g.botExitCount(candidate),
	}
	eval.cycleAdvance, eval.cycleClearance, eval.cycleSafe, eval.cycleNext = g.botCycleEval(candidate)

	for depth, read := 0, 0; read < len(queue); depth++ {
		end := len(queue)
		for ; read < end; read++ {
			index := int(queue[read])
			if index == candidate.newTail {
				eval.tailDistance = min(eval.tailDistance, depth)
			}
			if index != candidate.start && g.hasFoodIndex(index) {
				eval.foodDistance = min(eval.foodDistance, depth)
			}

			if index >= width {
				next := index - width
				if g.botSeen[next] != stamp && !g.botBlocked(candidate, next, true) {
					g.botSeen[next] = stamp
					queue = append(queue, cellIndex(next))
				}
			}
			if down := index + width; down < cells {
				if g.botSeen[down] != stamp && !g.botBlocked(candidate, down, true) {
					g.botSeen[down] = stamp
					queue = append(queue, cellIndex(down))
				}
			}
			col := index % width
			if col != 0 {
				next := index - 1
				if g.botSeen[next] != stamp && !g.botBlocked(candidate, next, true) {
					g.botSeen[next] = stamp
					queue = append(queue, cellIndex(next))
				}
			}
			if col != width-1 {
				next := index + 1
				if g.botSeen[next] != stamp && !g.botBlocked(candidate, next, true) {
					g.botSeen[next] = stamp
					queue = append(queue, cellIndex(next))
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
	queue[0] = cellIndex(candidate.start)
	g.botSeen[candidate.start] = stamp

	for depth, read := 0, 0; read < len(queue); depth++ {
		end := len(queue)
		nextDepth := depth + 1
		for ; read < end; read++ {
			index := int(queue[read])

			if index >= width {
				next := index - width
				if g.hasFoodIndex(next) {
					if g.botFoodHasExitAt(candidate, next, index, nextDepth) {
						return nextDepth
					}
				} else if g.botSeen[next] != stamp && g.botFreeAt(candidate, next, nextDepth) {
					g.botSeen[next] = stamp
					queue = append(queue, cellIndex(next))
				}
			}
			if down := index + width; down < cells {
				if g.hasFoodIndex(down) {
					if g.botFoodHasExitAt(candidate, down, index, nextDepth) {
						return nextDepth
					}
				} else if g.botSeen[down] != stamp && g.botFreeAt(candidate, down, nextDepth) {
					g.botSeen[down] = stamp
					queue = append(queue, cellIndex(down))
				}
			}
			col := index % width
			if col != 0 {
				next := index - 1
				if g.hasFoodIndex(next) {
					if g.botFoodHasExitAt(candidate, next, index, nextDepth) {
						return nextDepth
					}
				} else if g.botSeen[next] != stamp && g.botFreeAt(candidate, next, nextDepth) {
					g.botSeen[next] = stamp
					queue = append(queue, cellIndex(next))
				}
			}
			if col != width-1 {
				next := index + 1
				if g.hasFoodIndex(next) {
					if g.botFoodHasExitAt(candidate, next, index, nextDepth) {
						return nextDepth
					}
				} else if g.botSeen[next] != stamp && g.botFreeAt(candidate, next, nextDepth) {
					g.botSeen[next] = stamp
					queue = append(queue, cellIndex(next))
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

	offset := int(g.botBodyOrder[index])
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

func (g *game) markBotBody() {
	g.botBodyStamp++
	if g.botBodyStamp == 0 {
		clear(g.botBodySeen)
		g.botBodyStamp = 1
	}

	for offset := 0; offset < g.length; offset++ {
		index := int(g.snake[g.ring(offset)])
		g.botBodySeen[index] = g.botBodyStamp
		g.botBodyOrder[index] = cellIndex(offset)
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
		h = botHashMix(h, uint64(g.food[i])+1)
	}
	for i := 0; i < g.length; i++ {
		h = botHashMix(h, uint64(g.snake[g.ring(i)])+1)
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

func (g *game) nextBotStamp() uint16 {
	g.botStamp++
	if g.botStamp == 0 {
		clear(g.botSeen)
		g.botStamp = 1
	}
	return g.botStamp
}
