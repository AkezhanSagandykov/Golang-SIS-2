package cli_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"taskflow/internal/cli"
	"taskflow/internal/service"
	"taskflow/internal/storage"
)

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

func run(t *testing.T, script string) string {
	t.Helper()
	clock := fixedClock{now: time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)}
	svc := service.New(storage.NewMemoryStore(), clock)
	var out bytes.Buffer
	if err := cli.New(svc, &out, clock).Run(strings.NewReader(script)); err != nil {
		t.Fatalf("Run: %v", err)
	}
	return out.String()
}

func assertContains(t *testing.T, out string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(out, want) {
			t.Errorf("output does not contain %q\n--- output ---\n%s", want, out)
		}
	}
}

func TestCLI_HappyPath(t *testing.T) {
	out := run(t, `
add Подготовиться к защите !high #go #Study @2026-10-13
add Купить продукты !low
start 1
list
done 1
list done
show 1
cancel 2
stats
`)
	assertContains(t, out,
		"создана задача #1",
		"создана задача #2",
		"задача #1: in_progress",
		"#1   in_progress  high    Подготовиться к защите  #go #study  до 13.10.2026",
		"#2   todo         low     Купить продукты",
		"задача #1: done",
		"статус:    done",
		"теги:      #go #study",
		"задача #2: cancelled",
		"todo         0",
		"done         1",
		"cancelled    1",
		"всего        2",
	)
}

func TestCLI_ListFilters(t *testing.T) {
	out := run(t, `
add a !high #go
add b !low #go
add c !high
list !high #go
`)
	last := out[strings.LastIndex(out, "> #"):]
	assertContains(t, last, "#1   todo")
	if strings.Contains(last, "#2 ") || strings.Contains(last, "#3 ") {
		t.Errorf("filter returned extra tasks:\n%s", last)
	}
}

func TestCLI_Errors(t *testing.T) {
	out := run(t, `
add
add x !urgent
add y @13.10.2026
add z @2020-01-01
start 99
start abc
start
list blocked
foo
add ok
done 1
delete 1
delete 1
list
`)
	assertContains(t, out,
		"ошибка: некорректное поле title",
		"ошибка: некорректное поле priority",
		`ошибка: некорректная дата "13.10.2026"`,
		"ошибка: некорректное поле deadline",
		"ошибка: задача не найдена (start: task 99: task not found)",
		`ошибка: некорректный id "abc"`,
		"ошибка: нужно указать один id",
		"ошибка: некорректное поле status",
		`ошибка: неизвестная команда "foo"`,
		"ошибка: нельзя перевести задачу #1 из todo в done (complete: task 1: todo -> done: invalid status transition)",
		"задача #1 удалена",
		"ошибка: задача не найдена (delete: task 1: task not found)",
		"задач нет",
	)
}

func TestCLI_OverdueAndHelp(t *testing.T) {
	out := run(t, "add today @2026-10-10\noverdue\nhelp\n")
	assertContains(t, out, "создана задача #1", "задач нет", "Команды:")
}

func TestCLI_ShowsOverdueMark(t *testing.T) {
	today := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	svc := service.New(storage.NewMemoryStore(), fixedClock{now: today})
	var out bytes.Buffer
	app := cli.New(svc, &out, fixedClock{now: today.AddDate(0, 0, 7)})

	if err := app.Execute("add late @2026-10-11"); err != nil {
		t.Fatal(err)
	}
	if err := app.Execute("list"); err != nil {
		t.Fatal(err)
	}
	assertContains(t, out.String(), "[просрочена]")
}

func TestCLI_ExitStopsReading(t *testing.T) {
	out := run(t, "add first\nexit\nadd second\n")
	if strings.Contains(out, "#2") {
		t.Fatalf("commands after exit were executed:\n%s", out)
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("disk on fire") }

func TestCLI_ReadErrorIsReturned(t *testing.T) {
	svc := service.New(storage.NewMemoryStore(), nil)
	err := cli.New(svc, &bytes.Buffer{}, nil).Run(errReader{})
	if err == nil || err.Error() != "disk on fire" {
		t.Fatalf("err = %v, want read error", err)
	}
}
