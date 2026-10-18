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
	demo := flag.Bool("demo", false, "добавить несколько задач для демонстрации")
	flag.Parse()

	clock := service.SystemClock{}
	store := storage.NewMemoryStore()
	svc := service.New(store, clock)
	app := cli.New(svc, os.Stdout, clock)

	if *demo {
		if err := seed(svc, clock.Now()); err != nil {
			fmt.Fprintln(os.Stderr, "не удалось добавить демо-задачи:", err)
			os.Exit(1)
		}
	}

	fmt.Println("TaskFlow — трекер задач. Введите help, чтобы увидеть команды.")
	if err := app.Run(os.Stdin); err != nil {
		fmt.Fprintln(os.Stderr, "ошибка чтения ввода:", err)
		os.Exit(1)
	}
}

func seed(svc *service.Service, now time.Time) error {
	inThreeDays := now.AddDate(0, 0, 3)

	defense, err := svc.Create(task.Params{
		Title: "Подготовиться к защите Checkpoint 1", Priority: task.PriorityHigh,
		Tags: []string{"go", "study"}, Deadline: inThreeDays,
	})
	if err != nil {
		return err
	}
	tests, err := svc.Create(task.Params{Title: "Написать unit-тесты", Priority: task.PriorityHigh, Tags: []string{"go"}})
	if err != nil {
		return err
	}
	if _, err := svc.Create(task.Params{Title: "Прочитать про горутины", Priority: task.PriorityLow, Tags: []string{"go", "reading"}}); err != nil {
		return err
	}
	groceries, err := svc.Create(task.Params{Title: "Купить продукты", Priority: task.PriorityLow})
	if err != nil {
		return err
	}

	if _, err := svc.Start(defense.ID); err != nil {
		return err
	}
	if _, err := svc.Start(tests.ID); err != nil {
		return err
	}
	if _, err := svc.Complete(tests.ID); err != nil {
		return err
	}
	if _, err := svc.Cancel(groceries.ID); err != nil {
		return err
	}
	return nil
}
