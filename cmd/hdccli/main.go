package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	hdc "github.com/airhandsome/hdckit-go/hdc"
	"github.com/spf13/cobra"
)

var (
	host  string
	port  int
	bin   string
	debug bool
)

func main() {
	root := &cobra.Command{Use: "hdccli", Short: "OpenHarmony hdc CLI", Example: `
# 列出设备
hdccli list

# 仅一台设备连接时，target 可省略
hdccli shell "echo hello"

# 指定设备执行 shell
hdccli shell <target> "echo hello"

# 端口转发（添加/列表/移除）
hdccli forward add tcp:9000 tcp:8000
hdccli forward list
hdccli forward remove tcp:9000 tcp:8000

# 反向端口转发	hdccli reverse add tcp:8001 tcp:9100
hdccli reverse list

# 文件操作
hdccli file send ./a.txt /data/local/tmp/a.txt
hdccli file recv /data/local/tmp/a.txt ./a.txt

# 安装/卸载
hdccli install ./app.hap
hdccli uninstall com.example.app

# Hilog（清空后查看）
hdccli hilog --clear

# UiDriver 示例
hdccli ui size
hdccli ui capture
hdccli ui input "hello"`}
	root.PersistentFlags().StringVar(&host, "host", "127.0.0.1", "hdc host")
	root.PersistentFlags().IntVar(&port, "port", 8710, "hdc port")
	root.PersistentFlags().StringVar(&bin, "bin", "hdc", "hdc binary path")
	root.PersistentFlags().BoolVar(&debug, "debug", true, "enable debug logs")

	root.AddCommand(cmdList(), cmdTrack(), cmdShell(), cmdForward(), cmdReverse(), cmdFile(), cmdInstall(), cmdUninstall(), cmdHilog(), cmdUi())

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func client() *hdc.Client {
	return hdc.NewClient(hdc.Options{Host: host, Port: port, Bin: bin, Debug: debug})
}

func singleTargetOrErr(ctx context.Context) (string, error) {
	ts, err := client().ListTargets(ctx)
	if err != nil {
		return "", err
	}
	if len(ts) == 1 {
		return ts[0], nil
	}
	return "", errors.New("multiple or zero devices; please specify target explicitly")
}

func cmdList() *cobra.Command {
	return &cobra.Command{Use: "list", Short: "List targets", Example: "hdccli list", RunE: func(cmd *cobra.Command, args []string) error {
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
	return &cobra.Command{Use: "track", Short: "Track targets", Example: "hdccli track", RunE: func(cmd *cobra.Command, args []string) error {
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
	return &cobra.Command{Use: "shell [target] <cmd>", Args: cobra.MinimumNArgs(1), Short: "Run shell on target", Example: "hdccli shell \"echo hello\"\nhdccli shell <target> \"echo hello\"", RunE: func(cmd *cobra.Command, args []string) error {
		var target string
		var command []string
		if len(args) >= 2 {
			target = args[0]
			command = args[1:]
		} else {
			var err error
			target, err = singleTargetOrErr(context.Background())
			if err != nil {
				return err
			}
			command = args[0:]
		}
		t := client().Target(target)
		c, err := t.Shell(context.Background(), join(command))
		if err != nil {
			return err
		}
		out, _ := c.ReadAll(context.Background())
		os.Stdout.Write(out)
		return nil
	}}
}

func cmdForward() *cobra.Command {
	fwd := &cobra.Command{Use: "forward", Short: "Forward port operations", Example: "hdccli forward add tcp:9000 tcp:8000\nhdccli forward list\nhdccli forward remove tcp:9000 tcp:8000"}
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
		if len(args) == 2 {
			tg, err := singleTargetOrErr(context.Background())
			if err != nil {
				return err
			}
			t := client().Target(tg)
			if err := t.Forward(context.Background(), args[0], args[1]); err != nil {
				return err
			}
			fmt.Println("OK")
			return nil
		}
		return cmd.Usage()
	}
	add := &cobra.Command{Use: "add [target] <local> <remote>", Args: cobra.MinimumNArgs(2), Short: "Add forward", Example: "hdccli forward add tcp:9000 tcp:8000", RunE: func(cmd *cobra.Command, args []string) error {
		var target, local, remote string
		if len(args) == 3 {
			target, local, remote = args[0], args[1], args[2]
		} else if len(args) == 2 {
			var err error
			target, err = singleTargetOrErr(context.Background())
			if err != nil {
				return err
			}
			local, remote = args[0], args[1]
		} else {
			return cmd.Usage()
		}
		return client().Target(target).Forward(context.Background(), local, remote)
	}}
	list := &cobra.Command{Use: "list [target]", Args: cobra.RangeArgs(0, 1), Short: "List forwards", Example: "hdccli forward list", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 1 {
			items, err := client().Target(args[0]).ListForwards(context.Background())
			if err != nil {
				return err
			}
			for _, it := range items {
				fmt.Printf("%s %s %s\n", it.Target, it.Local, it.Remote)
			}
			return nil
		}
		ts, err := client().ListTargets(context.Background())
		if err != nil {
			return err
		}
		if len(ts) == 1 {
			items, err := client().Target(ts[0]).ListForwards(context.Background())
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
	remove := &cobra.Command{Use: "remove [target] <local> <remote>", Args: cobra.MinimumNArgs(2), Short: "Remove forward", Example: "hdccli forward remove tcp:9000 tcp:8000", RunE: func(cmd *cobra.Command, args []string) error {
		var target, local, remote string
		if len(args) == 3 {
			target, local, remote = args[0], args[1], args[2]
		} else if len(args) == 2 {
			var err error
			target, err = singleTargetOrErr(context.Background())
			if err != nil {
				return err
			}
			local, remote = args[0], args[1]
		} else {
			return cmd.Usage()
		}
		return client().Target(target).RemoveForward(context.Background(), local, remote)
	}}
	fwd.AddCommand(add, list, remove)
	return fwd
}

func cmdReverse() *cobra.Command {
	rev := &cobra.Command{Use: "reverse", Short: "Reverse port operations", Example: "hdccli reverse add tcp:8001 tcp:9100\nhdccli reverse list\nhdccli reverse remove tcp:8001 tcp:9100"}
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
		if len(args) == 2 {
			tg, err := singleTargetOrErr(context.Background())
			if err != nil {
				return err
			}
			t := client().Target(tg)
			if err := t.Reverse(context.Background(), args[0], args[1]); err != nil {
				return err
			}
			fmt.Println("OK")
			return nil
		}
		return cmd.Usage()
	}
	add := &cobra.Command{Use: "add [target] <remote> <local>", Args: cobra.MinimumNArgs(2), Short: "Add reverse", Example: "hdccli reverse add tcp:8001 tcp:9100", RunE: func(cmd *cobra.Command, args []string) error {
		var target, remote, local string
		if len(args) == 3 {
			target, remote, local = args[0], args[1], args[2]
		} else if len(args) == 2 {
			var err error
			target, err = singleTargetOrErr(context.Background())
			if err != nil {
				return err
			}
			remote, local = args[0], args[1]
		} else {
			return cmd.Usage()
		}
		return client().Target(target).Reverse(context.Background(), remote, local)
	}}
	list := &cobra.Command{Use: "list [target]", Args: cobra.RangeArgs(0, 1), Short: "List reverses", Example: "hdccli reverse list", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 1 {
			items, err := client().Target(args[0]).ListReverses(context.Background())
			if err != nil {
				return err
			}
			for _, it := range items {
				fmt.Printf("%s %s %s\n", it.Target, it.Local, it.Remote)
			}
			return nil
		}
		ts, err := client().ListTargets(context.Background())
		if err != nil {
			return err
		}
		if len(ts) == 1 {
			items, err := client().Target(ts[0]).ListReverses(context.Background())
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
	remove := &cobra.Command{Use: "remove [target] <remote> <local>", Args: cobra.MinimumNArgs(2), Short: "Remove reverse", Example: "hdccli reverse remove tcp:8001 tcp:9100", RunE: func(cmd *cobra.Command, args []string) error {
		var target, remote, local string
		if len(args) == 3 {
			target, remote, local = args[0], args[1], args[2]
		} else if len(args) == 2 {
			var err error
			target, err = singleTargetOrErr(context.Background())
			if err != nil {
				return err
			}
			remote, local = args[0], args[1]
		} else {
			return cmd.Usage()
		}
		return client().Target(target).RemoveReverse(context.Background(), remote, local)
	}}
	rev.AddCommand(add, list, remove)
	return rev
}

func cmdFile() *cobra.Command {
	file := &cobra.Command{Use: "file", Short: "File operations"}
	recv := &cobra.Command{Use: "recv [target] <remote> <local>", Args: cobra.MinimumNArgs(2), Example: "hdccli file recv /data/local/tmp/a.txt ./a.txt", RunE: func(cmd *cobra.Command, args []string) error {
		var target, remote, local string
		if len(args) == 3 {
			target, remote, local = args[0], args[1], args[2]
		} else if len(args) == 2 {
			var err error
			target, err = singleTargetOrErr(context.Background())
			if err != nil {
				return err
			}
			remote, local = args[0], args[1]
		} else {
			return cmd.Usage()
		}
		return client().Target(target).RecvFile(context.Background(), remote, local)
	}}
	send := &cobra.Command{Use: "send [target] <local> <remote>", Args: cobra.MinimumNArgs(2), Example: "hdccli file send ./a.txt /data/local/tmp/a.txt", RunE: func(cmd *cobra.Command, args []string) error {
		var target, local, remote string
		if len(args) == 3 {
			target, local, remote = args[0], args[1], args[2]
		} else if len(args) == 2 {
			var err error
			target, err = singleTargetOrErr(context.Background())
			if err != nil {
				return err
			}
			local, remote = args[0], args[1]
		} else {
			return cmd.Usage()
		}
		return client().Target(target).SendFile(context.Background(), local, remote)
	}}
	file.AddCommand(recv, send)
	return file
}

func cmdInstall() *cobra.Command {
	return &cobra.Command{Use: "install [target] <hap>", Args: cobra.MinimumNArgs(1), Short: "Install hap", Example: "hdccli install ./app.hap", RunE: func(cmd *cobra.Command, args []string) error {
		var target, hap string
		if len(args) == 2 {
			target, hap = args[0], args[1]
		} else if len(args) == 1 {
			var err error
			target, err = singleTargetOrErr(context.Background())
			if err != nil {
				return err
			}
			hap = args[0]
		} else {
			return cmd.Usage()
		}
		return client().Target(target).Install(context.Background(), hap)
	}}
}

func cmdUninstall() *cobra.Command {
	return &cobra.Command{Use: "uninstall [target] <bundle>", Args: cobra.MinimumNArgs(1), Short: "Uninstall bundle", Example: "hdccli uninstall com.example.app", RunE: func(cmd *cobra.Command, args []string) error {
		var target, bundle string
		if len(args) == 2 {
			target, bundle = args[0], args[1]
		} else if len(args) == 1 {
			var err error
			target, err = singleTargetOrErr(context.Background())
			if err != nil {
				return err
			}
			bundle = args[0]
		} else {
			return cmd.Usage()
		}
		return client().Target(target).Uninstall(context.Background(), bundle)
	}}
}

func cmdHilog() *cobra.Command {
	clear := false
	c := &cobra.Command{Use: "hilog [target]", Args: cobra.MinimumNArgs(0), Short: "Open hilog", Example: "hdccli hilog --clear", RunE: func(cmd *cobra.Command, args []string) error {
		var target string
		if len(args) >= 1 {
			target = args[0]
		} else {
			var err error
			target, err = singleTargetOrErr(context.Background())
			if err != nil {
				return err
			}
		}
		h, err := client().Target(target).OpenHilog(context.Background(), clear)
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
	size := &cobra.Command{Use: "size [target]", Args: cobra.MinimumNArgs(0), Example: "hdccli ui size", RunE: func(cmd *cobra.Command, args []string) error {
		var target string
		if len(args) >= 1 {
			target = args[0]
		} else {
			var err error
			target, err = singleTargetOrErr(context.Background())
			if err != nil {
				return err
			}
		}
		drv := client().Target(target).CreateUiDriver()
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
	var outDir string
	var maxCount int
	var timeoutSec int
	capture := &cobra.Command{Use: "capture [target]", Args: cobra.MinimumNArgs(0), Example: "hdccli ui capture --out frames --count 5", RunE: func(cmd *cobra.Command, args []string) error {
		var target string
		if len(args) >= 1 {
			target = args[0]
		} else {
			var err error
			target, err = singleTargetOrErr(context.Background())
			if err != nil {
				return err
			}
		}
		if outDir == "" {
			outDir = "frames"
		}
		if maxCount <= 0 {
			maxCount = 10
		}
		if timeoutSec <= 0 {
			timeoutSec = 30
		}
		if err := os.MkdirAll(outDir, 0o755); err != nil {
			return err
		}
		var saved atomic.Int64
		drv := client().Target(target).CreateUiDriver()
		if err := drv.Start(context.Background()); err != nil {
			return err
		}
		defer drv.Stop()
		_, err := drv.StartCaptureScreen(context.Background(), func(b []byte) {
			n := saved.Add(1)
			if n > int64(maxCount) {
				return
			}
			ext := detectImageExt(b)
			name := fmt.Sprintf("frame_%03d.%s", n, ext)
			_ = os.WriteFile(filepath.Join(outDir, name), b, 0o644)
			fmt.Fprintf(os.Stderr, "saved %s (%d bytes)\n", name, len(b))
		}, 1)
		if err != nil {
			return err
		}
		// wait until reached count or timeout
		deadline := time.Now().Add(time.Duration(timeoutSec) * time.Second)
		for time.Now().Before(deadline) {
			if saved.Load() >= int64(maxCount) {
				break
			}
			time.Sleep(200 * time.Millisecond)
		}
		return drv.StopCaptureScreen(context.Background())
	}}
	capture.Flags().StringVar(&outDir, "out", "frames", "output directory for frames")
	capture.Flags().IntVar(&maxCount, "count", 10, "max frames to save")
	capture.Flags().IntVar(&timeoutSec, "timeout", 30, "max seconds to wait for frames")
	input := &cobra.Command{Use: "input [target] <text>", Args: cobra.MinimumNArgs(1), Example: "hdccli ui input \"hello\"", RunE: func(cmd *cobra.Command, args []string) error {
		var target, text string
		if len(args) == 2 {
			target, text = args[0], args[1]
		} else if len(args) == 1 {
			var err error
			target, err = singleTargetOrErr(context.Background())
			if err != nil {
				return err
			}
			text = args[0]
		} else {
			return cmd.Usage()
		}
		drv := client().Target(target).CreateUiDriver()
		if err := drv.Start(context.Background()); err != nil {
			return err
		}
		defer drv.Stop()
		return drv.InputText(context.Background(), text, 0, 0)
	}}
	ui.AddCommand(size, capture, input)
	return ui
}

func detectImageExt(b []byte) string {
	if len(b) >= 8 && b[0] == 0x89 && b[1] == 0x50 && b[2] == 0x4E && b[3] == 0x47 {
		return "png"
	}
	if len(b) >= 3 && b[0] == 0xFF && b[1] == 0xD8 && b[2] == 0xFF {
		return "jpg"
	}
	return "bin"
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
