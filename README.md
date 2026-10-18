# TaskFlow — Checkpoint 1 (Core Golang)

Консольный трекер задач со статусами. Хранение в памяти, без конкурентности.
Нужен Go 1.22 или новее. Все команды запускаются из папки `taskflow`, где лежит `go.mod`.

## Запуск

| Что | Команда |
|---|---|
| Запустить с демо-задачами | `go run ./cmd/taskflow -demo` |
| Запустить пустым | `go run ./cmd/taskflow` |
| Все тесты | `go test ./...` |
| Тесты с покрытием | `go test -cover ./...` |
| Подробно, с именами тестов | `go test -v ./internal/task` |
| Проверки компилятора | `go build ./...` и `go vet ./...` |

**GoLand:** открой `cmd/taskflow/main.go` → зелёный ▶ рядом с `func main()` → Run.
Чтобы передать `-demo`: Run → Edit Configurations → Program arguments → `-demo`.
Тесты: правый клик по папке `internal` → Run 'go test internal/...' (или ▶ рядом с любой `func TestXxx`).

## Команды в программе

```
add <название> [!low|!medium|!high] [#тег ...] [@ГГГГ-ММ-ДД]   создать задачу
list [статус] [!приоритет] [#тег]                              список с фильтрами
show <id>                                                      подробности
start <id> | done <id> | cancel <id> | reopen <id>             сменить статус
delete <id>                                                    удалить
overdue                                                        просроченные
stats                                                          статистика по статусам
help, exit
```

Пример сессии (`go run ./cmd/taskflow -demo`):

```
> list
#1   in_progress  high    Подготовиться к защите Checkpoint 1  #go #study  до 13.10.2026
#2   done         high    Написать unit-тесты  #go
#3   todo         low     Прочитать про горутины  #go #reading
#4   cancelled    low     Купить продукты
> start 2
ошибка: нельзя перевести задачу #2 из done в in_progress (start: task 2: done -> in_progress: invalid status transition)
> stats
  todo         1
  in_progress  1
  done         1
  cancelled    1
  всего        4
```

## Статусы и переходы

| Откуда | Куда можно |
|---|---|
| todo | in_progress, cancelled |
| in_progress | todo, done, cancelled |
| done | — |
| cancelled | todo |

## Структура

```
taskflow/
├── cmd/taskflow/main.go        сборка приложения: store → service → cli
└── internal/
    ├── task/                   доменная модель (ни от чего не зависит)
    │   ├── task.go             Task, Params, New, TransitionTo, IsOverdue, Clone
    │   ├── status.go           Status + таблица переходов (конечный автомат)
    │   ├── priority.go         Priority
    │   └── errors.go           ErrNotFound, ErrInvalidTransition, ErrValidation,
    │                           TransitionError, ValidationError
    ├── storage/memory.go       MemoryStore: map[task.ID]task.Task
    ├── service/service.go      интерфейсы Repository и Clock, Service, Filter
    └── cli/cli.go              команды; читает io.Reader, пишет в io.Writer
```

Зависимости идут в одну сторону: `cli → service → task ← storage`.
`service` не импортирует `storage` — он знает только интерфейс `Repository`.

## Где в коде каждое требование

| Требование | Где |
|---|---|
| Доменная модель задач и состояний | `task.Task`, `task.Status` + `transitions`, `task.Priority` |
| Структуры и методы | `Task` (методы на `*Task`), `Status`/`Priority`/`Filter` (методы по значению), `MemoryStore`, `Service` |
| Интерфейсы | `service.Repository`, `service.Clock`, `fmt.Stringer` (Status, Priority), `error` (TransitionError, ValidationError), `io.Reader`/`io.Writer` (cli) |
| In-memory хранилище | `storage.MemoryStore`: `map[task.ID]task.Task` + сортировка слайса в `List` |
| Обработка ошибок | sentinel-ошибки, свои типы с `Unwrap`, оборачивание через `%w`, `errors.Is` / `errors.As` в CLI |
| Unit-тесты | 50 тестов (+41 подтест), табличные; покрытие: task, storage, service — 100%, cli — 94% |
