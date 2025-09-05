package main

import (
	"context"
	"fmt"
	"os"
	"time"

	hdc "github.com/airhandsome/hdckit-go/hdc"
	"github.com/spf13/cobra"
)

var (
	host string
	port int
	bin  string
)

func main() {
	root := &cobra.Command{Use: "hdccli", Short: "OpenHarmony hdc CLI"}
	root.PersistentFlags().StringVar(&host, "host", "127.0.0.1", "hdc host")
	root.PersistentFlags().IntVar(&port, "port", 8710, "hdc port")
	root.PersistentFlags().StringVar(&bin, "bin", "hdc", "hdc binary path")

	root.AddCommand(cmdList(), cmdTrack(), cmdShell(), cmdForward(), cmdReverse(), cmdFile(), cmdInstall(), cmdUninstall(), cmdHilog(), cmdUi())

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func client() *hdc.Client { return hdc.NewClient(hdc.Options{Host: host, Port: port, Bin: bin}) }

func cmdList() *cobra.Command {
	return &cobra.Command{Use: "list", Short: "List targets", RunE: func(cmd *cobra.Command, args []string) error {
		ts, err := client().ListTargets(context.Background())
		if err != nil {
			return err
		}
		for _, t := range ts {
			fmt.Println(t)
		}
		return nil
	}}
}

func cmdTrack() *cobra.Command {
	return &cobra.Command{Use: "track", Short: "Track targets", RunE: func(cmd *cobra.Command, args []string) error {
		c := client()
		tr, err := c.TrackTargets(context.Background())
		if err != nil {
			return err
		}
		defer tr.Close()
		for {
			select {
			case a := <-tr.Added():
				fmt.Println("add:", a)
			case r := <-tr.Removed():
				fmt.Println("remove:", r)
			case e := <-tr.Errors():
				fmt.Fprintln(os.Stderr, "error:", e)
			}
		}
	}}
}

func cmdShell() *cobra.Command {
	return &cobra.Command{Use: "shell <target> <cmd>", Args: cobra.MinimumNArgs(2), Short: "Run shell on target", RunE: func(cmd *cobra.Command, args []string) error {
		t := client().Target(args[0])
		c, err := t.Shell(context.Background(), join(args[1:]))
		if err != nil {
			return err
		}
		out, _ := c.ReadAll(context.Background())
		os.Stdout.Write(out)
		return nil
	}}
}

func cmdForward() *cobra.Command {
	fwd := &cobra.Command{Use: "forward", Short: "Forward port operations"}
	// backward compatible: forward <target> <local> <remote>
	fwd.RunE = func(cmd *cobra.Command, args []string) error {
		if len(args) == 3 {
			t := client().Target(args[0])
			if err := t.Forward(context.Background(), args[1], args[2]); err != nil {
				return err
			}
			fmt.Println("OK")
			return nil
		}
		return cmd.Usage()
	}
	add := &cobra.Command{Use: "add <target> <local> <remote>", Args: cobra.ExactArgs(3), Short: "Add forward", RunE: func(cmd *cobra.Command, args []string) error {
		t := client().Target(args[0])
		if err := t.Forward(context.Background(), args[1], args[2]); err != nil {
			return err
		}
		fmt.Println("OK")
		return nil
	}}
	list := &cobra.Command{Use: "list [target]", Args: cobra.RangeArgs(0, 1), Short: "List forwards", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 1 {
			t := client().Target(args[0])
			items, err := t.ListForwards(context.Background())
			if err != nil {
				return err
			}
			for _, it := range items {
				fmt.Printf("%s %s %s\n", it.Target, it.Local, it.Remote)
			}
			return nil
		}
		items, err := client().ListForwards(context.Background())
		if err != nil {
			return err
		}
		for _, it := range items {
			fmt.Printf("%s %s %s\n", it.Target, it.Local, it.Remote)
		}
		return nil
	}}
	remove := &cobra.Command{Use: "remove <target> <local> <remote>", Args: cobra.ExactArgs(3), Short: "Remove forward", RunE: func(cmd *cobra.Command, args []string) error {
		t := client().Target(args[0])
		if err := t.RemoveForward(context.Background(), args[1], args[2]); err != nil {
			return err
		}
		fmt.Println("OK")
		return nil
	}}
	fwd.AddCommand(add, list, remove)
	return fwd
}

