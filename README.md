## hdckit-go: OpenHarmony HDC Go client

Cross-platform Go client for controlling OpenHarmony devices via hdc.

### Install
```bash
go get github.com/example/hdckit-go
```

### Quick Start
```go
package main

import (
    "context"
    "fmt"
    hdc "github.com/example/hdckit-go/hdc"
)

func main() {
    client := hdc.NewClient(hdc.Options{})
    targets, err := client.ListTargets(context.Background())
    if err != nil { panic(err) }
    if len(targets) == 0 { panic("no devices") }
    t := client.Target(targets[0])
    conn, err := t.Shell(context.Background(), "echo hello")
    if err != nil { panic(err) }
    out, _ := conn.ReadAll(context.Background())
    fmt.Println(string(out))
}
```

### Track Targets
```go
tr, _ := client.TrackTargets(context.Background())
defer tr.Close()
for {
    select {
    case a := <-tr.Added():
        fmt.Println("online:", a)
    case r := <-tr.Removed():
        fmt.Println("offline:", r)
    case err := <-tr.Errors():
        fmt.Println("error:", err)
    }
}
```

### Hilog
```go
hc, _ := client.Target(targets[0]).OpenHilog(context.Background(), true)
buf, _ := hc.ReadAll(context.Background())
fmt.Print(string(buf))
```

### UiDriver (basic)
```go
drv := client.Target(targets[0]).CreateUiDriver()
if err := drv.Start(context.Background()); err != nil { panic(err) }
size, _ := drv.GetDisplaySize(context.Background())
fmt.Println("display:", size)
_ = drv.InputText(context.Background(), "hello", 0, 0)
layout, _ := drv.CaptureLayout(context.Background())
fmt.Printf("layout: %#v\n", layout)
drv.Stop()
```

### CLI (optional)
Build and run a minimal CLI:
```bash
go build -o hdccli ./cmd/hdccli
./hdccli list
./hdccli shell <target> "echo hello"
./hdccli hilog <target>
./hdccli forward <target> tcp:9000 tcp:8000
./hdccli reverse <target> tcp:8001 tcp:9100
./hdccli install <target> ./app.hap
./hdccli uninstall <target> com.example.app
```

