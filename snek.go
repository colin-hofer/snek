// Package snek implements a small, dependency-free Snake game for ANSI
// terminals.
//
// The package keeps the game deliberately simple. The snake body is stored as a
// compact ring of cell indexes, and occupied cells are tracked with one bit per
// board cell. A frame is streamed directly to the terminal writer; no per-frame
// maps, screen buffers, or body copies are allocated.
package snek

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"syscall"
	"time"
	"unsafe"
)

const (
	tileWidth      uint16 = 2
	hudRows        uint16 = 1
	minBoardWidth  uint16 = 12
	minBoardHeight uint16 = 6
	foodCount      int    = 5
	maxBoardCells  int    = 1<<16 - 1
	maxBotLoopSeen int    = 2048
)

const (
	ansiReset = "\x1b[0m"
	ansiBold  = "\x1b[1m"
	bgBlack   = "\x1b[48;5;16m"
	fgWhite   = "\x1b[38;5;255m"
	fgMuted   = "\x1b[38;5;245m"
	fgGreen   = "\x1b[38;5;46m"
	fgGreen2  = "\x1b[38;5;35m"
	fgAmber   = "\x1b[38;5;214m"
	fgRed     = "\x1b[38;5;203m"

	dotGlyph  = "⬤"
	foodGlyph = "◉"

	foodTile     = bgBlack + fgAmber + foodGlyph + " "
	headTile     = bgBlack + fgGreen + dotGlyph + " "
	bodyTile     = bgBlack + fgGreen2 + dotGlyph + " "
	emptyTile    = bgBlack + "  "
	cursorHome   = "\x1b[H"
	spaceChunk   = "                                                                "
	hudSpeedText = "   1-5 speed"
	hudQuitText  = "   q quit"
	hudRestart   = "   r restart"
	hudScoreText = "   score "
	hudGameOver  = "  game over"
	hudBotOff    = "   b bot"
	hudBotOn     = "   bot on"

	overlayStart   = "SNEK"
	overlayOver    = "GAME OVER"
	overlayMove    = "wasd/arrows or b bot"
	overlaySpeedQ  = "   1-5 speed   q quit"
	overlayRestart = "r restart   1-5 speed   q quit"
)

// Options configures a terminal game session.
type Options struct {
	Bot bool
}

// Run starts Snek on the process standard input and output.
func Run() error {
	return RunWithOptions(Options{})
}

// RunWithOptions starts Snek on the process standard input and output.
func RunWithOptions(opts Options) error {
	return RunTerminalWithOptions(os.Stdin, os.Stdout, opts)
}

// RunTerminal starts Snek using in for keyboard input and out for rendering.
// Both files must refer to an interactive terminal.
func RunTerminal(in, out *os.File) error {
	return RunTerminalWithOptions(in, out, Options{})
}

