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
	g.foods = 1
	g.food[0] = point{0, 0}

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
	g.foods = 1
	g.food[0] = point{head.x + 1, head.y}

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
	g.head = 0
	g.length = 1
	g.snake[0] = point{3, 1}
	g.set(g.snake[0])
	g.dir = dirRight
	g.next = dirRight
	g.foods = 1
	g.food[0] = point{0, 0}

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
	g.foods = 1
	g.food[0] = point{x: head.x + 1, y: head.y}

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
	g.foods = 1
	g.food[0] = point{0, 3}

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
	g.foods = 1
	g.food[0] = point{3, 1}

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
	g.foods = 1
	g.food[0] = point{2, 2}

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
	g.foods = 1
	g.food[0] = point{2, 0}

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

func TestNewGameStartsWithConfiguredFoodCount(t *testing.T) {
	g := newGame(40, 20, 1)

	if g.foods != foodCount {
		t.Fatalf("expected %d foods, got %d", foodCount, g.foods)
	}
	for i := 0; i < g.foods; i++ {
		if g.has(g.food[i]) {
			t.Fatalf("food %d spawned on the snake", i)
		}
		for j := i + 1; j < g.foods; j++ {
			if g.food[i] == g.food[j] {
				t.Fatalf("foods %d and %d overlap at %+v", i, j, g.food[i])
			}
		}
	}
}

func TestEatingFoodKeepsConfiguredFoodCountVisible(t *testing.T) {
	g := newGame(40, 20, 1)
	head := g.headPoint()
	g.food[0] = point{head.x + 1, head.y}

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

func TestBotActionDoesNotAllocate(t *testing.T) {
	g := newGame(40, 20, 1)

	allocs := testing.AllocsPerRun(1000, func() {
		botAction(g)
	})
	if allocs != 0 {
		t.Fatalf("expected bot action to allocate 0 times, got %.2f", allocs)
	}
}

func TestStepDoesNotAllocate(t *testing.T) {
	g := newGame(40, 20, 1)
	g.foods = 1
	g.food[0] = point{0, 0}

	allocs := testing.AllocsPerRun(1000, func() {
		g.step()
	})
	if allocs != 0 {
		t.Fatalf("expected step to allocate 0 times, got %.2f", allocs)
	}
}

func BenchmarkBotAction(b *testing.B) {
	g := newGame(40, 20, 1)

	b.ReportAllocs()
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
		fmt.Fprintf(&b, "f%d,%d;", g.food[i].x, g.food[i].y)
	}
	b.WriteByte('|')
	for i := 0; i < g.length; i++ {
		p := g.snake[g.ring(i)]
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
		g.snake[i] = p
		g.set(p)
	}
}
