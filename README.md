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

