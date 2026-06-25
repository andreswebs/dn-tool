# `urfave/cli` v3 — Reference

Reference for the `github.com/urfave/cli/v3` library. Describes types, fields, function signatures, and call patterns. For an introduction or step-by-step walkthrough, see the upstream docs at <https://cli.urfave.org/v3/>.

## Module

| Item        | Value                                    |
| ----------- | ---------------------------------------- |
| Import path | `github.com/urfave/cli/v3`               |
| Package     | `cli`                                    |
| Install     | `go get github.com/urfave/cli/v3@latest` |
| Stdlib only | yes (no external dependencies)           |


``` go
import "github.com/urfave/cli/v3"
```

## Conceptual model

- A program is a single `*cli.Command`. There is no separate `App` type.
- Subcommands are `*cli.Command` values nested in the parent's `Commands` field.
- The handler signature is `func(ctx context.Context, cmd *cli.Command) error`. Flag and argument values are read from the `*cli.Command`, not from a `cli.Context` (which does not exist in v3).
- `cmd.Run(ctx, os.Args)` parses arguments and dispatches. It does **not** call `os.Exit`; the caller decides what to do with the returned error.

## `cli.Command`

The root program and every subcommand are both `*cli.Command`.

### Identity and help text

| Field         | Type       | Purpose                                                    |
| ------------- | ---------- | ---------------------------------------------------------- |
| `Name`        | `string`   | Command name as invoked.                                   |
| `Aliases`     | `[]string` | Alternative names.                                         |
| `Usage`       | `string`   | One-line summary used in help.                             |
| `UsageText`   | `string`   | Override of the auto-generated usage line.                 |
| `ArgsUsage`   | `string`   | Override of the auto-generated args portion of usage.      |
| `Description` | `string`   | Long-form description shown under "Description:".          |
| `Category`    | `string`   | Grouping label in parent command's help output.            |
| `Version`     | `string`   | Version string; enables `--version`/`-v` when set on root. |
| `Copyright`   | `string`   | Footer text in help output.                                |
| `Authors`     | `[]any`    | Typically `mail.Address` values from `net/mail`.           |
| `Hidden`      | `bool`     | Omit from help output.                                     |


### Structure

| Field                    | Type                       | Purpose                                           |
| ------------------------ | -------------------------- | ------------------------------------------------- |
| `Commands`               | `[]*Command`               | Subcommands.                                      |
| `Flags`                  | `[]Flag`                   | Flag definitions for this command.                |
| `Arguments`              | `[]Argument`               | Typed positional arguments for this command.      |
| `DefaultCommand`         | `string`                   | Name of the subcommand to run when none is given. |
| `MutuallyExclusiveFlags` | `[]MutuallyExclusiveFlags` | Flag groups where at most one may be set.         |


### Lifecycle hooks

| Field                      | Signature                                                  |
| -------------------------- | ---------------------------------------------------------- |
| `Action`                   | `func(context.Context, *Command) error`                    |
| `Before`                   | `func(context.Context, *Command) (context.Context, error)` |
| `After`                    | `func(context.Context, *Command) error`                    |
| `CommandNotFound`          | `func(context.Context, *Command, string) error`            |
| `OnUsageError`             | `func(context.Context, *Command, error, bool) error`       |
| `ExitErrHandler`           | `func(context.Context, error)`                             |
| `InvalidFlagAccessHandler` | `func(*Command, string) error`                             |


`Before` may return a derived `context.Context`; the new context is passed to `Action`. `After` runs whether or not `Action` returned an error.

### I/O

| Field       | Type        | Default     |
| ----------- | ----------- | ----------- |
| `Reader`    | `io.Reader` | `os.Stdin`  |
| `Writer`    | `io.Writer` | `os.Stdout` |
| `ErrWriter` | `io.Writer` | `os.Stderr` |


### Parsing options

| Field                       | Type     | Effect                                                 |
| --------------------------- | -------- | ------------------------------------------------------ |
| `UseShortOptionHandling`    | `bool`   | Allow combined short flags, e.g. `-abc` as `-a -b -c`. |
| `SkipFlagParsing`           | `bool`   | Pass all args through as positional.                   |
| `AllowExtFlags`             | `bool`   | Accept unknown flags as args instead of erroring.      |
| `PrefixMatchCommands`       | `bool`   | Match a subcommand by unique prefix.                   |
| `Suggest`                   | `bool`   | "Did you mean ...?" for misspelled subcommands.        |
| `SliceFlagSeparator`        | `string` | Separator for slice flag values (default `,`).         |
| `DisableSliceFlagSeparator` | `bool`   | Disable separator splitting entirely.                  |
| `MapFlagKeyValueSeparator`  | `string` | Separator for `key=value` map flags (default `=`).     |
| `ReadArgsFromStdin`         | `bool`   | Read additional args from stdin.                       |
| `StopOnNthArg`              | `*int`   | Stop flag parsing after N positional args.             |


