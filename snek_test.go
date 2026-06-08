package snek

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func TestStepMovesSnakeWithoutGrowing(t *testing.T) {
	g := newGame(40, 20, 1)
	n := g.length
	setFoods(g, point{0, 0})

	g.step()

	if g.over {
		t.Fatal("expected game to continue")
	}
	if g.length != n {
		t.Fatalf("expected length %d, got %d", n, g.length)
	}
	if got, want := g.headPoint(), (point{g.width/2 + 1, g.height / 2}); got != want {
		t.Fatalf("expected head at %+v, got %+v", want, got)
	}
}

func TestStepEatsFoodAndGrows(t *testing.T) {
	g := newGame(40, 20, 1)
	n := g.length
	head := g.headPoint()
	setFoods(g, point{head.x + 1, head.y})

	g.step()

	if g.over {
		t.Fatal("expected game to continue")
	}
	if g.score != 1 {
		t.Fatalf("expected score 1, got %d", g.score)
	}
	if got, want := g.length, n+1; got != want {
		t.Fatalf("expected length %d, got %d", want, got)
	}
	if g.foods != 1 {
		t.Fatalf("expected one controlled food after eating, got %d", g.foods)
	}
}

func TestStepEndsGameAtWall(t *testing.T) {
	g := newGame(4, 4, 1)
	clear(g.occupied)
	setSnake(g, point{3, 1})
	g.dir = dirRight
	g.next = dirRight
	setFoods(g, point{0, 0})

	g.step()

	if !g.over {
		t.Fatal("expected wall collision to end the game")
	}
}

func TestTurnRejectsReverseDirection(t *testing.T) {
	g := newGame(40, 20, 1)

	g.turn(moveLeft)
	if g.next != dirRight {
		t.Fatalf("expected reverse turn to be rejected, got %+v", g.next)
	}
	g.turn(moveUp)
	if g.next != dirUp {
		t.Fatalf("expected upward turn to be queued, got %+v", g.next)
	}
}

func TestBotActionMovesTowardReachableFood(t *testing.T) {
	g := newGame(12, 8, 1)
	head := g.headPoint()
	setFoods(g, point{x: head.x + 1, y: head.y})

	got, ok := botAction(g)
	if !ok {
		t.Fatal("expected bot to find a move")
	}
	if got != moveRight {
		t.Fatalf("expected bot to move right toward food, got %d", got)
	}
}

func TestBotActionAvoidsImmediateWall(t *testing.T) {
	g := newGame(6, 6, 1)
	setSnake(g, point{5, 3})
	g.dir = dirRight
	g.next = dirRight
	setFoods(g, point{0, 3})

	got, ok := botAction(g)
	if !ok {
		t.Fatal("expected bot to find a move")
	}
	if got == moveRight {
		t.Fatal("expected bot to avoid moving into the wall")
	}
}

func TestBotActionAvoidsTrapFood(t *testing.T) {
	g := newGame(5, 5, 1)
	setSnake(g,
		point{2, 1},
		point{2, 2},
		point{3, 2},
		point{4, 2},
		point{4, 1},
		point{3, 0},
		point{0, 0},
	)
	g.dir = dirRight
	g.next = dirRight
	setFoods(g, point{3, 1})

	got, ok := botAction(g)
	if !ok {
		t.Fatal("expected bot to find a move")
	}
	if got != moveUp {
		t.Fatalf("expected bot to avoid trapped food and move up, got %d", got)
	}
}

func TestBotSimulatorEatsTailRingFood(t *testing.T) {
	g := newGame(5, 5, 1)
	setSnake(g,
		point{1, 1},
		point{1, 2},
		point{1, 3},
		point{2, 3},
		point{3, 3},
		point{3, 2},
		point{3, 1},
		point{2, 1},
	)
	g.dir = dirUp
	g.next = dirUp
	setFoods(g, point{2, 2})

	result := simulateBot(g, 32)
	if result.loop {
		t.Fatalf("bot repeated a state before eating food: loop %d->%d\n%s",
			result.loopStart, result.steps, result.traceString())
	}
	if result.over {
		t.Fatalf("bot died before eating food\n%s", result.traceString())
	}
	if result.noMove {
		t.Fatalf("bot had no legal move before eating food\n%s", result.traceString())
	}
	if g.score == 0 {
		t.Fatalf("bot did not eat food in %d simulated steps\n%s", result.steps, result.traceString())
	}
}