// RunTerminalWithOptions starts Snek using in for keyboard input and out for
// rendering. Both files must refer to an interactive terminal.
func RunTerminalWithOptions(in, out *os.File, opts Options) error {
	if in == nil || out == nil {
		return fmt.Errorf("snek: nil terminal file")
	}

	oldState, err := enableRawMode(int(in.Fd()))
	if err != nil {
		return fmt.Errorf("snek: raw terminal mode: %w", err)
	}
	defer restoreTerminal(int(in.Fd()), oldState)

	actions := make(chan action, 16)
	go readInput(bufio.NewReader(in), actions)

	writer := bufio.NewWriter(out)
	enterGameScreen(writer)
	defer exitGameScreen(writer)

	spec, err := currentBoardSpec(int(out.Fd()))
	if err != nil {
		return err
	}

	var game game
	game.reset(spec.width, spec.height, uint64(time.Now().UnixNano()))

	botEnabled := opts.Bot
	started := botEnabled
	modeIndex := defaultMode
	ticker := time.NewTicker(modes[modeIndex].tick)
	defer ticker.Stop()

	draw(writer, &game, spec, modes[modeIndex], started, botEnabled)

	for {
		select {
		case input := <-actions:
			switch input {
			case quit:
				return nil
			case easy, normal, hard, expert, bananas:
				next, ok := modeIndexForAction(input)
				if ok && next != modeIndex {
					modeIndex = next
					ticker.Reset(modes[modeIndex].tick)
				}
				draw(writer, &game, spec, modes[modeIndex], started, botEnabled)
			case toggleBot:
				botEnabled = !botEnabled
				if !game.over {
					started = started || botEnabled
					if botEnabled {
						if move, ok := botAction(&game); ok {
							game.turn(move)
						}
					} else {
						game.releaseBotState()
					}
				}
				draw(writer, &game, spec, modes[modeIndex], started, botEnabled)
			case restart:
				if game.over {
					spec, err = currentBoardSpec(int(out.Fd()))
					if err != nil {
						return err
					}
					game.reset(spec.width, spec.height, uint64(time.Now().UnixNano()))
					started = botEnabled
					draw(writer, &game, spec, modes[modeIndex], started, botEnabled)
				}
			default:
				if !game.over {
					botEnabled = false
					game.releaseBotState()
					game.turn(input)
					started = true
					draw(writer, &game, spec, modes[modeIndex], started, botEnabled)
				}
			}
		case <-ticker.C:
			nextSpec, err := currentBoardSpec(int(out.Fd()))
			if err != nil {
				return err
			}
			if nextSpec != spec {
				spec = nextSpec
				game.reset(spec.width, spec.height, uint64(time.Now().UnixNano()))
				started = botEnabled
				draw(writer, &game, spec, modes[modeIndex], started, botEnabled)
				continue
			}

			if botEnabled && !game.over {
				if move, ok := botAction(&game); ok {
					game.turn(move)
					started = true
				}
			}
			if started && !game.over {
				game.step()
				draw(writer, &game, spec, modes[modeIndex], started, botEnabled)
			}
		}
	}
}

type point struct {
	x uint16
	y uint16
}

type cellIndex uint16

type direction struct {
	x int8
	y int8
}

var (
	dirUp    = direction{0, -1}
	dirDown  = direction{0, 1}
	dirLeft  = direction{-1, 0}
	dirRight = direction{1, 0}
)

type action uint8

const (
	moveUp action = iota
	moveDown
	moveLeft
	moveRight
	restart
	quit
	toggleBot
	easy
	normal
	hard
	expert
	bananas
)

type mode struct {
	name string
	tick time.Duration
}

var modes = [...]mode{
	{name: "easy", tick: 160 * time.Millisecond},
	{name: "normal", tick: 110 * time.Millisecond},
	{name: "hard", tick: 75 * time.Millisecond},
	{name: "expert", tick: 50 * time.Millisecond},
	{name: "bananas", tick: 10 * time.Millisecond},
}

const defaultMode = 1

// game owns all mutable game state. The snake slice is a ring whose head is at
// snake[head]. occupied and foodBits are bitsets for constant-time membership
// checks.
type game struct {
	width  uint16
	height uint16

	snake    []cellIndex
	occupied []uint64
	head     int
	length   int

	dir      direction
	next     direction
	food     [foodCount]cellIndex
	foodBits []uint64
	foods    int

	score int
	over  bool
	rand  random

	botSeen      []uint16
	botQueue     []cellIndex
	botBodySeen  []uint16
	botBodyOrder []cellIndex
	botLoopHash  []uint64
	botLoopMask  []uint8
	botStamp     uint16
	botBodyStamp uint16
	botLoopScore int
	botLoopLen   int
	botLoopNext  int
}

func newGame(width, height uint16, seed uint64) *game {
	var g game
	g.reset(width, height, seed)
	return &g
}

