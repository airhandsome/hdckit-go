## hdckit-go: OpenHarmony HDC Go client

Cross-platform Go client and CLI for controlling OpenHarmony devices via hdc.

### Requirements
- hdc binary in PATH or specify via options/flags
- Default server port 8710 (override by `OHOS_HDC_SERVER_PORT`)
- For UiDriver, place `uitestkit_sdk/uitest_agent_v1.1.0.so` in working dir (or parent dir). Go SDK will auto-push/update agent to `/data/local/tmp/agent.so`.
- prepare hdc command line

### Install (library)
```bash
go get github.com/airhandsome/hdckit-go/hdc
```

### Programmatic usage
```go
package main

import (
    "context"
    "fmt"
    hdc "github.com/airhandsome/hdckit-go/hdc"
)

func main() {
    // Create client (auto start hdc server if needed)
    client := hdc.NewClient(hdc.Options{ /* Host:"127.0.0.1", Port:8710, Bin:"hdc" */})

    // List devices
    targets, err := client.ListTargets(context.Background())
    if err != nil { panic(err) }
    if len(targets) == 0 { panic("no devices") }

    // Shell
    t := client.Target(targets[0])
    conn, err := t.Shell(context.Background(), "echo hello")
    if err != nil { panic(err) }
    out, _ := conn.ReadAll(context.Background())
    fmt.Println(string(out))

    // File send/recv
    if err := t.SendFile(context.Background(), "./a.txt", "/data/local/tmp/a.txt"); err != nil { panic(err) }
    if err := t.RecvFile(context.Background(), "/data/local/tmp/a.txt", "./a.txt"); err != nil { panic(err) }

    // Install/Uninstall
    // _ = t.Install(context.Background(), "./app.hap")
    // _ = t.Uninstall(context.Background(), "com.example.app")

    // Forward/Reverse
    if err := t.Forward(context.Background(), "tcp:9000", "tcp:8000"); err != nil { panic(err) }
    _ = t.RemoveForward(context.Background(), "tcp:9000", "tcp:8000")
}
```

### UiDriver usage
```go
// Ensure uitest agent file exists in ./uitestkit_sdk/uitest_agent_v1.1.0.so
drv := client.Target(targets[0]).CreateUiDriver()
// Optionally override SDK path/version
// drv.SetSdk("./uitestkit_sdk/uitest_agent_v1.1.0.so", "1.1.0")
if err := drv.Start(context.Background()); err != nil { panic(err) }
defer drv.Stop()

// Display size
size, _ := drv.GetDisplaySize(context.Background())
fmt.Println("display:", size)

// Capture screen frames (save inside your callback)
_, err := drv.StartCaptureScreen(context.Background(), func(frame []byte) {
    // write frame to file or stream elsewhere
}, 1)
if err != nil { panic(err) }
// ... later
_ = drv.StopCaptureScreen(context.Background())

// Layout & input
layout, _ := drv.CaptureLayout(context.Background())
fmt.Printf("layout: %#v\n", layout)
_ = drv.InputText(context.Background(), "hello", 0, 0)
```

### Command Line (Cobra CLI)
Build:
```bash
go build -o hdccli ./cmd/hdccli
```

Global flags:
- `--host` hdc host (default 127.0.0.1)
- `--port` hdc port (default 8710)
- `--bin`  hdc binary path (default hdc)

Notes:
- If only one device is connected, target can be omitted in all commands.

Common commands:
```bash
# List devices
./hdccli list

# Shell (single device connected)
./hdccli shell "echo hello"
# Shell (with target)
./hdccli shell <target> "echo hello"

# Forward ports
./hdccli forward add tcp:9000 tcp:8000
./hdccli forward list            # all
./hdccli forward list <target>   # for one device
./hdccli forward remove tcp:9000 tcp:8000

# Reverse ports
./hdccli reverse add tcp:8001 tcp:9100
./hdccli reverse list
./hdccli reverse remove tcp:8001 tcp:9100

# Files
./hdccli file send ./a.txt /data/local/tmp/a.txt
./hdccli file recv /data/local/tmp/a.txt ./a.txt

# App install/uninstall
./hdccli install ./app.hap
./hdccli uninstall com.example.app

# Hilog (optionally clear first)
./hdccli hilog --clear

# UiDriver
./hdccli ui size
./hdccli ui input "hello"
./hdccli ui capture --out frames --count 20 --timeout 60
```

Ui capture options:
- `--out` output folder (default `frames`)
- `--count` frames to save (default 10)
- `--timeout` max seconds to wait (default 30)
- Each frame auto-detected as PNG/JPEG; otherwise saved as `.bin`.

### Environment & behavior
- Server auto-start: client attempts `hdc start` once on first connection failure.
- Port selection: explicit `Options.Port` > `OHOS_HDC_SERVER_PORT` > default `8710`.
- UiDriver: enables `persist.ace.testmode`, ensures agent presence/version, starts uitest daemon, forwards tcp:8012.

### Troubleshooting
- No devices: confirm `hdc list targets` in shell returns devices; check USB/IP connection.
- Cannot connect: try `--bin` to point to your hdc; verify firewall/port.
- Ui capture shows few frames: increase `--timeout` and `--count`; low FPS or screen static may limit frames.