func TestBotSimulatorEatsEdgeFood(t *testing.T) {
	g := newGame(5, 5, 1)
	setSnake(g,
		point{1, 0},
		point{1, 1},
		point{2, 1},
		point{3, 1},
		point{3, 0},
	)
	g.dir = dirUp
	g.next = dirUp
	setFoods(g, point{2, 0})

	result := simulateBot(g, 64)
	if result.loop {
		t.Fatalf("bot repeated a state-action before eating edge food: loop %d->%d\n%s",
			result.loopStart, result.steps, result.traceString())
	}
	if result.over {
		t.Fatalf("bot died before eating edge food\n%s", result.traceString())
	}
	if result.noMove {
		t.Fatalf("bot had no legal move before eating edge food\n%s", result.traceString())
	}
	if g.score == 0 {
		t.Fatalf("bot did not eat edge food in %d simulated steps\n%s", result.steps, result.traceString())
	}
}

func TestBotSimulatorEatsSingleFoodPlacements(t *testing.T) {
	const (
		width  = 12
		height = 8
		limit  = width * height * 4
	)

	for y := uint16(0); y < height; y++ {
		for x := uint16(0); x < width; x++ {
			g := newGame(width, height, 1)
			food := point{x: x, y: y}
			if g.has(food) {
				continue
			}
			setFoods(g, food)

			result := simulateBot(g, limit)
			if result.loop {
				t.Fatalf("bot looped before eating food at %+v: loop %d->%d\n%s",
					food, result.loopStart, result.steps, result.traceString())
			}
			if result.over {
				t.Fatalf("bot died before eating food at %+v\n%s", food, result.traceString())
			}
			if result.noMove {
				t.Fatalf("bot had no legal move before eating food at %+v\n%s", food, result.traceString())
			}
			if g.score == 0 {
				t.Fatalf("bot did not eat food at %+v in %d simulated steps\n%s",
					food, result.steps, result.traceString())
			}
		}
	}
}

func TestBotDirectSingleFoodReducesDistance(t *testing.T) {
	g := newGame(12, 8, 1)
	food := point{x: 2, y: 0}
	setFoods(g, food)

	for step := 0; step < 16 && g.score == 0; step++ {
		before := manhattan(g.headPoint(), food)
		move, ok := botAction(g)
		if !ok {
			t.Fatal("expected bot to find a move")
		}
		g.turn(move)
		g.step()
		if g.over {
			t.Fatalf("bot died after move %s", actionName(move))
		}
		after := manhattan(g.headPoint(), food)
		if g.score == 0 && after >= before {
			t.Fatalf("expected move %s to reduce distance from %d, got %d", actionName(move), before, after)
		}
	}
	if g.score == 0 {
		t.Fatal("expected bot to eat the food")
	}
}

func TestNewGameStartsWithConfiguredFoodCount(t *testing.T) {
	g := newGame(40, 20, 1)

	if g.foods != foodCount {
		t.Fatalf("expected %d foods, got %d", foodCount, g.foods)
	}
	for i := 0; i < g.foods; i++ {
		if g.hasIndex(int(g.food[i])) {
			t.Fatalf("food %d spawned on the snake", i)
		}
		for j := i + 1; j < g.foods; j++ {
			if g.food[i] == g.food[j] {
				t.Fatalf("foods %d and %d overlap at %+v", i, j, g.pointAt(int(g.food[i])))
			}
		}
	}
}

func TestEatingFoodKeepsConfiguredFoodCountVisible(t *testing.T) {
	g := newGame(40, 20, 1)
	head := g.headPoint()
	moveFood(g, 0, point{head.x + 1, head.y})

	g.step()

	if g.over {
		t.Fatal("expected game to continue")
	}
	if g.foods != foodCount {
		t.Fatalf("expected %d foods after eating, got %d", foodCount, g.foods)
	}
	if g.isFood(g.headPoint()) {
		t.Fatal("expected eaten food to be replaced away from the head")
	}
}

func TestBoardSizeFitsTerminal(t *testing.T) {
	width, height, ok := boardSize(40, 12)
	if !ok {
		t.Fatal("expected 40x12 terminal to fit the minimum board")
	}
	if width*tileWidth != 40 {
		t.Fatalf("expected board to use 40 columns, got %d", width*tileWidth)
	}
	if height+hudRows != 12 {
		t.Fatalf("expected board to use 12 rows, got %d", height+hudRows)
	}
}