func (g *game) reset(width, height uint16, seed uint64) {
	cells := int(width) * int(height)
	if cells <= 0 {
		cells = 1
	}
	if len(g.snake) != cells {
		g.snake = make([]cellIndex, cells)
	}
	words := (cells + 63) / 64
	if len(g.occupied) != words {
		g.occupied = make([]uint64, words)
	} else {
		clear(g.occupied)
	}
	if len(g.foodBits) != words {
		g.foodBits = make([]uint64, words)
	} else {
		clear(g.foodBits)
	}
	g.width = width
	g.height = height
	g.head = 0
	g.length = initialLength(width)
	g.dir = dirRight
	g.next = dirRight
	g.foods = 0
	g.score = 0
	g.over = false
	g.rand = newRandom(seed)
	g.releaseBotState()

	centerY := height / 2
	startX := width / 2
	if need := uint16(g.length - 1); startX < need {
		startX = need
	}
	for i := 0; i < g.length; i++ {
		p := point{x: startX - uint16(i), y: centerY}
		g.snake[i] = cellIndex(g.index(p))
		g.set(p)
	}
	for i := 0; i < foodCount; i++ {
		if !g.spawnFood(i) {
			break
		}
	}
}

func (g *game) ensureBotState() {
	cells := int(g.width) * int(g.height)
	if len(g.botSeen) != cells {
		g.botSeen = make([]uint16, cells)
	}
	if len(g.botQueue) != cells {
		g.botQueue = make([]cellIndex, cells)
	}
	if len(g.botBodySeen) != cells {
		g.botBodySeen = make([]uint16, cells)
	}
	if len(g.botBodyOrder) != cells {
		g.botBodyOrder = make([]cellIndex, cells)
	}
	loopCells := min(cells, maxBotLoopSeen)
	if len(g.botLoopHash) != loopCells {
		g.botLoopHash = make([]uint64, loopCells)
	}
	if len(g.botLoopMask) != loopCells {
		g.botLoopMask = make([]uint8, loopCells)
	}
}

func (g *game) releaseBotState() {
	g.botSeen = nil
	g.botQueue = nil
	g.botBodySeen = nil
	g.botBodyOrder = nil
	g.botLoopHash = nil
	g.botLoopMask = nil
	g.botStamp = 0
	g.botBodyStamp = 0
	g.botLoopScore = 0
	g.botLoopLen = 0
	g.botLoopNext = 0
}

func initialLength(width uint16) int {
	if width < 3 {
		return int(width)
	}
	return 3
}

func (g *game) turn(input action) {
	var next direction

	switch input {
	case moveUp:
		next = dirUp
	case moveDown:
		next = dirDown
	case moveLeft:
		next = dirLeft
	case moveRight:
		next = dirRight
	default:
		return
	}

	if opposite(next, g.dir) || opposite(next, g.next) {
		return
	}
	g.next = next
}

func (g *game) step() {
	if g.over {
		return
	}

	g.dir = g.next
	head := g.headPoint()
	nextX := int(head.x) + int(g.dir.x)
	nextY := int(head.y) + int(g.dir.y)
	if nextX < 0 || nextX >= int(g.width) || nextY < 0 || nextY >= int(g.height) {
		g.over = true
		return
	}

	next := point{x: uint16(nextX), y: uint16(nextY)}
	nextIndex := g.index(next)
	eaten, grow := g.foodAtIndex(nextIndex)
	tail := g.tailIndex()
	if g.hasIndex(nextIndex) && (grow || nextIndex != tail) {
		g.over = true
		return
	}

	g.head--
	if g.head < 0 {
		g.head = len(g.snake) - 1
	}
	g.snake[g.head] = cellIndex(nextIndex)
	g.setIndex(nextIndex)

	if grow {
		g.score++
		g.length++
		if g.length >= len(g.snake) || !g.spawnFood(eaten) {
			g.over = true
		}
		return
	}
	if nextIndex != tail {
		g.clearIndex(tail)
	}
}

