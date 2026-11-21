package cli_test

import (
	"bytes"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"taskflow/internal/cli"
	"taskflow/internal/service"
	"taskflow/internal/storage"
	"taskflow/internal/task"
)

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

var today = time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)

type app struct {
	svc *service.Service
	cli *cli.CLI
	out *bytes.Buffer
}

func newApp(t *testing.T, proc service.Processor, queueSize int) *app {
	t.Helper()
	clock := fixedClock{now: today}
	svc, err := service.New(storage.NewMemoryStore(), clock, proc, service.Config{Workers: 2, QueueSize: queueSize})
	if err != nil {
		t.Fatalf("service.New: %v", err)
	}
	t.Cleanup(svc.Shutdown)
	out := &bytes.Buffer{}
	return &app{svc: svc, cli: cli.New(svc, out, clock), out: out}
}

func (a *app) run(t *testing.T, script string) string {
	t.Helper()
	if err := a.cli.Run(strings.NewReader(script)); err != nil {
		t.Fatalf("Run: %v", err)
	}
	return a.out.String()
}

func assertContains(t *testing.T, out string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(out, want) {
			t.Errorf("output does not contain %q\n--- output ---\n%s", want, out)
		}
	}
}

func TestCLI_AddSubmitAndShowResult(t *testing.T) {
	a := newApp(t, nil, 5)
	a.run(t, `
add Подготовиться к защите !high #go #Study @2026-11-24 | Повторить пул воркеров, очередь и пул тестов.
add Позвонить в деканат
submit 1 2
`)
	a.svc.Shutdown()
	out := a.run(t, "list\nshow 1\nshow 2\nstats\n")

	assertContains(t, out,
		"создана задача #1",
		"создана задача #2",
		"задача #1 в очереди",
		"задача #2 в очереди",
		"#1   done         high    Подготовиться к защите  #go #study  до 24.11.2026  [7 слов, воркер ",
		"#2   failed       medium  Позвонить в деканат  [ошибка анализа, воркер ",
		"статус:    done",
		"описание:  Повторить пул воркеров, очередь и пул тестов.",
		"анализ:    7 слов, 45 символов, 6 уникальных слов",
		"ключевые:  воркеров, очередь, повторить",
		"ошибка:    description has no words to analyze",
		"done         1",
		"failed       1",
		"всего        2",
		"очередь:     0 из 5, воркеров: 2",
	)
}

func TestCLI_ListFilters(t *testing.T) {
	a := newApp(t, nil, 5)
	out := a.run(t, `
add a !high #go
add b !low #go
add c !high
cancel 2
list !high #go
`)
	last := out[strings.LastIndex(out, "> #"):]
	assertContains(t, last, "#1   todo")
	if strings.Contains(last, "#2 ") || strings.Contains(last, "#3 ") {
		t.Errorf("filter returned extra tasks:\n%s", last)
	}
	assertContains(t, a.run(t, "list cancelled\n"), "#2   cancelled")
}

func TestCLI_Errors(t *testing.T) {
	a := newApp(t, nil, 5)
	out := a.run(t, `
add
add x !urgent
add y @24.11.2026
add z @2020-01-01
show 99
show abc
submit
submit abc 99
list blocked
start 1
foo
add ok | текст
cancel 1
`)
	assertContains(t, out,
		"ошибка: некорректное поле title",
		"ошибка: некорректное поле priority",
		`ошибка: некорректная дата "24.11.2026"`,
		"ошибка: некорректное поле deadline",
		"ошибка: задача не найдена (get: task 99: task not found)",
		`ошибка: некорректный id "abc"`,
		"ошибка: нужно указать хотя бы один id",
		"ошибка: задача не найдена (submit: task 99: task not found)",
		"ошибка: некорректное поле status",
		`ошибка: неизвестная команда "start"`,
		`ошибка: неизвестная команда "foo"`,
		"задача #1: cancelled",
	)
}

func TestCLI_InvalidStatesAfterProcessing(t *testing.T) {
	a := newApp(t, nil, 5)
	a.run(t, "add ok | Hello Go world\nsubmit 1\n")
	a.svc.Shutdown()
	out := a.run(t, "submit 1\ncancel 1\nreopen 1\nadd new | текст\nsubmit 2\nshow 2\n")

	assertContains(t, out,
		"ошибка: нельзя перевести задачу #1 из done в queued (submit: task 1: done -> queued: invalid status transition)",
		"ошибка: нельзя перевести задачу #1 из done в cancelled",
		"ошибка: нельзя перевести задачу #1 из done в todo",
		"ошибка: обработка остановлена, новые задачи не принимаются (submit: task 2: worker pool is stopped)",
		"статус:    todo",
	)
}