### Help and completion

| Field                             | Type                              | Effect                                                    |
| --------------------------------- | --------------------------------- | --------------------------------------------------------- |
| `HideHelp`                        | `bool`                            | Suppress `--help` flag.                                   |
| `HideHelpCommand`                 | `bool`                            | Suppress the auto `help` subcommand.                      |
| `HideVersion`                     | `bool`                            | Suppress `--version` flag.                                |
| `CustomHelpTemplate`              | `string`                          | Per-command help template override.                       |
| `CustomRootCommandHelpTemplate`   | `string`                          | Help template for the root command.                       |
| `EnableShellCompletion`           | `bool`                            | Enable auto-completion subcommand.                        |
| `ShellCompletionCommandName`      | `string`                          | Name of the completion subcommand (default `completion`). |
| `ShellComplete`                   | `ShellCompleteFunc`               | Custom completer for this command.                        |
| `ConfigureShellCompletionCommand` | `ConfigureShellCompletionCommand` | Customize the generated completion command.               |


### Methods on `*Command`

#### Entry

``` go
func (cmd *Command) Run(ctx context.Context, osArgs []string) error
```

#### Flag accessors

| Method                     | Returns                            |
| -------------------------- | ---------------------------------- |
| `String(name string)`      | `string`                           |
| `Bool(name string)`        | `bool`                             |
| `Int(name string)`         | `int`                              |
| `Float(name string)`       | `float64`                          |
| `Duration(name string)`    | `time.Duration`                    |
| `Timestamp(name string)`   | `time.Time`                        |
| `StringSlice(name string)` | `[]string`                         |
| `IntSlice(name string)`    | `[]int`                            |
| `FloatSlice(name string)`  | `[]float64`                        |
| `StringMap(name string)`   | `map[string]string`                |
| `Value(name string)`       | `any`                              |
| `IsSet(name string)`       | `bool`                             |
| `Count(name string)`       | `int` (repeats of a counting flag) |


#### Argument accessors

| Method                      | Returns     |
| --------------------------- | ----------- |
| `StringArg(name string)`    | `string`    |
| `IntArg(name string)`       | `int`       |
| `FloatArg(name string)`     | `float64`   |
| `TimestampArg(name string)` | `time.Time` |
| `StringArgs(name string)`   | `[]string`  |
| `IntArgs(name string)`      | `[]int`     |
| `FloatArgs(name string)`    | `[]float64` |


(Plus `Int8Arg`/`Int16Arg`/`Int32Arg`/`Int64Arg`, `UintArg`/`Uint8Arg`/... and their plural forms.)

#### Untyped argument access

``` go
func (cmd *Command) Args() Args
func (cmd *Command) NArg() int

type Args interface {
    Get(n int) string
    First() string
    Tail() []string
    Len() int
    Present() bool
    Slice() []string
}
```

#### Navigation

| Method                    | Returns      | Purpose                                          |
| ------------------------- | ------------ | ------------------------------------------------ |
| `Command(name string)`    | `*Command`   | Look up a subcommand by name.                    |
| `Root()`                  | `*Command`   | Root of the command tree.                        |
| `Lineage()`               | `[]*Command` | Path from current command to root.               |
| `FullName()`              | `string`     | Dotted invocation path, e.g. `app tbx validate`. |
| `HasName(name string)`    | `bool`       | True if `name` is the command's name or alias.   |
| `FlagNames()`             | `[]string`   | Names of all flags visible to this command.      |
| `LocalFlagNames()`        | `[]string`   | Names of flags defined on this command only.     |
| `NumFlags()`              | `int`        |                                                  |
| `Set(name, value string)` | `error`      | Programmatically set a flag value.               |


## Flags

All flag types are aliases of the generic `FlagBase[T, C, VC]`.

``` go
type FlagBase[T any, C any, VC ValueCreator[T, C]] struct {
    Name             string
    Aliases          []string
    Usage            string
    Category         string
    Value            T          // default value
    Destination      *T         // optional bind target
    Required         bool
    Hidden           bool
    Local            bool       // not inherited by subcommands
    OnlyOnce         bool
    Sources          ValueSourceChain
    TakesFile        bool
    DefaultText      string
    HideDefault      bool
    Action           func(context.Context, *Command, T) error
    Validator        func(T) error
    ValidateDefaults bool
    Config           C
}
```