func (g *game) spawnFood(slot int) bool {
	if slot < 0 || slot >= foodCount {
		return false
	}

	otherFoods := g.foods
	if slot < g.foods {
		otherFoods--
	}
	if g.length+otherFoods >= len(g.snake) {
		return false
	}

	cells := int(g.width) * int(g.height)
	for {
		index := int(g.rand.next(uint32(cells)))
		if !g.hasIndex(index) && !g.foodOccupiesIndex(index, slot) {
			if slot < g.foods {
				g.clearFoodIndex(int(g.food[slot]))
			}
			g.food[slot] = cellIndex(index)
			g.setFoodIndex(index)
			if slot == g.foods {
				g.foods++
			}
			return true
		}
	}
}

func (g *game) foodAt(p point) (int, bool) {
	return g.foodAtIndex(g.index(p))
}

func (g *game) foodAtIndex(index int) (int, bool) {
	if !g.hasFoodIndex(index) {
		return 0, false
	}
	for i := 0; i < g.foods; i++ {
		if int(g.food[i]) == index {
			return i, true
		}
	}
	return 0, false
}

func (g *game) isFood(p point) bool {
	return g.hasFoodIndex(g.index(p))
}

func (g *game) foodOccupies(p point, skip int) bool {
	return g.foodOccupiesIndex(g.index(p), skip)
}

func (g *game) foodOccupiesIndex(index int, skip int) bool {
	if !g.hasFoodIndex(index) {
		return false
	}
	return skip < 0 || skip >= g.foods || int(g.food[skip]) != index
}

func (g *game) headPoint() point {
	return g.pointAt(g.headIndex())
}

func (g *game) tailPoint() point {
	return g.pointAt(g.tailIndex())
}

func (g *game) headIndex() int {
	return int(g.snake[g.head])
}

func (g *game) tailIndex() int {
	return int(g.snake[g.ring(g.length-1)])
}

func (g *game) ring(offset int) int {
	i := g.head + offset
	if i >= len(g.snake) {
		i -= len(g.snake)
	}
	return i
}

func (g *game) has(p point) bool {
	i := g.index(p)
	return g.hasIndex(i)
}

func (g *game) hasIndex(i int) bool {
	return g.occupied[i>>6]&(uint64(1)<<uint(i&63)) != 0
}

func (g *game) hasFoodIndex(i int) bool {
	return g.foodBits[i>>6]&(uint64(1)<<uint(i&63)) != 0
}

func (g *game) set(p point) {
	i := g.index(p)
	g.setIndex(i)
}

func (g *game) clear(p point) {
	i := g.index(p)
	g.clearIndex(i)
}

func (g *game) setIndex(i int) {
	g.occupied[i>>6] |= uint64(1) << uint(i&63)
}

func (g *game) clearIndex(i int) {
	g.occupied[i>>6] &^= uint64(1) << uint(i&63)
}

func (g *game) setFood(p point) {
	i := g.index(p)
	g.setFoodIndex(i)
}

func (g *game) clearFood(p point) {
	i := g.index(p)
	g.clearFoodIndex(i)
}

func (g *game) setFoodIndex(i int) {
	g.foodBits[i>>6] |= uint64(1) << uint(i&63)
}

func (g *game) clearFoodIndex(i int) {
	g.foodBits[i>>6] &^= uint64(1) << uint(i&63)
}

func (g *game) index(p point) int {
	return int(p.y)*int(g.width) + int(p.x)
}

func (g *game) pointAt(index int) point {
	width := int(g.width)
	return point{x: uint16(index % width), y: uint16(index / width)}
}

func opposite(a, b direction) bool {
	return a.x == -b.x && a.y == -b.y
}

type random uint64

func newRandom(seed uint64) random {
	if seed == 0 {
		seed = 1
	}
	return random(seed)
}

func (r *random) next(n uint32) uint32 {
	x := uint64(*r)
	x ^= x >> 12
	x ^= x << 25
	x ^= x >> 27
	*r = random(x)
	return uint32((x * 2685821657736338717) % uint64(n))
}

func modeIndexForAction(input action) (int, bool) {
	switch input {
	case easy:
		return 0, true
	case normal:
		return 1, true
	case hard:
		return 2, true
	case expert:
		return 3, true
	case bananas:
		return 4, true
	default:
		return 0, false
	}
}

