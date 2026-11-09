package fieldops

import (
	"testing"

	"github.com/justmamadou/Kodi/internal/platform/db"
)

func ptr[T any](v T) *T { return &v }

func TestResolveProgress(t *testing.T) {
	tests := []struct {
		name       string
		current    db.Task
		in         ProgressInput
		wantStatus string
		wantPct    int16
		wantErr    bool
	}{
		{"démarrer", db.Task{Status: TaskTodo}, ProgressInput{Status: ptr(TaskInProgress)}, TaskInProgress, 0, false},
		{"avancer", db.Task{Status: TaskInProgress, ProgressPct: 10}, ProgressInput{ProgressPct: ptr(int16(40))}, TaskInProgress, 40, false},
		{"terminer force 100", db.Task{Status: TaskInProgress, ProgressPct: 40}, ProgressInput{Status: ptr(TaskDone)}, TaskDone, 100, false},
		{"100 % termine la tâche", db.Task{Status: TaskInProgress, ProgressPct: 40}, ProgressInput{ProgressPct: ptr(int16(100))}, TaskDone, 100, false},
		{"terminée à 50 % refusé", db.Task{Status: TaskInProgress}, ProgressInput{Status: ptr(TaskDone), ProgressPct: ptr(int16(50))}, "", 0, true},
		{"avancement au-dessus de 100", db.Task{Status: TaskInProgress}, ProgressInput{ProgressPct: ptr(int16(101))}, "", 0, true},
		{"avancement négatif", db.Task{Status: TaskInProgress}, ProgressInput{ProgressPct: ptr(int16(-1))}, "", 0, true},
		{"rien à faire", db.Task{Status: TaskTodo}, ProgressInput{}, "", 0, true},
		{"rouvrir une tâche terminée", db.Task{Status: TaskDone, ProgressPct: 100}, ProgressInput{Status: ptr(TaskInProgress)}, TaskInProgress, 99, false},
		{"transition interdite", db.Task{Status: TaskDone, ProgressPct: 100}, ProgressInput{Status: ptr(TaskBlocked)}, "", 0, true},
		{"100 % sur statut non terminé refusé", db.Task{Status: TaskTodo}, ProgressInput{Status: ptr(TaskBlocked), ProgressPct: ptr(int16(100))}, "", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, pct, err := ResolveProgress(tt.current, tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("erreur = %v, attendue = %v", err, tt.wantErr)
			}
			if !tt.wantErr && (status != tt.wantStatus || pct != tt.wantPct) {
				t.Fatalf("(%s, %d), attendu (%s, %d)", status, pct, tt.wantStatus, tt.wantPct)
			}
		})
	}
}