### Concrete flag types

| Type                         | Value type          | Config type                    |
| ---------------------------- | ------------------- | ------------------------------ |
| `StringFlag`                 | `string`            | `StringConfig`                 |
| `BoolFlag`                   | `bool`              | `BoolConfig`                   |
| `IntFlag`                    | `int`               | `IntegerConfig`                |
| `Int8Flag`..`Int64Flag`      | sized int           | `IntegerConfig`                |
| `UintFlag`..`Uint64Flag`     | sized uint          | `IntegerConfig`                |
| `FloatFlag`                  | `float64`           | `NoConfig`                     |
| `Float32Flag`, `Float64Flag` | float               | `NoConfig`                     |
| `DurationFlag`               | `time.Duration`     | `NoConfig`                     |
| `TimestampFlag`              | `time.Time`         | `TimestampConfig`              |
| `StringSliceFlag`            | `[]string`          | `StringConfig`                 |
| `IntSliceFlag`               | `[]int`             | `IntegerConfig`                |
| `FloatSliceFlag`             | `[]float64`         | `NoConfig`                     |
| `StringMapFlag`              | `map[string]string` | `StringConfig`                 |
| `GenericFlag`                | `Value`             | `NoConfig`                     |
| `BoolWithInverseFlag`        | `bool`              | generates paired `--no-<name>` |


### Selected config types

``` go
type BoolConfig struct {
    Count *int // *int receives the number of repeats (-vvv -> 3)
}

type TimestampConfig struct {
    Layout   string             // single layout (back-compat)
    Layouts  []string           // multiple accepted layouts
    Timezone *time.Location
}
```

### Field semantics

| Field         | Behavior                                                            |
| ------------- | ------------------------------------------------------------------- |
| `Value`       | Default when the flag is not set and no `Sources` resolve.          |
| `Destination` | Pointer that receives the parsed value.                             |
| `Required`    | Error if the flag is not set.                                       |
| `Local`       | `true` means the flag is not inherited by subcommands.              |
| `OnlyOnce`    | Error if the flag is supplied more than once.                       |
| `Sources`     | Ordered chain of external sources (env, file). First resolved wins. |
| `Action`      | Per-flag callback run after parsing if the flag was set.            |
| `Validator`   | Pure validation function on the parsed value.                       |
| `TakesFile`   | Hint for shell completion that the value names a file path.         |


### Help placeholders

Backticks in `Usage` mark the placeholder token shown in help output:

``` go
&cli.StringFlag{
    Name:  "config",
    Aliases: []string{"c"},
    Usage: "load configuration from `FILE`",
}
// Help: --config FILE, -c FILE   load configuration from FILE
```

## Value sources

``` go
type ValueSource interface {
    Lookup() (string, bool)
}
type ValueSourceChain struct { /* opaque */ }

func EnvVar(key string) ValueSource
func File(path string) ValueSource
func EnvVars(keys ...string) ValueSourceChain
func Files(paths ...string) ValueSourceChain
func NewValueSourceChain(src ...ValueSource) ValueSourceChain
```

Resolution order is the order sources appear in the chain; the first successful `Lookup()` wins. The command-line value, when present, overrides all sources.

JSON / YAML / TOML alternate sources live in `github.com/urfave/cli-altsrc/v3` (separate module).

## Arguments

``` go
type Argument interface {
    HasName(string) bool
    Parse([]string) ([]string, error)
    Usage() string
    Get() any
}

type ArgumentBase[T, C, VC] struct {
    Name        string
    Value       T
    Destination *T
    UsageText   string
    Config      C
}

type ArgumentsBase[T, C, VC] struct {
    Name        string
    Value       T
    Destination *[]T
    UsageText   string
    Min         int  // minimum count required
    Max         int  // -1 means unlimited
    Config      C
}
```

Concrete singular and plural aliases:

| Singular               | Plural          | Value type  |
| ---------------------- | --------------- | ----------- |
| `StringArg`            | `StringArgs`    | `string`    |
| `IntArg`               | `IntArgs`       | `int`       |
| `Int8Arg`..`Int64Arg`  | (plurals)       | sized int   |
| `UintArg`..`Uint64Arg` | (plurals)       | sized uint  |
| `FloatArg`             | `FloatArgs`     | `float64`   |
| `TimestampArg`         | `TimestampArgs` | `time.Time` |


Constraints:

- `Max` must be non-zero and strictly greater than `Min`.
- A glob argument (`Max: -1`) is valid only as the last entry in `Arguments`.

