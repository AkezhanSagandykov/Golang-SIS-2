package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"taskflow/internal/cli"
	"taskflow/internal/service"
	"taskflow/internal/storage"
	"taskflow/internal/task"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "ошибка:", err)
		os.Exit(1)
	}
}

func run() error {
	demo := flag.Bool("demo", false, "добавить несколько задач для демонстрации")
	workers := flag.Int("workers", service.DefaultWorkers, "количество воркеров (не меньше 2)")
	queueSize := flag.Int("queue", service.DefaultQueueSize, "вместимость очереди")
	flag.Parse()

	clock := service.SystemClock{}
	svc, err := service.New(storage.NewMemoryStore(), clock, service.DescriptionAnalyzer{}, service.Config{
		Workers:   *workers,
		QueueSize: *queueSize,
	})
	if err != nil {
		return err
	}
	defer func() {
		fmt.Println("Останавливаю воркеры и жду задачи из очереди...")
		svc.Shutdown()
		fmt.Println("Все воркеры завершились.")
	}()

	if *demo {
		if err := seed(svc, clock.Now()); err != nil {
			return fmt.Errorf("демо-задачи: %w", err)
		}
	}

	fmt.Printf("TaskFlow — трекер задач. Воркеров: %d, очередь: %d. Введите help, чтобы увидеть команды.\n", *workers, *queueSize)
	return cli.New(svc, os.Stdout, clock).Run(os.Stdin)
}

func seed(svc *service.Service, now time.Time) error {
	params := []task.Params{
		{
			Title: "Подготовиться к защите Checkpoint 2", Priority: task.PriorityHigh,
			Tags: []string{"go", "study"}, Deadline: now.AddDate(0, 0, 3),
			Description: "Повторить, как работает пул воркеров: очередь, воркеры, мьютекс и WaitGroup. " +
				"Повторить, что происходит, когда очередь заполнена, и как воркеры останавливаются.",
		},
		{
			Title: "Написать тесты для пула", Priority: task.PriorityHigh, Tags: []string{"go"},
			Description: "Тесты проверяют, что воркеры обрабатывают задачи одновременно, " +
				"что полная очередь отклоняет задачи и что после остановки новые задачи не принимаются.",
		},
		{
			Title: "Прочитать про горутины", Priority: task.PriorityLow, Tags: []string{"go", "reading"},
			Description: "Горутины, планировщик Go, каналы, буферизованные каналы и утечки горутин.",
		},
		{
			Title: "Позвонить в деканат", Priority: task.PriorityMedium,
		},
		{
			Title: "Купить продукты", Priority: task.PriorityLow,
			Description: "Молоко, хлеб, яблоки и сыр.",
		},
	}

	for _, p := range params {
		if _, err := svc.Create(p); err != nil {
			return err
		}
	}
	if _, err := svc.Cancel(5); err != nil {
		return err
	}
	return nil
}