type board struct {
	width  uint16
	height uint16
	cols   uint16
	rows   uint16
}

func currentBoardSpec(fd int) (board, error) {
	cols, rows, err := terminalSize(fd)
	if err != nil {
		return board{}, err
	}

	width, height, ok := boardSize(cols, rows)
	if !ok {
		return board{}, fmt.Errorf("snek: terminal too small: need at least %dx%d, got %dx%d",
			minBoardWidth*tileWidth, minBoardHeight+hudRows, cols, rows)
	}
	return board{width: width, height: height, cols: cols, rows: rows}, nil
}

func boardSize(cols, rows uint16) (uint16, uint16, bool) {
	if rows <= hudRows {
		return 0, 0, false
	}

	width := cols / tileWidth
	height := rows - hudRows
	if width > minBoardWidth && width%2 != 0 {
		width--
	}
	if int(width)*int(height) > maxBoardCells {
		if maxHeight := uint16(maxBoardCells / int(width)); maxHeight >= minBoardHeight {
			height = maxHeight
		} else {
			height = minBoardHeight
			width = uint16(maxBoardCells / int(height))
			if width > minBoardWidth && width%2 != 0 {
				width--
			}
		}
	}
	return width, height, width >= minBoardWidth && height >= minBoardHeight
}

func draw(out *bufio.Writer, g *game, b board, m mode, started, botEnabled bool) {
	_, _ = out.WriteString(cursorHome)

	head := g.headPoint()
	index := 0
	for y := uint16(0); y < g.height; y++ {
		writeAt(out, y+1, 1)
		for x := uint16(0); x < g.width; x++ {
			switch {
			case g.hasIndex(index):
				if x == head.x && y == head.y {
					_, _ = out.WriteString(headTile)
				} else {
					_, _ = out.WriteString(bodyTile)
				}
			case g.hasFoodIndex(index):
				_, _ = out.WriteString(foodTile)
			default:
				_, _ = out.WriteString(emptyTile)
			}
			index++
		}
		if rest := b.cols - g.width*tileWidth; rest > 0 {
			_, _ = out.WriteString(bgBlack)
			writeSpaces(out, int(rest))
		}
		_, _ = out.WriteString(ansiReset)
	}

	writeHUD(out, g, b.cols, b.rows, m, botEnabled)
	if !started {
		drawStartOverlay(out, b, m)
	} else if g.over {
		drawGameOverOverlay(out, b, g, m)
	}
	_ = out.Flush()
}

func writeHUD(out *bufio.Writer, g *game, width, row uint16, m mode, botEnabled bool) {
	remaining := int(width)
	if remaining <= 0 {
		return
	}

	accent := fgGreen
	if g.over {
		accent = fgRed
	}

	writeAt(out, row, 1)
	_, _ = out.WriteString(bgBlack)
	_, _ = out.WriteString(accent)
	_, _ = out.WriteString("●")
	remaining--
	_, _ = out.WriteString(fgWhite)

	if g.over {
		writeBounded(out, hudGameOver, &remaining)
		writeBounded(out, "   ", &remaining)
		writeBounded(out, m.name, &remaining)
	} else {
		writeBounded(out, "  ", &remaining)
		writeBounded(out, m.name, &remaining)
	}
	writeBounded(out, hudScoreText, &remaining)
	writeIntBounded(out, g.score, &remaining)
	if g.over {
		writeBounded(out, hudRestart, &remaining)
	}
	writeBounded(out, hudSpeedText, &remaining)
	writeBounded(out, "   wasd/arrows", &remaining)
	if botEnabled {
		writeBounded(out, hudBotOn, &remaining)
	} else {
		writeBounded(out, hudBotOff, &remaining)
	}
	writeBounded(out, hudQuitText, &remaining)
	writeSpaces(out, remaining)
	_, _ = out.WriteString(ansiReset)
}