A predefined slice that consumes everything:

``` go
var AnyArguments = []cli.Argument{ &cli.StringArgs{Max: -1} }
```

## Callback function types

``` go
type ActionFunc            func(context.Context, *Command) error
type BeforeFunc            func(context.Context, *Command) (context.Context, error)
type AfterFunc             func(context.Context, *Command) error
type CommandNotFoundFunc   func(context.Context, *Command, string) error
type OnUsageErrorFunc      func(context.Context, *Command, error, bool) error
type InvalidFlagAccessFunc func(*Command, string) error
type ShellCompleteFunc     func(context.Context, *Command)
type ExitErrHandlerFunc    func(context.Context, error)
type SuggestCommandFunc    func([]string, string) string
```

## Exit codes and errors

`Command.Run` returns an `error`. The process exit code is taken from that error only when it implements `ExitCoder`.

``` go
type ExitCoder interface {
    Exit() string
    ExitCode() int
}

func Exit(message any, exitCode int) ExitCoder
func HandleExitCoder(err error)

type MultiError interface {
    Errors() []error
}
```

Package-level overrides:

``` go
var OsExiter  func(int)    = os.Exit
var ErrWriter io.Writer    = os.Stderr
```

## Help and version

Package-level templates and printers that can be overridden:

``` go
var RootCommandHelpTemplate string
var CommandHelpTemplate     string
var SubcommandHelpTemplate  string
var HelpFlag                cli.Flag           // default --help, -h
var VersionFlag             cli.Flag           // default --version, -v
var HelpPrinter             func(w io.Writer, templ string, data any)
var VersionPrinter          HelpPrinterFunc
var FlagStringer            func(cli.Flag) string
```

Helpers:

``` go
func ShowRootCommandHelpAndExit(cmd *Command, exitCode int)
func ShowSubcommandHelpAndExit(cmd *Command, exitCode int)
func ShowCommandHelpAndExit(ctx context.Context, cmd *Command, command string, code int)
func ShowVersion(cmd *Command)
```

## Shell completion

Setting `EnableShellCompletion: true` on the root command exposes a hidden `completion <shell>` subcommand that emits a completion script. Supported shells: `bash`, `zsh`, `fish`, `powershell`.

``` sh
source <(${BIN} completion bash)
```

Per-command custom completer:

``` go
ShellComplete: func(ctx context.Context, cmd *cli.Command) {
    fmt.Fprintln(cmd.Root().Writer, "--option-a")
    fmt.Fprintln(cmd.Root().Writer, "--option-b")
},
```

Built-in completer helpers: `DefaultRootCommandComplete`, `DefaultCompleteWithFlags`. A `(*Command).ToFishCompletion() (string, error)` method renders fish completion programmatically.

## Common code patterns

### Minimal root command

``` go
package main

import (
    "context"
    "fmt"
    "log"
    "os"

    "github.com/urfave/cli/v3"
)

func main() {
    cmd := &cli.Command{
        Name:  "boom",
        Usage: "make an explosive entrance",
        Action: func(ctx context.Context, cmd *cli.Command) error {
            fmt.Println("boom!")
            return nil
        },
    }
    if err := cmd.Run(context.Background(), os.Args); err != nil {
        log.Fatal(err)
    }
}
```

### Subcommands

``` go
cmd := &cli.Command{
    Name: "app",
    Commands: []*cli.Command{
        {
            Name:    "add",
            Aliases: []string{"a"},
            Usage:   "add a task",
            Action: func(ctx context.Context, cmd *cli.Command) error {
                fmt.Println("added:", cmd.Args().First())
                return nil
            },
        },
        {
            Name: "template",
            Commands: []*cli.Command{
                {Name: "add",    Action: addTemplate},
                {Name: "remove", Action: removeTemplate},
            },
        },
    },
}
```

### Flags with binding, env source, validation

``` go
var port int

cmd := &cli.Command{
    Flags: []cli.Flag{
        &cli.StringFlag{
            Name:    "lang",
            Aliases: []string{"l"},
            Value:   "english",
            Usage:   "language for the greeting",
            Sources: cli.EnvVars("APP_LANG", "LANG"),
        },
        &cli.IntFlag{
            Name:        "port",
            Value:       8080,
            Destination: &port,
            Validator: func(p int) error {
                if p < 1 || p >= 65536 {
                    return fmt.Errorf("port %d out of range", p)
                }
                return nil
            },
        },
        &cli.BoolFlag{
            Name:     "verbose",
            Aliases:  []string{"v"},
            Local:    false, // inherited by subcommands
        },
    },
    Action: func(ctx context.Context, cmd *cli.Command) error {
        lang := cmd.String("lang")
        verbose := cmd.Bool("verbose")
        _ = lang
        _ = verbose
        return nil
    },
}
```

