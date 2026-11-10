// Staticlint is the project multichecker: the standard go vet analyzers, staticcheck, third-party
// analyzers and the custom osexit analyzer in a single binary.
//
// Run it from the module root:
//
//	go run ./cmd/staticlint ./...
//
// or build and run the binary:
//
//	go build -o staticlint ./cmd/staticlint
//	./staticlint ./...
//
// Flags are the standard multichecker flags (go doc golang.org/x/tools/go/analysis/multichecker):
// -<analyzer>=false disables a check, -fix applies fixes, -json prints JSON,
// -help prints the list of analyzers. Exit code 3 means there are findings. CI runs the same command.
//
// # Standard analyzers from golang.org/x/tools/go/analysis/passes
//
//   - appends — append with no values to append
//   - asmdecl — mismatches between assembly files and Go declarations
//   - assign — useless assignment x = x
//   - atomic — x = atomic.AddInt64(&x, 1) and the like
//   - atomicalign — misaligned 64-bit sync/atomic arguments
//   - bools — mistakes in boolean expressions
//   - buildtag — malformed //go:build and // +build
//   - cgocall — passing Go pointers to cgo in violation of the rules
//   - composite — unkeyed composite literals of types from other packages
//   - copylock — locks copied by value
//   - deepequalerrors — reflect.DeepEqual on errors
//   - defers — defer with arguments evaluated immediately (time.Since)
//   - directive — malformed //go: directives
//   - errorsas — second argument of errors.As is not a pointer
//   - framepointer — frame pointer clobbered in assembly
//   - gofix — //go:fix inline directives
//   - hostport — address built with fmt.Sprintf("%s:%d") instead of net.JoinHostPort
//   - httpmux — new-syntax ServeMux patterns with an old go version
//   - httpresponse — using resp before checking the error
//   - ifaceassert — impossible interface type assertions
//   - inline — calls to functions marked //go:fix inline
//   - loopclosure — loop variable capture (for go < 1.22)
//   - lostcancel — lost cancel from context.WithCancel
//   - nilfunc — comparing a function with nil
//   - nilness — certain nil dereferences and redundant checks
//   - printf — Printf format does not match the arguments
//   - reflectvaluecompare — comparing reflect.Value with ==
//   - scannererr — bufio.Scanner.Err is not checked
//   - shadow — shadowed variables
//   - shift — shifts equal to or exceeding the type width
//   - sigchanyzer — unbuffered channel in signal.Notify
//   - slog — mismatched key-value pairs in log/slog
//   - sortslice — sort.Slice on a non-slice
//   - sqlrowserr — sql.Rows.Err is not checked
//   - stdmethods — wrong signatures of String, Read, MarshalJSON and the like
//   - stdversion — API newer than the go version in go.mod
//   - stringintconv — string(int)
//   - structtag — malformed struct tags
//   - testinggoroutine — t.Fatal from a test goroutine
//   - tests — mistakes in test and example names and signatures
//   - timeformat — time format 2006-02-01
//   - unmarshal — Unmarshal into a non-pointer
//   - unreachable — unreachable code
//   - unsafeptr — invalid uintptr to unsafe.Pointer conversions
//   - unusedresult — unused results of pure functions
//   - unusedwrite — writes to fields that are never read
//   - waitgroup — wg.Add inside a goroutine
//   - modernize — suggestions for modern Go (except any: we do not enforce interface{} → any)
//
// Passes from the same directory that are not enabled, because they find no bugs:
//
//   - buildssa, ctrlflow, inspect — helper passes that only build SSA, a control flow graph or an AST
//     inspector for other analyzers; they report nothing and run anyway as dependencies
//   - fieldalignment — suggests reordering struct fields to save bytes; noisy, and the order chosen for
//     readability matters more here than a few bytes of padding
//   - findcall — demo analyzer that reports calls to a function with a given name
//   - pkgfact — demo analyzer of package facts
//   - usesgenerics — reports which generic features a package uses, an input for other analyzers
//
// # Staticcheck analyzers
//
// The checks come from config.json next to the staticlint executable, or from the built-in
// cmd/staticlint/internal/checks/config.json if there is none:
//
//	{"staticcheck": ["all", "-ST1000"]}
//
// "all" enables every SA (bugs), S (simplifications), ST (style) and QF (quick fixes) check, "-NAME" disables one,
// a plain name such as "SA5000" enables one; later entries take precedence. The built-in config disables ST1000:
// the package comment requirement is covered by revive in golangci-lint.
//
// All enabled staticcheck checks (the list is updated by go generate, do not edit by hand):
//
//   - SA1000 — Invalid regular expression
//   - SA1001 — Invalid template
//   - SA1002 — Invalid format in `time.Parse`
//   - SA1003 — Unsupported argument to functions in `encoding/binary`
//   - SA1004 — Suspiciously small untyped constant in `time.Sleep`
//   - SA1005 — Invalid first argument to `exec.Command`
//   - SA1006 — `Printf` with dynamic first argument and no further arguments
//   - SA1007 — Invalid URL in `net/url.Parse`
//   - SA1008 — Non-canonical key in `http.Header` map
//   - SA1010 — `(*regexp.Regexp).FindAll` called with `n == 0`, which will always return zero results
//   - SA1011 — Various methods in the "strings" package expect valid UTF-8, but invalid input is provided
//   - SA1012 — A nil `context.Context` is being passed to a function, consider using `context.TODO` instead
//   - SA1013 — `io.Seeker.Seek` is being called with the whence constant as the first argument, but it should be the second
//   - SA1014 — Non-pointer value passed to `Unmarshal` or `Decode`
//   - SA1015 — Using `time.Tick` in a way that will leak. Consider using `time.NewTicker`, and only use `time.Tick` in tests, commands and endless functions
//   - SA1016 — Trapping a signal that cannot be trapped
//   - SA1017 — Channels used with `os/signal.Notify` should be buffered
//   - SA1018 — `strings.Replace` called with `n == 0`, which does nothing
//   - SA1019 — Using a deprecated function, variable, constant or field
//   - SA1020 — Using an invalid host:port pair with a `net.Listen`-related function
//   - SA1021 — Using `bytes.Equal` to compare two `net.IP`
//   - SA1023 — Modifying the buffer in an `io.Writer` implementation
//   - SA1024 — A string cutset contains duplicate characters
//   - SA1025 — It is not possible to use `(*time.Timer).Reset`'s return value correctly
//   - SA1026 — Cannot marshal channels or functions
//   - SA1027 — Atomic access to 64-bit variable must be 64-bit aligned
//   - SA1028 — `sort.Slice` can only be used on slices
//   - SA1029 — Inappropriate key in call to `context.WithValue`
//   - SA1030 — Invalid argument in call to a `strconv` function
//   - SA1031 — Overlapping byte slices passed to an encoder
//   - SA1032 — Wrong order of arguments to `errors.Is`
//   - SA2000 — `(*sync.WaitGroup).Add` called inside the goroutine, leading to a race condition
//   - SA2001 — Empty critical section, did you mean to defer the unlock?
//   - SA2002 — Called `testing.T.FailNow` or `SkipNow` in a goroutine, which isn't allowed
//   - SA2003 — Deferred `Lock` right after locking, likely meant to defer `Unlock` instead
//   - SA3000 — `TestMain` doesn't call `os.Exit`, hiding test failures
//   - SA3001 — Assigning to `b.N` in benchmarks distorts the results
//   - SA4000 — Binary operator has identical expressions on both sides
//   - SA4001 — `&*x` gets simplified to `x`, it does not copy `x`
//   - SA4003 — Comparing unsigned values against negative values is pointless
//   - SA4004 — The loop exits unconditionally after one iteration
//   - SA4005 — Field assignment that will never be observed. Did you mean to use a pointer receiver?
//   - SA4006 — A value assigned to a variable is never read before being overwritten. Forgotten error check or dead code?
//   - SA4008 — The variable in the loop condition never changes, are you incrementing the wrong variable?
//   - SA4009 — A function argument is overwritten before its first use
//   - SA4010 — The result of `append` will never be observed anywhere
//   - SA4011 — Break statement with no effect. Did you mean to break out of an outer loop?
//   - SA4012 — Comparing a value against NaN even though no value is equal to NaN
//   - SA4013 — Negating a boolean twice (`!!b`) is the same as writing `b`. This is either redundant, or a typo.
//   - SA4014 — An if/else if chain has repeated conditions and no side-effects; if the condition didn't match the first time, it won't match the second time, either
//   - SA4015 — Calling functions like `math.Ceil` on floats converted from integers doesn't do anything useful
//   - SA4016 — Certain bitwise operations, such as `x ^ 0`, do not do anything useful
//   - SA4017 — Discarding the return values of a function without side effects, making the call pointless
//   - SA4018 — Self-assignment of variables
//   - SA4019 — Multiple, identical build constraints in the same file
//   - SA4020 — Unreachable case clause in a type switch
//   - SA4021 — "x = append(y)" is equivalent to "x = y"
//   - SA4022 — Comparing the address of a variable against nil
//   - SA4023 — Impossible comparison of interface value with untyped nil
//   - SA4024 — Checking for impossible return value from a builtin function
//   - SA4025 — Integer division of literals that results in zero
//   - SA4026 — Go constants cannot express negative zero
//   - SA4027 — `(*net/url.URL).Query` returns a copy, modifying it doesn't change the URL
//   - SA4028 — `x % 1` is always zero
//   - SA4029 — Ineffective attempt at sorting slice
//   - SA4030 — Ineffective attempt at generating random number
//   - SA4031 — Checking never-nil value against nil
//   - SA4032 — Comparing `runtime.GOOS` or `runtime.GOARCH` against impossible value
//   - SA5000 — Assignment to nil map
//   - SA5001 — Deferring `Close` before checking for a possible error
//   - SA5002 — The empty for loop ("for {}") spins and can block the scheduler
//   - SA5003 — Defers in infinite loops will never execute
//   - SA5004 — "for { select { ..." with an empty default branch spins
//   - SA5005 — The finalizer references the finalized object, preventing garbage collection
//   - SA5007 — Infinite recursive call
//   - SA5008 — Invalid struct tag
//   - SA5009 — Invalid Printf call
//   - SA5010 — Impossible type assertion
//   - SA5012 — Passing odd-sized slice to function expecting even size
//   - SA6000 — Using `regexp.Match` or related in a loop, should use `regexp.Compile`
//   - SA6001 — Missing an optimization opportunity when indexing maps by byte slices
//   - SA6002 — Storing non-pointer values in `sync.Pool` allocates memory
//   - SA6003 — Converting a string to a slice of runes before ranging over it
//   - SA6005 — Inefficient string comparison with `strings.ToLower` or `strings.ToUpper`
//   - SA6006 — Using io.WriteString to write `[]byte`
//   - SA9001 — Defers in range loops may not run when you expect them to
//   - SA9002 — Using a non-octal `os.FileMode` that looks like it was meant to be in octal.
//   - SA9003 — Empty body in an if or else branch
//   - SA9004 — Only the first constant has an explicit type
//   - SA9005 — Trying to marshal a struct with no public fields nor custom marshaling
//   - SA9006 — Dubious bit shifting of a fixed size integer value
//   - SA9007 — Deleting a directory that shouldn't be deleted
//   - SA9008 — `else` branch of a type assertion is probably not reading the right value
//   - SA9009 — Ineffectual Go compiler directive
//   - SA9010 — Returned function should be called in defer
//   - S1000 — Use plain channel send or receive instead of single-case select
//   - S1001 — Replace for loop with call to copy
//   - S1002 — Omit comparison with boolean constant
//   - S1003 — Replace call to `strings.Index` with `strings.Contains`
//   - S1004 — Replace call to `bytes.Compare` with `bytes.Equal`
//   - S1005 — Drop unnecessary use of the blank identifier
//   - S1006 — Use "for { ... }" for infinite loops
//   - S1007 — Simplify regular expression by using raw string literal
//   - S1008 — Simplify returning boolean expression
//   - S1009 — Omit redundant nil check on slices, maps, and channels
//   - S1010 — Omit default slice index
//   - S1011 — Use a single `append` to concatenate two slices
//   - S1012 — Replace `time.Now().Sub(x)` with `time.Since(x)`
//   - S1016 — Use a type conversion instead of manually copying struct fields
//   - S1017 — Replace manual trimming with `strings.TrimPrefix`
//   - S1018 — Use "copy" for sliding elements
//   - S1019 — Simplify "make" call by omitting redundant arguments
//   - S1020 — Omit redundant nil check in type assertion
//   - S1021 — Merge variable declaration and assignment
//   - S1023 — Omit redundant control flow
//   - S1024 — Replace `x.Sub(time.Now())` with `time.Until(x)`
//   - S1025 — Don't use `fmt.Sprintf("%s", x)` unnecessarily
//   - S1028 — Simplify error construction with `fmt.Errorf`
//   - S1029 — Range over the string directly
//   - S1030 — Use `bytes.Buffer.String` or `bytes.Buffer.Bytes`
//   - S1031 — Omit redundant nil check around loop
//   - S1032 — Use `sort.Ints(x)`, `sort.Float64s(x)`, and `sort.Strings(x)`
//   - S1033 — Unnecessary guard around call to "delete"
//   - S1034 — Use result of type assertion to simplify cases
//   - S1035 — Redundant call to `net/http.CanonicalHeaderKey` in method call on `net/http.Header`
//   - S1036 — Unnecessary guard around map access
//   - S1037 — Elaborate way of sleeping
//   - S1038 — Unnecessarily complex way of printing formatted string
//   - S1039 — Unnecessary use of `fmt.Sprint`
//   - S1040 — Type assertion to current type
//   - ST1001 — Dot imports are discouraged
//   - ST1003 — Poorly chosen identifier
//   - ST1005 — Incorrectly formatted error string
//   - ST1006 — Poorly chosen receiver name
//   - ST1008 — A function's error value should be its last return value
//   - ST1011 — Poorly chosen name for variable of type `time.Duration`
//   - ST1012 — Poorly chosen name for error variable
//   - ST1013 — Should use constants for HTTP error codes, not magic numbers
//   - ST1015 — A switch's default case should be the first or last case
//   - ST1016 — Use consistent method receiver names
//   - ST1017 — Don't use Yoda conditions
//   - ST1018 — Avoid zero-width and control characters in string literals
//   - ST1019 — Importing the same package multiple times
//   - ST1020 — The documentation of an exported function should start with the function's name
//   - ST1021 — The documentation of an exported type should start with type's name
//   - ST1022 — The documentation of an exported variable or constant should start with variable's name
//   - ST1023 — Redundant type in variable declaration
//   - QF1001 — Apply De Morgan's law
//   - QF1002 — Convert untagged switch to tagged switch
//   - QF1003 — Convert if/else-if chain to tagged switch
//   - QF1004 — Use `strings.ReplaceAll` instead of `strings.Replace` with `n == -1`
//   - QF1005 — Expand call to `math.Pow`
//   - QF1006 — Lift `if`+`break` into loop condition
//   - QF1007 — Merge conditional assignment into variable declaration
//   - QF1008 — Omit embedded fields from selector expression
//   - QF1009 — Use `time.Time.Equal` instead of `==` operator
//   - QF1010 — Convert slice of bytes to string when printing it
//   - QF1011 — Omit redundant type from variable declaration
//   - QF1012 — Use `fmt.Fprintf(x, ...)` instead of `x.Write(fmt.Sprintf(...))`
//
// # Third-party analyzers
//
//   - errcheck (github.com/kisielk/errcheck) — unchecked errors
//   - ineffassign (github.com/gordonklaus/ineffassign) — assignments whose value is never used
//
// # Custom osexit analyzer
//
// It forbids a direct os.Exit call in the main function of package main, including in closures,
// through a renamed import and through a dot import, and os.Exit taken there as a value (f := os.Exit).
// main itself must not call os.Exit; how the kit and its commands exit instead is described below.
// An indirect exit (log.Fatal, an slog logger with Fatal, C._exit via cgo) is not caught:
// it is visible in the code by its name. An os.Exit call outside main, in another function or package, is not caught
// either: a game built on the kit exits through kit.Run, so its main never calls os.Exit, and the helper commands
// exit through the logger's Fatal; both cases are deliberate.
package main
