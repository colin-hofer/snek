package snake

import (
	"bufio"
	"bytes"
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

func TestNewGameStartsWithFiveFoods(t *testing.T) {
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

func TestEatingOneFoodKeepsFiveVisible(t *testing.T) {
	g := newGame(40, 20, 1)
	head := g.headPoint()
	g.food[2] = point{head.x + 1, head.y}

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
