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
)

const helpText = `Команды:
  add <название> [!low|!medium|!high] [#тег ...] [@ГГГГ-ММ-ДД]   создать задачу
  list [статус] [!приоритет] [#тег]                              список (статусы: todo, in_progress, done, cancelled)
  show <id>                                                      подробности о задаче
  start <id> | done <id> | cancel <id> | reopen <id>             сменить статус
  delete <id>                                                    удалить задачу
  overdue                                                        просроченные задачи
  stats                                                          сколько задач в каждом статусе
  help                                                           эта справка
  exit                                                           выход`

type CLI struct {
	svc   *service.Service
	out   io.Writer
	clock service.Clock
}

func New(svc *service.Service, out io.Writer, clock service.Clock) *CLI {
	if clock == nil {
		clock = service.SystemClock{}
	}
	return &CLI{svc: svc, out: out, clock: clock}
}

func (c *CLI) Run(in io.Reader) error {
	scanner := bufio.NewScanner(in)
	c.prompt()
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "exit" || line == "quit" {
			return nil
		}
		if line != "" {
			if err := c.Execute(line); err != nil {
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
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return nil
	}
	cmd, args := strings.ToLower(fields[0]), fields[1:]

	switch cmd {
	case "help":
		fmt.Fprintln(c.out, helpText)
		return nil
	case "add":
		return c.add(args)
	case "list":
		return c.list(args)
	case "show":
		return c.show(args)
	case "start", "done", "cancel", "reopen":
		return c.changeStatus(cmd, args)
	case "delete":
		return c.delete(args)
	case "overdue":
		return c.overdue()
	case "stats":
		return c.stats()
	default:
		return fmt.Errorf("неизвестная команда %q, введите help", cmd)
	}
}

func (c *CLI) add(args []string) error {
	var (
		p          task.Params
		titleWords []string
	)
	for _, arg := range args {
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
	fmt.Fprintf(c.out, "  создана:   %s\n", t.CreatedAt.Format("02.01.2006 15:04"))
	fmt.Fprintf(c.out, "  изменена:  %s\n", t.UpdatedAt.Format("02.01.2006 15:04"))
	return nil
}

func (c *CLI) changeStatus(cmd string, args []string) error {
	id, err := parseID(args)
	if err != nil {
		return err
	}

	var t task.Task
	switch cmd {
	case "start":
		t, err = c.svc.Start(id)
	case "done":
		t, err = c.svc.Complete(id)
	case "cancel":
		t, err = c.svc.Cancel(id)
	case "reopen":
		t, err = c.svc.Reopen(id)
	}
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
		return 0, errors.New("нужно указать один id задачи, например: start 1")
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
	default:
		return err.Error()
	}
}
