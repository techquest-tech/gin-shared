package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

func CloseOnlyNotified() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	signal.Notify(sigCh, syscall.SIGTERM)

	c := <-sigCh

	fmt.Printf("Got Interrupt(%s), app existing...", c.String())
}
