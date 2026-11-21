package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"taskflow/internal/service"
	"taskflow/internal/task"
	"taskflow/internal/worker"
)

// ErrExit is returned by Execute for the "exit" and "quit" commands; Run stops reading input.
var ErrExit = errors.New("exit")

// command is the single place that defines a CLI command: its names, help line and handler.
// The help text and the dispatcher are both built from the same table, so they cannot drift.
type command struct {
	names   []string // the first name is shown in help, the rest are aliases
	usage   string
	summary string
	run     func(c *CLI, args []string, rest string) error
}

func commandTable() []command {
	return []command{
		{names: []string{"add"}, usage: "add <название> [!low|!medium|!high] [#тег ...] [@ГГГГ-ММ-ДД] [| описание]", summary: "создать задачу",
			run: func(c *CLI, _ []string, rest string) error { return c.add(rest) }},
		{names: []string{"submit"}, usage: "submit <id> [id ...]", summary: "отправить задачи на анализ воркерам",
			run: func(c *CLI, args []string, _ string) error { return c.submit(args) }},
		{names: []string{"list"}, usage: "list [статус] [!приоритет] [#тег]", summary: "список (статусы: todo, queued, in_progress, done, failed, cancelled)",
			run: func(c *CLI, args []string, _ string) error { return c.list(args) }},
		{names: []string{"show"}, usage: "show <id>", summary: "подробности и результат анализа",
			run: func(c *CLI, args []string, _ string) error { return c.show(args) }},
		{names: []string{"cancel"}, usage: "cancel <id>", summary: "отменить задачу",
			run: func(c *CLI, args []string, _ string) error { return c.setStatus(args, c.svc.Cancel) }},
		{names: []string{"reopen"}, usage: "reopen <id>", summary: "вернуть задачу в todo (из failed или cancelled)",
			run: func(c *CLI, args []string, _ string) error { return c.setStatus(args, c.svc.Reopen) }},
		{names: []string{"delete"}, usage: "delete <id>", summary: "удалить задачу",
			run: func(c *CLI, args []string, _ string) error { return c.delete(args) }},
		{names: []string{"overdue"}, usage: "overdue", summary: "просроченные задачи",
			run: func(c *CLI, _ []string, _ string) error { return c.overdue() }},
		{names: []string{"stats"}, usage: "stats", summary: "статусы задач и состояние очереди",
			run: func(c *CLI, _ []string, _ string) error { return c.stats() }},
		{names: []string{"help"}, usage: "help", summary: "эта справка",
			run: func(c *CLI, _ []string, _ string) error { c.help(); return nil }},
		{names: []string{"exit", "quit"}, usage: "exit | quit", summary: "выход (воркеры доделают задачи из очереди)",
			run: func(*CLI, []string, string) error { return ErrExit }},
	}
}

type CLI struct {
	svc      *service.Service
	out      io.Writer
	clock    service.Clock
	commands []command
	byName   map[string]command
}

func New(svc *service.Service, out io.Writer, clock service.Clock) *CLI {
	if clock == nil {
		clock = service.SystemClock{}
	}
	c := &CLI{svc: svc, out: out, clock: clock, commands: commandTable(), byName: map[string]command{}}
	for _, cmd := range c.commands {
		for _, name := range cmd.names {
			c.byName[name] = cmd
		}
	}
	return c
}

func (c *CLI) Run(in io.Reader) error {
	scanner := bufio.NewScanner(in)
	c.prompt()
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			if err := c.Execute(line); err != nil {
				if errors.Is(err, ErrExit) {
					return nil
				}
				fmt.Fprintf(c.out, "ошибка: %s\n", describeError(err))
			}
		}
		c.prompt()
	}
	return scanner.Err()
}

func (c *CLI) prompt() {
	fmt.Fprint(c.out, "> ")
}

func (c *CLI) Execute(line string) error {
	line = strings.TrimSpace(line)
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return nil
	}
	name := strings.ToLower(fields[0])
	cmd, ok := c.byName[name]
	if !ok {
		return fmt.Errorf("неизвестная команда %q, введите help", name)
	}
	rest := strings.TrimSpace(line[len(fields[0]):])
	return cmd.run(c, fields[1:], rest)
}

