package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

var once sync.Once

func CloseOnlyNotified() {
	once.Do(func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt)
		signal.Notify(sigCh, syscall.SIGTERM)

		<-sigCh

		fmt.Printf("app existing...")
	})
}
