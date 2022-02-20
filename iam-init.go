package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)


const (
	metadataEndpoint    = "http://169.254.169.254"
	credentialsEndpoint = metadataEndpoint + "/latest/meta-data/iam/security-credentials"
	versionString       = "0.0.1"
)

func main() {
	var mainCmd string
	var version bool
	var help bool

	flag.StringVar(&mainCmd, "c", "", "Main command")
	flag.BoolVar(&version, "version", false, "Display kube2iam-init version")
	flag.BoolVar(&help, "help", false, "Help")
	flag.Parse()

	if version {
		fmt.Println(versionString)
		os.Exit(0)
	}

	if help {
		fmt.Printf("usage: %s\n", os.Args[0])
		fmt.Println("  -c [command] Command to run")
		fmt.Println("  -version     Show version")
		fmt.Println("  -help        Show Help")
		os.Exit(0)
	}


	if mainCmd == "" {
		log.Fatal("[iam-init] No command defined, exiting")
	}

	// Routine to reap zombies (it's the job of init)
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)
	go removeZombies(ctx, &wg)

	i := 0
	max := 20
	for i < max {
		client := http.Client{
			Timeout: 2 * time.Second,
		}
		resp, err := client.Get(credentialsEndpoint)
		if err != nil {
			log.Printf("failed to get credentials. Attempt %d of %d\n", i, max)
			time.Sleep(time.Second)
		} else if resp.StatusCode == 200 {
			log.Println("Got successful response from iam instance profile, continuing")
			break
		}
		i++
	}
	if i == max {
		log.Println("[iam-init] failed getting credentials. Exiting...")
		os.Exit(1)
	}

	// Launch main command
	var mainRC int
	log.Printf("[iam-init] command launched : %s\n", mainCmd)
	err := run(mainCmd)
	if err != nil {
		log.Println("[iam-init] command failed")
		log.Printf("[iam-init] %s\n", err)
		mainRC = 1
	} else {
		log.Printf("[iam-init] command exited")
	}

	// Wait removeZombies goroutine
	cleanQuit(cancel, &wg, mainRC)
}

func removeZombies(ctx context.Context, wg *sync.WaitGroup) {
	for {
		var status syscall.WaitStatus

		// Wait for orphaned zombie process
		pid, _ := syscall.Wait4(-1, &status, syscall.WNOHANG, nil)

		if pid <= 0 {
			// PID is 0 or -1 if no child waiting
			// so we wait for 1 second for next check
			time.Sleep(1 * time.Second)
		} else {
			// PID is > 0 if a child was reaped
			// we immediately check if another one
			// is waiting
			continue
		}

		// Non-blocking test
		// if context is done
		select {
		case <-ctx.Done():
			// Context is done
			// so we stop goroutine
			wg.Done()
			return
		default:
		}
	}
}

func run(command string) error {

	var commandStr string
	var argsSlice []string

	// Split cmd and args
	commandSlice := strings.Fields(command)
	commandStr = commandSlice[0]
	// if there is args
	if len(commandSlice) > 1 {
		argsSlice = commandSlice[1:]
	}

	// Register chan to receive system signals
	sigs := make(chan os.Signal, 1)
	defer close(sigs)
	signal.Notify(sigs)
	defer signal.Reset()

	// Define command and rebind
	// stdout and stdin
	cmd := exec.Command(commandStr, argsSlice...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// Goroutine for signals forwarding
	go func() {
		for sig := range sigs {
			// Ignore SIGCHLD signals since
			// these are only usefull for iam-init
			if sig != syscall.SIGCHLD {
				// Forward signal to main process and all children
				syscall.Kill(-cmd.Process.Pid, sig.(syscall.Signal))
			}
		}
	}()

	// Start defined command
	err := cmd.Start()
	if err != nil {
		return err
	}

	// Wait for command to exit
	err = cmd.Wait()
	if err != nil {
		return err
	}

	return nil
}

func cleanQuit(cancel context.CancelFunc, wg *sync.WaitGroup, code int) {
	// Signal zombie goroutine to stop
	// and wait for it to release waitgroup
	cancel()
	wg.Wait()

	os.Exit(code)
}