func (c *CLI) help() {
	fmt.Fprintln(c.out, "Команды:")
	for _, cmd := range c.commands {
		fmt.Fprintf(c.out, "  %-74s %s\n", cmd.usage, cmd.summary)
	}
}

func (c *CLI) add(rest string) error {
	head, description, _ := strings.Cut(rest, "|")
	p := task.Params{Description: description}

	var titleWords []string
	for _, arg := range strings.Fields(head) {
		switch {
		case strings.HasPrefix(arg, "!"):
			priority, err := task.ParsePriority(arg[1:])
			if err != nil {
				return err
			}
			p.Priority = priority
		case strings.HasPrefix(arg, "#"):
			p.Tags = append(p.Tags, arg)
		case strings.HasPrefix(arg, "@"):
			deadline, err := c.parseDate(arg[1:])
			if err != nil {
				return err
			}
			p.Deadline = deadline
		default:
			titleWords = append(titleWords, arg)
		}
	}
	p.Title = strings.Join(titleWords, " ")

	t, err := c.svc.Create(p)
	if err != nil {
		return err
	}
	fmt.Fprintf(c.out, "создана задача #%d\n", t.ID)
	return nil
}

func (c *CLI) submit(args []string) error {
	if len(args) == 0 {
		return errors.New("нужно указать хотя бы один id, например: submit 1 2 3")
	}
	for _, arg := range args {
		id, err := parseID([]string{arg})
		if err != nil {
			fmt.Fprintf(c.out, "ошибка: %s\n", describeError(err))
			continue
		}
		if _, err := c.svc.Submit(id); err != nil {
			fmt.Fprintf(c.out, "ошибка: %s\n", describeError(err))
			continue
		}
		fmt.Fprintf(c.out, "задача #%d в очереди\n", id)
	}
	return nil
}

func (c *CLI) list(args []string) error {
	var f service.Filter
	for _, arg := range args {
		switch {
		case strings.HasPrefix(arg, "!"):
			priority, err := task.ParsePriority(arg[1:])
			if err != nil {
				return err
			}
			f.Priority = priority
		case strings.HasPrefix(arg, "#"):
			f.Tag = arg
		default:
			status, err := task.ParseStatus(arg)
			if err != nil {
				return err
			}
			f.Status = status
		}
	}

	tasks, err := c.svc.List(f)
	if err != nil {
		return err
	}
	c.printTasks(tasks)
	return nil
}

func (c *CLI) show(args []string) error {
	id, err := parseID(args)
	if err != nil {
		return err
	}
	t, err := c.svc.Get(id)
	if err != nil {
		return err
	}
	fmt.Fprintf(c.out, "Задача #%d: %s\n", t.ID, t.Title)
	fmt.Fprintf(c.out, "  статус:    %s\n", t.Status())
	fmt.Fprintf(c.out, "  приоритет: %s\n", t.Priority)
	if len(t.Tags) > 0 {
		fmt.Fprintf(c.out, "  теги:      #%s\n", strings.Join(t.Tags, " #"))
	}
	if !t.Deadline.IsZero() {
		fmt.Fprintf(c.out, "  дедлайн:   %s\n", t.Deadline.Format("02.01.2006"))
	}
	if t.Description != "" {
		fmt.Fprintf(c.out, "  описание:  %s\n", t.Description)
	}
	switch t.Status() {
	case task.StatusDone:
		r := t.Result
		fmt.Fprintf(c.out, "  анализ:    %d слов, %d символов, %d уникальных слов\n", r.Words, r.Characters, r.UniqueWords)
		if len(r.Keywords) > 0 {
			fmt.Fprintf(c.out, "  ключевые:  %s\n", strings.Join(r.Keywords, ", "))
		}
		fmt.Fprintf(c.out, "  воркер:    %d\n", t.Worker)
	case task.StatusFailed:
		fmt.Fprintf(c.out, "  ошибка:    %s\n", t.FailReason)
		fmt.Fprintf(c.out, "  воркер:    %d\n", t.Worker)
	}
	fmt.Fprintf(c.out, "  создана:   %s\n", t.CreatedAt.Format("02.01.2006 15:04"))
	fmt.Fprintf(c.out, "  изменена:  %s\n", t.UpdatedAt.Format("02.01.2006 15:04"))
	return nil
}