func TestBoardSizeRejectsTinyTerminal(t *testing.T) {
	_, _, ok := boardSize(20, 12)
	if ok {
		t.Fatal("expected terminal with 20 columns to be rejected")
	}
}

func TestBoardSizeCapsCellsAndKeepsEvenWidth(t *testing.T) {
	width, height, ok := boardSize(65535, 65535)
	if !ok {
		t.Fatal("expected huge terminal to fit after capping")
	}
	if width%2 != 0 {
		t.Fatalf("expected even board width, got %d", width)
	}
	if int(width)*int(height) > maxBoardCells {
		t.Fatalf("expected at most %d cells, got %d", maxBoardCells, int(width)*int(height))
	}
}

func TestModesGetFaster(t *testing.T) {
	for i := 1; i < len(modes); i++ {
		if modes[i].tick >= modes[i-1].tick {
			t.Fatalf("expected %s to be faster than %s", modes[i].name, modes[i-1].name)
		}
	}
}

func TestModeIndexForAction(t *testing.T) {
	tests := []struct {
		input action
		want  int
	}{
		{input: easy, want: 0},
		{input: normal, want: 1},
		{input: hard, want: 2},
		{input: expert, want: 3},
		{input: bananas, want: 4},
	}

	for _, test := range tests {
		got, ok := modeIndexForAction(test.input)
		if !ok {
			t.Fatalf("expected action %d to map to a mode", test.input)
		}
		if got != test.want {
			t.Fatalf("expected mode index %d, got %d", test.want, got)
		}
	}
}

func TestWriteCentered(t *testing.T) {
	var buf bytes.Buffer
	out := bufio.NewWriter(&buf)

	writeCentered(out, 1, 11, "SNAKE", fgWhite, false)
	if err := out.Flush(); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); !strings.Contains(got, "   SNAKE   ") {
		t.Fatalf("expected centered text in %q", got)
	}
}

func TestHeadTileUsesRoundedGlyph(t *testing.T) {
	if !strings.Contains(headTile, dotGlyph) {
		t.Fatal("expected head tile to use a rounded glyph")
	}
}

func TestBotCycleOrderCoversEvenDimensionBoards(t *testing.T) {
	for _, size := range []struct {
		width  uint16
		height uint16
	}{
		{width: 12, height: 7},
		{width: 13, height: 8},
		{width: 12, height: 8},
	} {
		g := newGame(size.width, size.height, 1)
		cells := int(size.width) * int(size.height)
		byOrder := make([]point, cells)
		seen := make([]bool, cells)
		for y := uint16(0); y < size.height; y++ {
			for x := uint16(0); x < size.width; x++ {
				p := point{x: x, y: y}
				order := g.botCycleOrder(g.index(p))
				if order < 0 || order >= cells {
					t.Fatalf("%dx%d order %d out of range for %+v", size.width, size.height, order, p)
				}
				if seen[order] {
					t.Fatalf("%dx%d duplicate cycle order %d", size.width, size.height, order)
				}
				seen[order] = true
				byOrder[order] = p
			}
		}

		for i := 0; i < cells; i++ {
			a := byOrder[i]
			b := byOrder[(i+1)%cells]
			if abs(int(a.x)-int(b.x))+abs(int(a.y)-int(b.y)) != 1 {
				t.Fatalf("%dx%d cycle step %d jumps from %+v to %+v", size.width, size.height, i, a, b)
			}
		}
	}
}

func TestReadInputTogglesBot(t *testing.T) {
	actions := make(chan action, 1)

	readInput(bufio.NewReader(strings.NewReader("b")), actions)

	select {
	case got := <-actions:
		if got != toggleBot {
			t.Fatalf("expected toggle bot action, got %d", got)
		}
	default:
		t.Fatal("expected bot toggle action")
	}
}

func TestBotStateIsLazyAndReleasable(t *testing.T) {
	g := newGame(40, 20, 1)
	if g.botSeen != nil || g.botQueue != nil || g.botBodySeen != nil || g.botBodyOrder != nil {
		t.Fatal("expected bot buffers to start unallocated")
	}

	if _, ok := botAction(g); !ok {
		t.Fatal("expected bot to find a move")
	}
	if len(g.botSeen) == 0 || len(g.botQueue) == 0 || len(g.botBodySeen) == 0 || len(g.botBodyOrder) == 0 {
		t.Fatal("expected bot buffers to be allocated after bot action")
	}

	g.releaseBotState()
	if g.botSeen != nil || g.botQueue != nil || g.botBodySeen != nil || g.botBodyOrder != nil {
		t.Fatal("expected bot buffers to be released")
	}
}