func cmdReverse() *cobra.Command {
	rev := &cobra.Command{Use: "reverse", Short: "Reverse port operations"}
	// backward compatible: reverse <target> <remote> <local>
	rev.RunE = func(cmd *cobra.Command, args []string) error {
		if len(args) == 3 {
			t := client().Target(args[0])
			if err := t.Reverse(context.Background(), args[1], args[2]); err != nil {
				return err
			}
			fmt.Println("OK")
			return nil
		}
		return cmd.Usage()
	}
	add := &cobra.Command{Use: "add <target> <remote> <local>", Args: cobra.ExactArgs(3), Short: "Add reverse", RunE: func(cmd *cobra.Command, args []string) error {
		t := client().Target(args[0])
		if err := t.Reverse(context.Background(), args[1], args[2]); err != nil {
			return err
		}
		fmt.Println("OK")
		return nil
	}}
	list := &cobra.Command{Use: "list [target]", Args: cobra.RangeArgs(0, 1), Short: "List reverses", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 1 {
			t := client().Target(args[0])
			items, err := t.ListReverses(context.Background())
			if err != nil {
				return err
			}
			for _, it := range items {
				fmt.Printf("%s %s %s\n", it.Target, it.Local, it.Remote)
			}
			return nil
		}
		items, err := client().ListReverses(context.Background())
		if err != nil {
			return err
		}
		for _, it := range items {
			fmt.Printf("%s %s %s\n", it.Target, it.Local, it.Remote)
		}
		return nil
	}}
	remove := &cobra.Command{Use: "remove <target> <remote> <local>", Args: cobra.ExactArgs(3), Short: "Remove reverse", RunE: func(cmd *cobra.Command, args []string) error {
		t := client().Target(args[0])
		if err := t.RemoveReverse(context.Background(), args[1], args[2]); err != nil {
			return err
		}
		fmt.Println("OK")
		return nil
	}}
	rev.AddCommand(add, list, remove)
	return rev
}

func cmdFile() *cobra.Command {
	file := &cobra.Command{Use: "file", Short: "File operations"}
	recv := &cobra.Command{Use: "recv <target> <remote> <local>", Args: cobra.ExactArgs(3), RunE: func(cmd *cobra.Command, args []string) error {
		t := client().Target(args[0])
		return t.RecvFile(context.Background(), args[1], args[2])
	}}
	send := &cobra.Command{Use: "send <target> <local> <remote>", Args: cobra.ExactArgs(3), RunE: func(cmd *cobra.Command, args []string) error {
		t := client().Target(args[0])
		return t.SendFile(context.Background(), args[1], args[2])
	}}
	file.AddCommand(recv, send)
	return file
}

func cmdInstall() *cobra.Command {
	return &cobra.Command{Use: "install <target> <hap>", Args: cobra.ExactArgs(2), Short: "Install hap", RunE: func(cmd *cobra.Command, args []string) error {
		return client().Target(args[0]).Install(context.Background(), args[1])
	}}
}

func cmdUninstall() *cobra.Command {
	return &cobra.Command{Use: "uninstall <target> <bundle>", Args: cobra.ExactArgs(2), Short: "Uninstall bundle", RunE: func(cmd *cobra.Command, args []string) error {
		return client().Target(args[0]).Uninstall(context.Background(), args[1])
	}}
}

func cmdHilog() *cobra.Command {
	clear := false
	c := &cobra.Command{Use: "hilog <target>", Args: cobra.ExactArgs(1), Short: "Open hilog", RunE: func(cmd *cobra.Command, args []string) error {
		h, err := client().Target(args[0]).OpenHilog(context.Background(), clear)
		if err != nil {
			return err
		}
		out, _ := h.ReadAll(context.Background())
		os.Stdout.Write(out)
		return nil
	}}
	c.Flags().BoolVar(&clear, "clear", false, "clear logs first")
	return c
}

func cmdUi() *cobra.Command {
	ui := &cobra.Command{Use: "ui", Short: "UiDriver operations"}
	size := &cobra.Command{Use: "size <target>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		drv := client().Target(args[0]).CreateUiDriver()
		if err := drv.Start(context.Background()); err != nil {
			return err
		}
		defer drv.Stop()
		m, err := drv.GetDisplaySize(context.Background())
		if err != nil {
			return err
		}
		fmt.Println(m)
		return nil
	}}
	capture := &cobra.Command{Use: "capture <target>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		drv := client().Target(args[0]).CreateUiDriver()
		if err := drv.Start(context.Background()); err != nil {
			return err
		}
		defer drv.Stop()
		_, err := drv.StartCaptureScreen(context.Background(), func(b []byte) {
			// naive throttle
			fmt.Fprintf(os.Stderr, "frame %d bytes\n", len(b))
		}, 1)
		if err != nil {
			return err
		}
		time.Sleep(3 * time.Second)
		return drv.StopCaptureScreen(context.Background())
	}}
	input := &cobra.Command{Use: "input <target> <text>", Args: cobra.ExactArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		drv := client().Target(args[0]).CreateUiDriver()
		if err := drv.Start(context.Background()); err != nil {
			return err
		}
		defer drv.Stop()
		return drv.InputText(context.Background(), args[1], 0, 0)
	}}
	ui.AddCommand(size, capture, input)
	return ui
}

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