type gateProcessor struct {
	started chan task.ID
	release chan struct{}
	once    sync.Once
}

func (p *gateProcessor) Process(t task.Task) (task.Result, error) {
	p.started <- t.ID
	<-p.release
	return task.Analyze(t.Description)
}

func (p *gateProcessor) open() { p.once.Do(func() { close(p.release) }) }

func waitStarted(t *testing.T, ch <-chan task.ID) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for a worker")
	}
}

func TestCLI_QueueFullAndBusyTasks(t *testing.T) {
	proc := &gateProcessor{started: make(chan task.ID, 10), release: make(chan struct{})}
	a := newApp(t, proc, 1)
	t.Cleanup(proc.open)

	a.run(t, "add a | текст\nadd b | текст\nadd c | текст\nadd d | текст\nsubmit 1\n")
	waitStarted(t, proc.started)
	a.run(t, "submit 2\n")
	waitStarted(t, proc.started)
	out := a.run(t, "submit 3 4\ndelete 1\nstats\n")

	assertContains(t, out,
		"задача #3 в очереди",
		"ошибка: очередь заполнена, задача осталась в todo (submit: task 4: queue is full)",
		"ошибка: задача в очереди или в обработке (delete: task 1: task is queued or being processed)",
		"queued       1",
		"in_progress  2",
		"очередь:     1 из 1, воркеров: 2",
	)
	proc.open()
}

func TestCLI_OverdueAndHelp(t *testing.T) {
	a := newApp(t, nil, 5)
	out := a.run(t, "add today @2026-11-21\noverdue\nhelp\n")
	assertContains(t, out, "создана задача #1", "задач нет", "Команды:", "submit <id>")
}

func TestCLI_ShowsOverdueMark(t *testing.T) {
	svc, err := service.New(storage.NewMemoryStore(), fixedClock{now: today}, nil, service.Config{Workers: 2, QueueSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(svc.Shutdown)
	var out bytes.Buffer
	app := cli.New(svc, &out, fixedClock{now: today.AddDate(0, 0, 7)})

	if err := app.Execute("add late @2026-11-22"); err != nil {
		t.Fatal(err)
	}
	if err := app.Execute("list"); err != nil {
		t.Fatal(err)
	}
	assertContains(t, out.String(), "[просрочена]")
}

func TestCLI_ExitStopsReading(t *testing.T) {
	a := newApp(t, nil, 5)
	out := a.run(t, "add first\nexit\nadd second\n")
	if strings.Contains(out, "#2") {
		t.Fatalf("commands after exit were executed:\n%s", out)
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("disk on fire") }

func TestCLI_ReadErrorIsReturned(t *testing.T) {
	a := newApp(t, nil, 5)
	if err := a.cli.Run(errReader{}); err == nil || err.Error() != "disk on fire" {
		t.Fatalf("err = %v, want read error", err)
	}
}

// Regression test for risk R1: an over-long line must be rejected without ending the session.
func TestCLI_LongLineIsRejectedAndSessionContinues(t *testing.T) {
	a := newApp(t, nil, 5)
	long := "add x | " + strings.Repeat("а", 70*1024)
	out := a.run(t, "add first\n"+long+"\nadd second\n")
	assertContains(t, out,
		"создана задача #1",
		"ошибка: строка слишком длинная",
		"создана задача #2",
	)
	if strings.Contains(out, "создана задача #3") {
		t.Errorf("the over-long line was executed:\n%s", out)
	}
}

// The last line may have no trailing newline and must still be executed.
func TestCLI_LastLineWithoutNewline(t *testing.T) {
	a := newApp(t, nil, 5)
	assertContains(t, a.run(t, "add first\r\nadd second"), "создана задача #1", "создана задача #2")
}

// Regression test for risk R2: only a bare "exit"/"quit" ends the session; a line that
// merely starts with one of these words must not lose the user's tasks.
func TestCLI_ExitWithArgumentsDoesNotEndSession(t *testing.T) {
	a := newApp(t, nil, 5)
	out := a.run(t, "add first\nquit smoking by friday\nexit now\nadd second\n")
	assertContains(t, out, "создана задача #2", "ошибка: команда exit/quit не принимает аргументов")
}
