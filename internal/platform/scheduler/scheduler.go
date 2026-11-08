// Package scheduler ejecuta tareas periódicas en segundo plano (goroutines con
// time.Ticker). Se usa para reevaluar la licencia y limpiar sesiones vencidas.
//
// Cada tarea se ejecuta una vez al arrancar y luego cada intervalo. Si una
// ejecución falla solo se registra en el log; la tarea sigue programada.
package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// Task es una tarea periódica.
type Task struct {
	Name      string
	Every     time.Duration
	Run       func(ctx context.Context) error
	Timeout   time.Duration // por defecto 1 minuto
	SkipFirst bool          // true = no ejecutar al arrancar
}

// Scheduler agrupa tareas y las detiene juntas.
type Scheduler struct {
	log   *slog.Logger
	tasks []Task
	wg    sync.WaitGroup
}

// New crea un scheduler vacío.
func New(log *slog.Logger) *Scheduler { return &Scheduler{log: log} }

// Add registra una tarea. Debe llamarse antes de Start.
func (s *Scheduler) Add(t Task) { s.tasks = append(s.tasks, t) }

// Start lanza todas las tareas. Terminan cuando ctx se cancela; use Wait para esperar.
func (s *Scheduler) Start(ctx context.Context) {
	for _, t := range s.tasks {
		s.wg.Add(1)
		go func(t Task) {
			defer s.wg.Done()
			if !t.SkipFirst {
				s.runOnce(ctx, t)
			}
			ticker := time.NewTicker(t.Every)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					s.runOnce(ctx, t)
				}
			}
		}(t)
	}
}

// Wait bloquea hasta que todas las tareas terminen.
func (s *Scheduler) Wait() { s.wg.Wait() }

func (s *Scheduler) runOnce(ctx context.Context, t Task) {
	timeout := t.Timeout
	if timeout == 0 {
		timeout = time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	defer func() {
		if r := recover(); r != nil {
			s.log.Error("tarea con pánico", "task", t.Name, "panic", r)
		}
	}()
	start := time.Now()
	if err := t.Run(ctx); err != nil {
		s.log.Error("tarea falló", "task", t.Name, "err", err)
		return
	}
	s.log.Debug("tarea ejecutada", "task", t.Name, "dur", time.Since(start))
}
