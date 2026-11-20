// taskdeck_remote_probe is a developer-only presence tester.
// It DOES NOT establish a remote TaskDeck session or bypass local authentication.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"bletonfc/vscode_tasks_menu/internal/remoteportal"
)

func main() {
	action := flag.String("action", "", "enroll or heartbeat")
	portal := flag.String("portal", "", "HTTPS Portal origin, e.g. https://remote.example.com")
	identity := flag.String("identity", "", "ABSOLUTE path to private device identity (per TaskDeck instance)")
	name := flag.String("name", "", "Display name of this TaskDeck instance (enroll only)")
	flag.Parse()

	if *identity == "" || *portal == "" || (*action != "enroll" && *action != "heartbeat") {
		flag.Usage()
		os.Exit(2)
	}
	agent, err := remoteportal.NewAgent(*portal, *identity, nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, "configuration error:", err)
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if *action == "enroll" {
		code, err := agent.Enroll(ctx, *name)
		if err != nil {
			fmt.Fprintln(os.Stderr, "enrollment failed:", err)
			os.Exit(1)
		}
		fmt.Println("Device ID:", agent.DeviceID())
		fmt.Println("One-time pairing code (keep private; expires in 5 minutes):", code)
		fmt.Println("Enter the Device ID and pairing code on the Portal while locally approving this instance.")
		return
	}
	if err := agent.Heartbeat(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "heartbeat failed:", err)
		os.Exit(1)
	}
	fmt.Println("Signed heartbeat accepted for Device ID:", agent.DeviceID())
}