func drawStartOverlay(out *bufio.Writer, b board, m mode) {
	row := overlayRow(b.height, 3)
	writeCentered(out, row, b.cols, overlayStart, fgGreen, true)
	writeCentered(out, row+1, b.cols, overlayMove, fgMuted, false)
	writeCenteredMode(out, row+2, b.cols, m, fgMuted)
}

func drawGameOverOverlay(out *bufio.Writer, b board, g *game, m mode) {
	row := overlayRow(b.height, 3)
	writeCentered(out, row, b.cols, overlayOver, fgRed, true)
	writeCenteredScore(out, row+1, b.cols, g.score, m, fgMuted)
	writeCentered(out, row+2, b.cols, overlayRestart, fgMuted, false)
}

func overlayRow(height, lines uint16) uint16 {
	if height <= lines {
		return 1
	}
	return (height-lines)/2 + 1
}

func writeCentered(out *bufio.Writer, row, width uint16, text, color string, bold bool) {
	available := int(width)
	if len(text) > available {
		text = text[:available]
	}

	left := (available - len(text)) / 2
	right := available - len(text) - left
	writeOverlayPrefix(out, row, color, bold)
	writeSpaces(out, left)
	_, _ = out.WriteString(text)
	writeSpaces(out, right)
	_, _ = out.WriteString(ansiReset)
}

func writeCenteredMode(out *bufio.Writer, row, width uint16, m mode, color string) {
	available := int(width)
	n := len(m.name) + len(overlaySpeedQ)
	left := 0
	if n < available {
		left = (available - n) / 2
	}

	remaining := available
	writeOverlayPrefix(out, row, color, false)
	writeSpaces(out, left)
	remaining -= left
	writeBounded(out, m.name, &remaining)
	writeBounded(out, overlaySpeedQ, &remaining)
	writeSpaces(out, remaining)
	_, _ = out.WriteString(ansiReset)
}

func writeCenteredScore(out *bufio.Writer, row, width uint16, score int, m mode, color string) {
	available := int(width)
	n := len("score ") + decimalLen(score) + len("   ") + len(m.name)
	left := 0
	if n < available {
		left = (available - n) / 2
	}

	remaining := available
	writeOverlayPrefix(out, row, color, false)
	writeSpaces(out, left)
	remaining -= left
	writeBounded(out, "score ", &remaining)
	writeIntBounded(out, score, &remaining)
	writeBounded(out, "   ", &remaining)
	writeBounded(out, m.name, &remaining)
	writeSpaces(out, remaining)
	_, _ = out.WriteString(ansiReset)
}

func writeOverlayPrefix(out *bufio.Writer, row uint16, color string, bold bool) {
	writeAt(out, row, 1)
	_, _ = out.WriteString(bgBlack)
	_, _ = out.WriteString(color)
	if bold {
		_, _ = out.WriteString(ansiBold)
	}
}

func writeAt(out *bufio.Writer, row, col uint16) {
	var buf [24]byte
	p := buf[:0]
	p = append(p, "\x1b["...)
	p = strconv.AppendUint(p, uint64(row), 10)
	p = append(p, ';')
	p = strconv.AppendUint(p, uint64(col), 10)
	p = append(p, 'H')
	_, _ = out.Write(p)
}

func writeBounded(out *bufio.Writer, text string, remaining *int) {
	if *remaining <= 0 {
		return
	}
	if len(text) > *remaining {
		text = text[:*remaining]
	}
	_, _ = out.WriteString(text)
	*remaining -= len(text)
}

func writeIntBounded(out *bufio.Writer, n int, remaining *int) {
	if *remaining <= 0 {
		return
	}

	var buf [20]byte
	p := strconv.AppendInt(buf[:0], int64(n), 10)
	if len(p) > *remaining {
		p = p[:*remaining]
	}
	_, _ = out.Write(p)
	*remaining -= len(p)
}

func writeSpaces(out *bufio.Writer, n int) {
	for n >= len(spaceChunk) {
		_, _ = out.WriteString(spaceChunk)
		n -= len(spaceChunk)
	}
	if n > 0 {
		_, _ = out.WriteString(spaceChunk[:n])
	}
}