func (c *CLI) setStatus(args []string, change func(task.ID) (task.Task, error)) error {
	id, err := parseID(args)
	if err != nil {
		return err
	}
	t, err := change(id)
	if err != nil {
		return err
	}
	fmt.Fprintf(c.out, "задача #%d: %s\n", t.ID, t.Status())
	return nil
}

func (c *CLI) delete(args []string) error {
	id, err := parseID(args)
	if err != nil {
		return err
	}
	if err := c.svc.Delete(id); err != nil {
		return err
	}
	fmt.Fprintf(c.out, "задача #%d удалена\n", id)
	return nil
}

func (c *CLI) overdue() error {
	tasks, err := c.svc.Overdue()
	if err != nil {
		return err
	}
	c.printTasks(tasks)
	return nil
}

func (c *CLI) stats() error {
	counts, err := c.svc.Stats()
	if err != nil {
		return err
	}
	total := 0
	for _, st := range task.AllStatuses() {
		fmt.Fprintf(c.out, "  %-12s %d\n", st, counts[st])
		total += counts[st]
	}
	fmt.Fprintf(c.out, "  %-12s %d\n", "всего", total)

	q := c.svc.Queue()
	fmt.Fprintf(c.out, "  очередь:     %d из %d, воркеров: %d\n", q.Queued, q.Capacity, q.Workers)
	return nil
}

func (c *CLI) printTasks(tasks []task.Task) {
	if len(tasks) == 0 {
		fmt.Fprintln(c.out, "задач нет")
		return
	}
	now := c.clock.Now()
	for _, t := range tasks {
		line := fmt.Sprintf("#%-3d %-12s %-7s %s", t.ID, t.Status(), t.Priority, t.Title)
		if len(t.Tags) > 0 {
			line += "  #" + strings.Join(t.Tags, " #")
		}
		if !t.Deadline.IsZero() {
			line += "  до " + t.Deadline.Format("02.01.2006")
		}
		switch t.Status() {
		case task.StatusDone:
			line += fmt.Sprintf("  [%d слов, воркер %d]", t.Result.Words, t.Worker)
		case task.StatusFailed:
			line += fmt.Sprintf("  [ошибка анализа, воркер %d]", t.Worker)
		}
		if t.IsOverdue(now) {
			line += "  [просрочена]"
		}
		fmt.Fprintln(c.out, line)
	}
}

func (c *CLI) parseDate(s string) (time.Time, error) {
	day, err := time.ParseInLocation("2006-01-02", s, c.clock.Now().Location())
	if err != nil {
		return time.Time{}, fmt.Errorf("некорректная дата %q, нужен формат ГГГГ-ММ-ДД: %w", s, err)
	}
	return day.Add(24*time.Hour - time.Second), nil
}

func parseID(args []string) (task.ID, error) {
	if len(args) != 1 {
		return 0, errors.New("нужно указать один id задачи, например: show 1")
	}
	n, err := strconv.Atoi(args[0])
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("некорректный id %q: нужно положительное число", args[0])
	}
	return task.ID(n), nil
}

func describeError(err error) string {
	var transitionErr *task.TransitionError
	var validationErr *task.ValidationError

	switch {
	case errors.As(err, &transitionErr):
		return fmt.Sprintf("нельзя перевести задачу #%d из %s в %s (%v)",
			transitionErr.ID, transitionErr.From, transitionErr.To, err)
	case errors.As(err, &validationErr):
		return fmt.Sprintf("некорректное поле %s (%v)", validationErr.Field, err)
	case errors.Is(err, task.ErrNotFound):
		return fmt.Sprintf("задача не найдена (%v)", err)
	case errors.Is(err, task.ErrTaskBusy):
		return fmt.Sprintf("задача в очереди или в обработке (%v)", err)
	case errors.Is(err, worker.ErrQueueFull):
		return fmt.Sprintf("очередь заполнена, задача осталась в todo (%v)", err)
	case errors.Is(err, worker.ErrStopped):
		return fmt.Sprintf("обработка остановлена, новые задачи не принимаются (%v)", err)
	default:
		return err.Error()
	}
}
