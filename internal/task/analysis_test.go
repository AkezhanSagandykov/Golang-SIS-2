package task_test

import (
	"errors"
	"slices"
	"testing"

	"taskflow/internal/task"
)

func TestAnalyze(t *testing.T) {
	tests := []struct {
		name string
		text string
		want task.Result
	}{
		{
			name: "english text",
			text: "Hello Go world",
			want: task.Result{Words: 3, Characters: 14, UniqueWords: 3, Keywords: []string{"hello", "world"}},
		},
		{
			name: "repeated words go first",
			text: "Тесты, тесты и ещё раз тесты! Пул воркеров.",
			want: task.Result{Words: 8, Characters: 43, UniqueWords: 6, Keywords: []string{"тесты", "воркеров"}},
		},
		{
			name: "case does not matter",
			text: "Go go GO gopher",
			want: task.Result{Words: 4, Characters: 15, UniqueWords: 2, Keywords: []string{"gopher"}},
		},
		{
			name: "ties are sorted alphabetically",
			text: "гамма бета альфа дельта",
			want: task.Result{Words: 4, Characters: 23, UniqueWords: 4, Keywords: []string{"альфа", "бета", "гамма"}},
		},
		{
			name: "digits count as words",
			text: "Версия 2 и версия 3",
			want: task.Result{Words: 5, Characters: 19, UniqueWords: 4, Keywords: []string{"версия"}},
		},
		{
			name: "only short words",
			text: "я и ты",
			want: task.Result{Words: 3, Characters: 6, UniqueWords: 3, Keywords: []string{}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := task.Analyze(tt.text)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Words != tt.want.Words || got.Characters != tt.want.Characters || got.UniqueWords != tt.want.UniqueWords {
				t.Errorf("got words=%d chars=%d unique=%d, want %d/%d/%d",
					got.Words, got.Characters, got.UniqueWords, tt.want.Words, tt.want.Characters, tt.want.UniqueWords)
			}
			if !slices.Equal(got.Keywords, tt.want.Keywords) {
				t.Errorf("Keywords = %q, want %q", got.Keywords, tt.want.Keywords)
			}
		})
	}
}

func TestAnalyze_NothingToAnalyze(t *testing.T) {
	for _, text := range []string{"", "   ", "!!! --- ...", "🙂"} {
		if _, err := task.Analyze(text); !errors.Is(err, task.ErrNothingToAnalyze) {
			t.Errorf("Analyze(%q): err = %v, want ErrNothingToAnalyze", text, err)
		}
	}
}

func TestAnalyze_IsDeterministic(t *testing.T) {
	text := "один два три четыре пять шесть семь восемь девять десять"
	first, err := task.Analyze(text)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 50; i++ {
		got, _ := task.Analyze(text)
		if !slices.Equal(got.Keywords, first.Keywords) {
			t.Fatalf("run %d: Keywords = %q, first run gave %q", i, got.Keywords, first.Keywords)
		}
	}
}