### Typed positional arguments

``` go
cmd := &cli.Command{
    Name:      "scan",
    ArgsUsage: "FILE",
    Arguments: []cli.Argument{
        &cli.StringArg{Name: "file"},
    },
    Action: func(ctx context.Context, cmd *cli.Command) error {
        path := cmd.StringArg("file")
        return scanFile(path)
    },
}
```

### Variadic typed arguments

``` go
Arguments: []cli.Argument{
    &cli.StringArgs{Name: "files", Min: 1, Max: -1},
},
Action: func(ctx context.Context, cmd *cli.Command) error {
    for _, f := range cmd.StringArgs("files") {
        // ...
    }
    return nil
},
```

### Required and mutually exclusive flags

``` go
Flags: []cli.Flag{
    &cli.StringFlag{Name: "config", Required: true},
},
MutuallyExclusiveFlags: []cli.MutuallyExclusiveFlags{
    {
        Required: true,
        Flags: [][]cli.Flag{
            { &cli.StringFlag{Name: "login"} },
            { &cli.Int64Flag{Name: "id"} },
        },
    },
},
```

### Counting flag

``` go
var count int
&cli.BoolFlag{
    Name:    "verbose",
    Aliases: []string{"v"},
    Config:  cli.BoolConfig{Count: &count},
}
// With UseShortOptionHandling, -vvv yields count == 3.
```

### Exit with specific code

``` go
Action: func(ctx context.Context, cmd *cli.Command) error {
    if !cmd.Bool("ok") {
        return cli.Exit("validation failed", 1)
    }
    return nil
},
```

In `main`:

``` go
if err := cmd.Run(context.Background(), os.Args); err != nil {
    cli.HandleExitCoder(err) // calls OsExiter with err.ExitCode()
}
```

### Context propagation via `Before`

``` go
Before: func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
    g, err := tbx.Load(cmd.String("tbx"))
    if err != nil {
        return ctx, err
    }
    return context.WithValue(ctx, glossaryKey{}, g), nil
},
Action: func(ctx context.Context, cmd *cli.Command) error {
    g := ctx.Value(glossaryKey{}).(*tbx.Glossary)
    _ = g
    return nil
},
```

### Authors

``` go
import "net/mail"

Authors: []any{
    mail.Address{Name: "Example Human", Address: "human@example.com"},
},
```

## Differences from v2

| v2                                 | v3                                                                 |
| ---------------------------------- | ------------------------------------------------------------------ |
| `cli.App{}`                        | `cli.Command{}` (root is a Command)                                |
| `func(c *cli.Context) error`       | `func(ctx context.Context, cmd *cli.Command) error`                |
| `c.String("x")`                    | `cmd.String("x")`                                                  |
| `Subcommands: []*cli.Command{...}` | `Commands: []*cli.Command{...}`                                    |
| `EnableBashCompletion: true`       | `EnableShellCompletion: true`                                      |
| `EnvVars: []string{"X"}`           | `Sources: cli.EnvVars("X")`                                        |
| `FilePath: "/p"`                   | `Sources: cli.Files("/p")`                                         |
| `PathFlag`                         | `StringFlag` with `TakesFile: true`                                |
| `TimestampFlag{Layout: t}`         | `TimestampFlag{Config: cli.TimestampConfig{Layouts: []string{t}}}` |
| `Authors: []*cli.Author{...}`      | `Authors: []any{ mail.Address{...} }`                              |
| `app.Run(os.Args)`                 | `cmd.Run(context.Background(), os.Args)`                           |
| altsrc in main module              | `github.com/urfave/cli-altsrc/v3` (separate module)                |


## Sources

- <https://cli.urfave.org/v3/getting-started/>
- <https://cli.urfave.org/v3/examples/flags/basics/>
- <https://cli.urfave.org/v3/examples/flags/value-sources/>
- <https://cli.urfave.org/v3/examples/flags/advanced/>
- <https://cli.urfave.org/v3/examples/arguments/advanced/>
- <https://cli.urfave.org/v3/examples/subcommands/basics/>
- <https://cli.urfave.org/v3/examples/exit-codes/>
- <https://cli.urfave.org/v3/examples/completions/shell-completions/>
- <https://cli.urfave.org/migrate-v2-to-v3/>
- <https://pkg.go.dev/github.com/urfave/cli/v3>
