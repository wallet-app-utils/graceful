package graceful

import (
	"context"
	"log"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

type CloseGroup struct {
	wg      sync.WaitGroup
	errors  chan error
	onError func(error)
}

var DefaultOnError = func(err error) {
	if err != nil {
		log.Printf("graceful error: %v\n", err)
	}
}

func Prepare(ctx context.Context) (shutdownCtx context.Context, group *CloseGroup) {
	group = &CloseGroup{
		errors:  make(chan error),
		onError: DefaultOnError,
	}

	// Ловим сигналы ОС
	sigCtx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)

	shutdownCtx, cancel := context.WithCancel(sigCtx)

	// ловим ошибки и отменяем всё
	go func() {
		for err := range group.errors {
			group.onError(err)
			cancel()
		}
	}()

	// Когда shutdownCtx закрывается, выключаем сигнал-нотификатор
	go func() {
		<-shutdownCtx.Done()
		stop()
	}()

	return shutdownCtx, group
}

// Process запускает процесс (например сервер или воркер)
func Process[T any](group *CloseGroup, target T, fn func(T) error) {
	group.wg.Add(1)
	go func() {
		defer group.wg.Done()
		if err := fn(target); err != nil {
			group.errors <- err
		}
	}()
}

// Close регистрирует функцию закрытия ресурса
func Close[T any](group *CloseGroup, target T, fn func(T) error) {
	group.wg.Add(1)
	go func() {
		defer group.wg.Done()
		if err := fn(target); err != nil {
			group.errors <- err
		}
	}()
}

// Wait ждёт завершения всех процессов с таймаутом
func (g *CloseGroup) Wait(timeout time.Duration) {
	done := make(chan struct{})
	go func() {
		g.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Println("All processes closed gracefully.")
	case <-time.After(timeout):
		log.Println("Timeout reached, some processes may not be closed.")
	}
	close(g.errors)
}
