package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	hdc "github.com/airhandsome/hdckit-go/hdc"
)

func main() {
	host := flag.String("host", "127.0.0.1", "hdc host")
	port := flag.Int("port", 8710, "hdc port")
	bin := flag.String("bin", "hdc", "hdc binary path")
	flag.Parse()
	args := flag.Args()
	if len(args) == 0 {
		usage()
		return
	}
	client := hdc.NewClient(hdc.Options{Host: *host, Port: *port, Bin: *bin})
	ctx := context.Background()
	switch args[0] {
	case "list":
		ts, err := client.ListTargets(ctx)
		if err != nil {
			die(err)
		}
		for _, t := range ts {
			fmt.Println(t)
		}
	case "shell":
		if len(args) < 3 {
			dief("shell <target> <cmd>")
		}
		t := client.Target(args[1])
		c, err := t.Shell(ctx, join(args[2:]))
		if err != nil {
			die(err)
		}
		out, _ := c.ReadAll(ctx)
		os.Stdout.Write(out)
	case "hilog":
		if len(args) < 2 {
			dief("hilog <target>")
		}
		t := client.Target(args[1])
		h, err := t.OpenHilog(ctx, true)
		if err != nil {
			die(err)
		}
		out, _ := h.ReadAll(ctx)
		os.Stdout.Write(out)
	case "forward":
		if len(args) < 4 {
			dief("forward <target> <local> <remote>")
		}
		t := client.Target(args[1])
		if err := t.Forward(ctx, args[2], args[3]); err != nil {
			die(err)
		}
		fmt.Println("OK")
	case "reverse":
		if len(args) < 4 {
			dief("reverse <target> <remote> <local>")
		}
		t := client.Target(args[1])
		if err := t.Reverse(ctx, args[2], args[3]); err != nil {
			die(err)
		}
		fmt.Println("OK")
	case "install":
		if len(args) < 3 {
			dief("install <target> <hap>")
		}
		t := client.Target(args[1])
		if err := t.Install(ctx, args[2]); err != nil {
			die(err)
		}
		fmt.Println("install bundle successfully")
	case "uninstall":
		if len(args) < 3 {
			dief("uninstall <target> <bundle>")
		}
		t := client.Target(args[1])
		if err := t.Uninstall(ctx, args[2]); err != nil {
			die(err)
		}
		fmt.Println("uninstall bundle successfully")
	default:
		usage()
	}
}

func usage() {
	fmt.Println("hdccli usage:")
	fmt.Println("  hdccli [--host ... --port ... --bin ...] <cmd> [args...]")
	fmt.Println("  cmds: list | shell <target> <cmd> | hilog <target> | forward <target> <local> <remote> | reverse <target> <remote> <local> | install <target> <hap> | uninstall <target> <bundle>")
}

func die(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
func dief(s string) { fmt.Fprintln(os.Stderr, s); os.Exit(2) }
func join(ss []string) string {
	if len(ss) == 0 {
		return ""
	}
	out := ss[0]
	for i := 1; i < len(ss); i++ {
		out += " " + ss[i]
	}
	return out
}