func decimalLen(n int) int {
	if n == 0 {
		return 1
	}

	size := 0
	for n > 0 {
		n /= 10
		size++
	}
	return size
}

func readInput(r *bufio.Reader, actions chan<- action) {
	for {
		b, err := r.ReadByte()
		if err != nil {
			return
		}

		switch b {
		case 'w', 'W':
			send(actions, moveUp)
		case 's', 'S':
			send(actions, moveDown)
		case 'a', 'A':
			send(actions, moveLeft)
		case 'd', 'D':
			send(actions, moveRight)
		case 'r', 'R':
			send(actions, restart)
		case 'q', 'Q', 3:
			send(actions, quit)
		case 'b', 'B':
			send(actions, toggleBot)
		case '1':
			send(actions, easy)
		case '2':
			send(actions, normal)
		case '3':
			send(actions, hard)
		case '4':
			send(actions, expert)
		case '5':
			send(actions, bananas)
		case '\x1b':
			readArrow(r, actions)
		}
	}
}

func readArrow(r *bufio.Reader, actions chan<- action) {
	open, err := r.ReadByte()
	if err != nil || open != '[' {
		return
	}
	key, err := r.ReadByte()
	if err != nil {
		return
	}

	switch key {
	case 'A':
		send(actions, moveUp)
	case 'B':
		send(actions, moveDown)
	case 'C':
		send(actions, moveRight)
	case 'D':
		send(actions, moveLeft)
	}
}

func send(actions chan<- action, a action) {
	select {
	case actions <- a:
	default:
	}
}

func enterGameScreen(out *bufio.Writer) {
	_, _ = out.WriteString("\x1b[?1049h\x1b[?7l" + bgBlack + "\x1b[2J\x1b[?25l")
	_ = out.Flush()
}

func exitGameScreen(out *bufio.Writer) {
	_, _ = out.WriteString("\x1b[0m\x1b[?7h\x1b[?25h\x1b[?1049l")
	_ = out.Flush()
}

func enableRawMode(fd int) (*syscall.Termios, error) {
	old := &syscall.Termios{}
	if err := ioctlTermios(fd, syscall.TCGETS, old); err != nil {
		return nil, err
	}

	raw := *old
	raw.Iflag &^= syscall.BRKINT | syscall.ICRNL | syscall.INPCK | syscall.ISTRIP | syscall.IXON
	raw.Oflag &^= syscall.OPOST
	raw.Cflag |= syscall.CS8
	raw.Lflag &^= syscall.ECHO | syscall.ICANON | syscall.IEXTEN | syscall.ISIG
	raw.Cc[syscall.VMIN] = 1
	raw.Cc[syscall.VTIME] = 0

	if err := ioctlTermios(fd, syscall.TCSETS, &raw); err != nil {
		return nil, err
	}
	return old, nil
}

func restoreTerminal(fd int, state *syscall.Termios) {
	if state != nil {
		_ = ioctlTermios(fd, syscall.TCSETS, state)
	}
}

type windowSize struct {
	rows    uint16
	cols    uint16
	xpixels uint16
	ypixels uint16
}

func terminalSize(fd int) (uint16, uint16, error) {
	var size windowSize
	if err := ioctlWindowSize(fd, syscall.TIOCGWINSZ, &size); err != nil {
		return 0, 0, err
	}
	if size.cols == 0 || size.rows == 0 {
		return 0, 0, fmt.Errorf("snek: terminal size unavailable")
	}
	return size.cols, size.rows, nil
}

func ioctlTermios(fd int, request uintptr, state *syscall.Termios) error {
	_, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		uintptr(fd),
		request,
		uintptr(unsafe.Pointer(state)),
	)
	if errno != 0 {
		return errno
	}
	return nil
}

func ioctlWindowSize(fd int, request uintptr, size *windowSize) error {
	_, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		uintptr(fd),
		request,
		uintptr(unsafe.Pointer(size)),
	)
	if errno != 0 {
		return errno
	}
	return nil
}