func TestBotActionDoesNotAllocate(t *testing.T) {
	g := newGame(40, 20, 1)
	if _, ok := botAction(g); !ok {
		t.Fatal("expected bot to find a move")
	}

	allocs := testing.AllocsPerRun(1000, func() {
		botAction(g)
	})
	if allocs != 0 {
		t.Fatalf("expected bot action to allocate 0 times, got %.2f", allocs)
	}
}

func TestStepDoesNotAllocate(t *testing.T) {
	g := newGame(40, 20, 1)
	setFoods(g, point{0, 0})

	allocs := testing.AllocsPerRun(1000, func() {
		g.step()
	})
	if allocs != 0 {
		t.Fatalf("expected step to allocate 0 times, got %.2f", allocs)
	}
}

func BenchmarkBotAction(b *testing.B) {
	g := newGame(40, 20, 1)
	botAction(g)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		botAction(g)
	}
}

type botSimulationResult struct {
	steps     int
	loopStart int
	loop      bool
	over      bool
	noMove    bool
	trace     []botSimulationStep
}

type botSimulationStep struct {
	step int
	head point
	move action
}

func simulateBot(g *game, maxSteps int) botSimulationResult {
	startScore := g.score
	seen := make(map[string]int, maxSteps)
	result := botSimulationResult{
		loopStart: -1,
		trace:     make([]botSimulationStep, 0, maxSteps),
	}

	for result.steps = 0; result.steps < maxSteps; result.steps++ {
		if g.score != startScore {
			return result
		}
		if g.over {
			result.over = true
			return result
		}

		move, ok := botAction(g)
		if !ok {
			result.noMove = true
			return result
		}
		key := botStateKey(g, move)
		if prev, ok := seen[key]; ok {
			result.loop = true
			result.loopStart = prev
			return result
		}
		seen[key] = result.steps

		result.trace = append(result.trace, botSimulationStep{
			step: result.steps,
			head: g.headPoint(),
			move: move,
		})
		g.turn(move)
		g.step()
	}

	return result
}

func botStateKey(g *game, input action) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d,%d,%d,%d,%d,%d,%d|", g.width, g.height, g.dir.x, g.dir.y, g.next.x, g.next.y, input)
	for i := 0; i < g.foods; i++ {
		p := g.pointAt(int(g.food[i]))
		fmt.Fprintf(&b, "f%d,%d;", p.x, p.y)
	}
	b.WriteByte('|')
	for i := 0; i < g.length; i++ {
		p := g.pointAt(int(g.snake[g.ring(i)]))
		fmt.Fprintf(&b, "%d,%d;", p.x, p.y)
	}
	return b.String()
}

func (r botSimulationResult) traceString() string {
	var b strings.Builder
	for _, step := range r.trace {
		fmt.Fprintf(&b, "step %02d head=(%d,%d) move=%s\n",
			step.step, step.head.x, step.head.y, actionName(step.move))
	}
	return b.String()
}

func actionName(a action) string {
	switch a {
	case moveUp:
		return "up"
	case moveDown:
		return "down"
	case moveLeft:
		return "left"
	case moveRight:
		return "right"
	default:
		return fmt.Sprintf("action(%d)", a)
	}
}

func setSnake(g *game, body ...point) {
	clear(g.occupied)
	g.head = 0
	g.length = len(body)
	for i, p := range body {
		g.snake[i] = cellIndex(g.index(p))
		g.set(p)
	}
}

func setFoods(g *game, foods ...point) {
	clear(g.foodBits)
	g.foods = len(foods)
	for i, p := range foods {
		g.food[i] = cellIndex(g.index(p))
		g.setFood(p)
	}
}

func moveFood(g *game, slot int, p point) {
	if slot < g.foods {
		g.clearFoodIndex(int(g.food[slot]))
	} else {
		g.foods = slot + 1
	}
	g.food[slot] = cellIndex(g.index(p))
	g.setFood(p)
}

func manhattan(a, b point) int {
	return abs(int(a.x)-int(b.x)) + abs(int(a.y)-int(b.y))
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
