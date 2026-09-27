// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2022 The Ebitengine Authors

//go:build darwin || freebsd || (linux && (386 || amd64 || arm || arm64 || loong64 || ppc64le || riscv64 || (s390x && (cgo || go1.27)))) || netbsd

package purego

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"reflect"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"time"
	"unsafe"

	internalstrings "github.com/bnema/purego/internal/strings"
)

var syscallXABI0 uintptr

func syscall_syscallN(fn uintptr, args ...uintptr) (r1, r2, err uintptr) {
	panic("purego: syscall_syscallN is only supported on windows")
}

// NewCallback converts a Go function to a function pointer conforming to the C calling convention.
// This is useful when interoperating with C code requiring callbacks. The argument is expected to be a
// function with zero or one uintptr-sized result. The function must not have arguments with size larger than the size
// of uintptr. Only a limited number of callbacks may be created in a single Go process, and any memory allocated
// for these callbacks is never released. At least 1024 callbacks can always be created. Although this function
// provides similar functionality to windows.NewCallback it is distinct.
//
// Every call to NewCallback creates a new callback even for the same function value, so passing a Go callback to C
// inside a loop (e.g. a qsort comparator) keeps consuming callbacks and eventually panics once they are exhausted.
// The same happens when a func value is passed to a C function, as [RegisterFunc] creates a new callback for each
// call. Create the callback once with NewCallback and reuse the returned pointer instead.
func NewCallback(fn any) uintptr {
	ty := reflect.TypeOf(fn)
	if ty == nil || ty.Kind() != reflect.Func {
		panic("purego: the type must be a function but was not")
	}
	for i := range ty.NumIn() {
		in := ty.In(i)
		if !in.AssignableTo(reflect.TypeFor[CDecl]()) {
			continue
		}
		if i != 0 {
			panic("purego: CDecl must be the first argument")
		}
	}
	return compileCallback(fn)
}

// CallbackArgs is the integer-class argument block of one C call into a
// NewCallbackInts callback. It is only valid during the callback.
type CallbackArgs struct {
	frame      unsafe.Pointer
	intFrame   unsafe.Pointer
	stackFrame unsafe.Pointer
}

// Int returns the i-th integer-class argument (0-based), following the
// platform C ABI: integer registers first, then stack slots.
func (a *CallbackArgs) Int(i int) uintptr {
	if i < numOfIntegerRegisters() {
		if a.intFrame != nil {
			return (*[callbackMaxFrame]uintptr)(a.intFrame)[i]
		}
		return (*[callbackMaxFrame]uintptr)(a.frame)[numOfFloatRegisters()+i]
	}
	k := i - numOfIntegerRegisters()
	if a.stackFrame != nil {
		return (*[callbackMaxFrame]uintptr)(a.stackFrame)[k]
	}
	return (*[callbackMaxFrame]uintptr)(a.frame)[numOfFloatRegisters()+numOfIntegerRegisters()+k]
}

// NewCallbackInts returns a C function pointer that calls fn with the raw
// integer-class arguments of the C call and returns its result in the C
// integer return register. Only integer-class C parameters (integers,
// pointers) are supported; float arguments are not visible. The call path
// does no reflection and no allocation. Like NewCallback, the callback is
// never freed and counts against the callback limit.
func NewCallbackInts(fn func(a *CallbackArgs) uintptr) uintptr {
	return compileFastCallback(fn)
}

// NewCallbackFnPtr converts a Go function pointer to a function pointer conforming to the C calling convention.
// Calling this function multiple times with the same function pointer returns the original callback address.
func NewCallbackFnPtr(fnPtr any) uintptr {
	val := reflect.ValueOf(fnPtr)
	if val.IsNil() {
		panic("purego: function must not be nil")
	}
	if val.Kind() != reflect.Ptr || val.Elem().Kind() != reflect.Func {
		panic("purego: the type must be a function pointer but was not")
	}
	if addr, idx, ok := getCallbackByFnPtr(val); ok {
		cbs.lock.Lock()
		recordCallbackLedger("dedupe", idx, addr, val.Pointer(), val.Elem(), false, "")
		cbs.lock.Unlock()
		return addr
	}
	addr := compileCallback(val.Elem().Interface())
	cbs.lock.Lock()
	cbs.knownFnPtr[val.Pointer()] = addr
	if idx, ok := cbs.knownIdx[addr]; ok {
		cbs.fnPtrKeys[idx] = val.Pointer()
		recordCallbackLedger("fnptr-bind", idx, addr, val.Pointer(), val.Elem(), false, "")
	}
	cbs.lock.Unlock()
	return addr
}

// UnrefCallback unreferences a callback created by NewCallback or NewCallbackFnPtr and frees its slot.
func UnrefCallback(cb uintptr) error {
	cbs.lock.Lock()
	defer cbs.lock.Unlock()
	idx, ok := cbs.knownIdx[cb]
	if !ok {
		recordCallbackLedger("release-miss", -1, cb, 0, reflect.Value{}, false, "callback not found")
		return errors.New("callback not found")
	}
	val := cbs.funcs[idx]
	key := cbs.fnPtrKeys[idx]
	if key != 0 {
		delete(cbs.knownFnPtr, key)
		cbs.fnPtrKeys[idx] = 0
	}
	delete(cbs.knownIdx, cb)
	cbs.holes[idx] = struct{}{}
	cbs.funcs[idx] = reflect.Value{}
	cbs.fast[idx] = nil
	recordCallbackLedger("release", idx, cb, key, val, false, "")
	return nil
}

// UnrefCallbackFnPtr unreferences a callback previously created via NewCallbackFnPtr.
func UnrefCallbackFnPtr(fnPtr any) error {
	val := reflect.ValueOf(fnPtr)
	if val.IsNil() {
		panic("purego: function must not be nil")
	}
	if val.Kind() != reflect.Ptr || val.Elem().Kind() != reflect.Func {
		panic("purego: the type must be a function pointer but was not")
	}
	cbs.lock.Lock()
	defer cbs.lock.Unlock()
	addr, ok := cbs.knownFnPtr[val.Pointer()]
	if !ok {
		recordCallbackLedger("release-fnptr-miss", -1, 0, val.Pointer(), val.Elem(), false, "callback not found")
		return errors.New("callback not found")
	}
	idx, ok := cbs.knownIdx[addr]
	if !ok {
		delete(cbs.knownFnPtr, val.Pointer())
		recordCallbackLedger("release-fnptr-miss", -1, addr, val.Pointer(), val.Elem(), false, "callback not found")
		return errors.New("callback not found")
	}
	callbackVal := cbs.funcs[idx]
	delete(cbs.knownFnPtr, val.Pointer())
	delete(cbs.knownIdx, addr)
	cbs.fnPtrKeys[idx] = 0
	cbs.holes[idx] = struct{}{}
	cbs.funcs[idx] = reflect.Value{}
	recordCallbackLedger("release-fnptr", idx, addr, val.Pointer(), callbackVal, false, "")
	return nil
}

// maxCb is the maximum number of callbacks
// only increase this if you have added more to the callbackasm function
const maxCB = 2000

var cbs = struct {
	lock       sync.RWMutex
	numFn      int                                // the highest allocated callback index + 1
	holes      map[int]struct{}                   // reusable callback slots
	funcs      [maxCB]reflect.Value               // the saved callbacks
	fast       [maxCB]func(*CallbackArgs) uintptr // integer-only callbacks
	knownIdx   map[uintptr]int                    // callback address -> slot index
	knownFnPtr map[uintptr]uintptr                // function pointer variable address -> callback address
	fnPtrKeys  [maxCB]uintptr                     // slot index -> function pointer variable address
	ledgerSeq  uint64                             // monotonic callback ledger event sequence
}{
	holes:      make(map[int]struct{}),
	knownIdx:   make(map[uintptr]int, maxCB),
	knownFnPtr: make(map[uintptr]uintptr, maxCB),
}

func getCallbackByFnPtr(val reflect.Value) (uintptr, int, bool) {
	cbs.lock.RLock()
	defer cbs.lock.RUnlock()
	addr, ok := cbs.knownFnPtr[val.Pointer()]
	if !ok {
		return 0, -1, false
	}
	idx, ok := cbs.knownIdx[addr]
	if !ok {
		return addr, -1, true
	}
	return addr, idx, true
}

type callbackLedgerEvent struct {
	Marker     string `json:"marker"`
	PID        int    `json:"pid"`
	Seq        uint64 `json:"seq"`
	Time       string `json:"time"`
	Event      string `json:"event"`
	Index      int    `json:"index"`
	Addr       string `json:"addr,omitempty"`
	FnPtrKey   string `json:"fn_ptr_key,omitempty"`
	Type       string `json:"type,omitempty"`
	Family     string `json:"family"`
	StackKey   string `json:"stack_key"`
	NumFn      int    `json:"num_fn"`
	Occupied   int    `json:"occupied"`
	Reusable   int    `json:"reusable"`
	KnownFnPtr int    `json:"known_fn_ptr"`
	Remaining  int    `json:"remaining"`
	Max        int    `json:"max"`
	ReusedSlot bool   `json:"reused_slot,omitempty"`
	Stack      string `json:"stack,omitempty"`
	Error      string `json:"error,omitempty"`
}

func callbackEnvEnabled(name string) bool {
	switch strings.ToLower(os.Getenv(name)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func callbackLedgerEnabled() bool {
	return callbackEnvEnabled("PUREGO_CALLBACK_LEDGER")
}

func callbackLedgerStackEnabled() bool {
	return callbackEnvEnabled("PUREGO_CALLBACK_LEDGER_STACK")
}

func callbackTraceEnabled() bool {
	return callbackEnvEnabled("PUREGO_CALLBACK_TRACE")
}

func callbackLedgerPath() string {
	if path := os.Getenv("PUREGO_CALLBACK_LEDGER_FILE"); path != "" {
		return path
	}
	return fmt.Sprintf("/tmp/purego-callback-ledger-%d.jsonl", os.Getpid())
}

func typeName(val reflect.Value) string {
	if !val.IsValid() {
		return ""
	}
	return val.Type().String()
}

func hexUintptr(v uintptr) string {
	if v == 0 {
		return ""
	}
	return fmt.Sprintf("0x%x", v)
}

func classifyCallbackStack(stack string) (family, stackKey string) {
	family = "unknown"
	stackKey = "unknown"
	lines := strings.Split(stack, "\n")
	for i, line := range lines {
		if line == "" || strings.HasPrefix(line, "goroutine ") || strings.Contains(line, "runtime/debug.Stack") || strings.Contains(line, "recordCallbackLedger") || strings.Contains(line, "traceCallbackAllocation") {
			continue
		}
		if strings.Contains(line, "github.com/bnema/purego.") || strings.Contains(line, "github.com/ebitengine/purego.") {
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(line), "/") || strings.HasPrefix(strings.TrimSpace(line), "../") {
			continue
		}
		stackKey = strings.TrimSpace(line)
		for j := i + 1; j < len(lines); j++ {
			fileLine := strings.TrimSpace(lines[j])
			if fileLine == "" {
				continue
			}
			family = classifyCallbackFamily(fileLine + " " + stackKey)
			return family, stackKey
		}
		family = classifyCallbackFamily(stackKey)
		return family, stackKey
	}
	return family, stackKey
}

func classifyCallbackFamily(s string) string {
	switch {
	case strings.Contains(s, "purego-cef2gtk"):
		return "purego-cef2gtk"
	case strings.Contains(s, "purego-cef"):
		return "purego-cef"
	case strings.Contains(s, "puregotk"):
		return "puregotk"
	case strings.Contains(s, "dumber"):
		return "dumber"
	case strings.Contains(s, "github.com/bnema/purego") || strings.Contains(s, "/purego/"):
		return "purego"
	default:
		return "unknown"
	}
}

func recordCallbackLedger(event string, idx int, addr uintptr, fnPtrKey uintptr, val reflect.Value, reused bool, errText string) {
	if !callbackLedgerEnabled() {
		return
	}
	stack := ""
	family := "unknown"
	stackKey := "unknown"
	if callbackLedgerStackEnabled() {
		stack = string(debug.Stack())
		family, stackKey = classifyCallbackStack(stack)
	}
	cbs.ledgerSeq++
	occupied := cbs.numFn - len(cbs.holes)
	entry := callbackLedgerEvent{
		Marker:     "PUREGO-CALLBACK-LEDGER",
		PID:        os.Getpid(),
		Seq:        cbs.ledgerSeq,
		Time:       time.Now().Format(time.RFC3339Nano),
		Event:      event,
		Index:      idx,
		Addr:       hexUintptr(addr),
		FnPtrKey:   hexUintptr(fnPtrKey),
		Type:       typeName(val),
		Family:     family,
		StackKey:   stackKey,
		NumFn:      cbs.numFn,
		Occupied:   occupied,
		Reusable:   len(cbs.holes),
		KnownFnPtr: len(cbs.knownFnPtr),
		Remaining:  maxCB - occupied,
		Max:        maxCB,
		ReusedSlot: reused,
		Error:      errText,
	}
	if callbackLedgerStackEnabled() {
		entry.Stack = stack
	}
	payload, err := json.Marshal(entry)
	if err != nil {
		return
	}
	payload = append(payload, '\n')
	if f, err := os.OpenFile(callbackLedgerPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600); err == nil {
		_, _ = f.Write(payload)
		_ = f.Close()
	}
}

func traceCallbackAllocation(event string, val reflect.Value, remaining int) {
	if !callbackTraceEnabled() {
		return
	}
	message := fmt.Sprintf("PUREGO-CALLBACK-TRACE event=%s remaining=%d live=%d type=%s\n%s\n", event, remaining, maxCB-remaining, val.Type(), debug.Stack())
	fmt.Fprint(os.Stderr, message)
	traceFile := os.Getenv("PUREGO_CALLBACK_TRACE_FILE")
	if traceFile == "" {
		traceFile = fmt.Sprintf("/tmp/purego-callback-trace-%d.log", os.Getpid())
	}
	if f, err := os.OpenFile(traceFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600); err == nil {
		_, _ = f.WriteString(message)
		_ = f.Close()
	}
}

func compileCallback(fn any) uintptr {
	val := reflect.ValueOf(fn)
	if val.Kind() != reflect.Func {
		panic("purego: the type must be a function but was not")
	}
	if val.IsNil() {
		panic("purego: function must not be nil")
	}
	ty := val.Type()
	for i := range ty.NumIn() {
		in := ty.In(i)
		switch in.Kind() {
		case reflect.Struct:
			if i == 0 && in.AssignableTo(reflect.TypeFor[CDecl]()) {
				continue
			}
			ensureCallbackStructSupported()
			checkStructFieldsSupported(in)
			continue
		case reflect.Interface, reflect.Func, reflect.Slice,
			reflect.Chan, reflect.Complex64, reflect.Complex128,
			reflect.Map, reflect.Invalid:
			panic("purego: unsupported argument type: " + in.Kind().String())
		}
	}
output:
	switch {
	case ty.NumOut() == 1:
		switch ty.Out(0).Kind() {
		case reflect.Struct:
			ensureCallbackStructSupported()
			checkStructFieldsSupported(ty.Out(0))
			break output
		case reflect.Pointer, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
			reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
			reflect.Bool, reflect.UnsafePointer:
			break output
		}
		panic("purego: unsupported return type: " + ty.String())
	case ty.NumOut() > 1:
		panic("purego: callbacks can only have one return")
	}
	cbs.lock.Lock()
	defer cbs.lock.Unlock()
	return allocateCallback(val, nil)
}

func compileFastCallback(fn func(*CallbackArgs) uintptr) uintptr {
	if fn == nil {
		panic("purego: function must not be nil")
	}
	cbs.lock.Lock()
	defer cbs.lock.Unlock()
	return allocateCallback(reflect.Value{}, fn)
}

// allocateCallback is called with cbs.lock held.
func allocateCallback(val reflect.Value, fast func(*CallbackArgs) uintptr) uintptr {
	index := -1
	reused := false
	for i := range cbs.holes {
		index = i
		delete(cbs.holes, i)
		reused = true
		break
	}
	if index < 0 {
		remaining := maxCB - cbs.numFn
		if remaining <= 0 {
			recordCallbackLedger("exhausted", -1, 0, 0, val, false, "maximum callbacks reached")
			if val.IsValid() {
				traceCallbackAllocation("exhausted", val, 0)
			}
			panic("purego: the maximum number of callbacks has been reached")
		}
		if val.IsValid() && (remaining <= 100 || remaining%250 == 0) {
			traceCallbackAllocation("allocated", val, remaining)
		}
		index = cbs.numFn
		cbs.numFn++
	}
	cbs.funcs[index] = val
	cbs.fast[index] = fast
	addr := callbackasmAddr(index)
	cbs.knownIdx[addr] = index
	if reused {
		recordCallbackLedger("reuse", index, addr, 0, val, true, "")
	} else {
		recordCallbackLedger("alloc", index, addr, 0, val, false, "")
	}
	return addr
}

const ptrSize = unsafe.Sizeof((*int)(nil))

const callbackMaxFrame = 64 * ptrSize

// callbackasm is implemented in zcallback_GOOS_GOARCH.s
//
//go:linkname __callbackasm callbackasm
var __callbackasm byte
var callbackasmABI0 = uintptr(unsafe.Pointer(&__callbackasm))

// callbackWrap_call allows the calling of the ABIInternal wrapper
// which is required for runtime.cgocallback without the
// <ABIInternal> tag which is only allowed in the runtime.
// This closure is used inside sys_darwin_GOARCH.s
var callbackWrap_call = callbackWrap

// noescape is runtime.noescape; the synchronous callee must not retain p.
//go:noescape
//go:linkname noescape runtime.noescape
func noescape(p unsafe.Pointer) unsafe.Pointer

// callbackWrap is called by assembly code which determines which Go function to call.
// This function takes the arguments and passes them to the Go function and returns the result.
func callbackWrap(a *callbackArgs) {
	// Slots are published under the lock before the C pointer is handed out.
	// A caller must not invoke a released callback while UnrefCallback reuses
	// its slot; while callable, fast slots are never cleared.
	if f := cbs.fast[a.index]; f != nil {
		ca := CallbackArgs{frame: a.args, intFrame: a.intFrame(), stackFrame: a.stackFrame()}
		a.result[0] = f((*CallbackArgs)(noescape(unsafe.Pointer(&ca))))
		return
	}
	cbs.lock.Lock()
	fn := cbs.funcs[a.index]
	cbs.lock.Unlock()
	fnType := fn.Type()
	args := make([]reflect.Value, fnType.NumIn())
	frame := (*[callbackMaxFrame]uintptr)(a.args)
	// stackFrame points to stack-passed arguments. On most architectures this is
	// contiguous with frame (after register args), but on ppc64le it's separate.
	var stackFrame *[callbackMaxFrame]uintptr
	var intFrame *[callbackMaxFrame]uintptr
	if sf := a.stackFrame(); sf != nil {
		// Only ppc64le uses separate stackArgs pointer due to NOSPLIT constraints
		stackFrame = (*[callbackMaxFrame]uintptr)(sf)
	}
	if intf := a.intFrame(); intf != nil {
		intFrame = (*[callbackMaxFrame]uintptr)(intf)
	}
	// floatsN and intsN track the number of register slots used, not argument count.
	// This distinction matters on ARM32 where float64 uses 2 slots (32-bit registers).
	var floatsN int
	var intsN int
	// On amd64/loong64/ppc64le/riscv64/s390x, when returning a struct larger than
	// maxRegAllocStructSize, the caller passes a hidden pointer in the first integer
	// register. Skip it to avoid misreading it as the first function argument.
	if (runtime.GOARCH == "amd64" || runtime.GOARCH == "loong64" || runtime.GOARCH == "riscv64" || runtime.GOARCH == "s390x") &&
		fnType.NumOut() == 1 && fnType.Out(0).Kind() == reflect.Struct &&
		fnType.Out(0).Size() > maxRegAllocStructSize {
		intsN = 1
	}
	// stackSlot points to the index into frame (or stackFrame) of the current stack element.
	// When stackFrame is nil, stack begins after float and integer registers in frame.
	// When stackFrame is not nil (ppc64le), stackSlot indexes into stackFrame starting at 0.
	stackSlot := numOfIntegerRegisters() + numOfFloatRegisters()
	if stackFrame != nil {
		// ppc64le: stackArgs is a separate pointer, indices start at 0
		stackSlot = 0
	}
	// stackByteOffset tracks the byte offset within the stack area for Darwin ARM64
	// tight packing. On Darwin ARM64, C passes small types packed on the stack.
	stackByteOffset := uintptr(0)
	for i := range args {
		// slots is the number of pointer-sized slots the argument takes
		inType := fnType.In(i)
		slots := int((inType.Size() + ptrSize - 1) / ptrSize)
		switch inType.Kind() {
		case reflect.Float32, reflect.Float64:
			if isARMSoftFloat() {
				// we should restore from integer slot, can skip unnecessary branching here
				if isARMPaddingNeeded(inType, -1, intsN) {
					intsN++
				}
				if intsN+slots <= numOfIntegerRegisters() {
					// the integers begin after the floats in frame
					args[i] = reflect.NewAt(inType, unsafe.Pointer(&frame[intsN+numOfFloatRegisters()])).Elem()
					intsN += slots
					continue
				}
				if isARMPaddingNeeded(inType, -1, stackSlot) {
					stackSlot++
				}
				args[i] = reflect.NewAt(inType, unsafe.Pointer(&frame[stackSlot])).Elem()
				stackSlot += slots
				intsN += slots
				continue
			}

			if floatsN+slots > numOfFloatRegisters() {
				if isDarwin && runtime.GOARCH == "arm64" {
					// Darwin ARM64: read from packed stack with proper alignment
					args[i] = callbackArgFromStack(a.args, stackSlot, &stackByteOffset, inType)
				} else if stackFrame != nil {
					// ppc64le/s390x: stack args are in separate stackFrame
					switch runtime.GOARCH {
					case "ppc64le":
						args[i] = callbackFloatFromDoubleSlot(unsafe.Pointer(&stackFrame[stackSlot]), inType)
					case "s390x":
						// s390x big-endian: sub-8-byte values are right-justified
						args[i] = callbackArgFromSlotBigEndian(unsafe.Pointer(&stackFrame[stackSlot]), inType)
					default:
						args[i] = reflect.NewAt(inType, unsafe.Pointer(&stackFrame[stackSlot])).Elem()
					}
					stackSlot += slots
				} else if isARMFloatPaddingNeeded(inType, -1, stackSlot) {
					stackSlot++
					args[i] = reflect.NewAt(inType, unsafe.Pointer(&frame[stackSlot])).Elem()
					stackSlot += slots
				} else {
					args[i] = reflect.NewAt(inType, unsafe.Pointer(&frame[stackSlot])).Elem()
					stackSlot += slots
				}
			} else {
				switch runtime.GOARCH {
				case "ppc64le":
					args[i] = callbackFloatFromDoubleSlot(unsafe.Pointer(&frame[floatsN]), inType)
				case "s390x":
					// s390x big-endian: float32 is right-justified in 8-byte FPR slot
					args[i] = callbackArgFromSlotBigEndian(unsafe.Pointer(&frame[floatsN]), inType)
				default:
					args[i] = reflect.NewAt(inType, unsafe.Pointer(&frame[floatsN])).Elem()
				}
			}
			floatsN += slots
		case reflect.String:
			slots = 1
			var ptr uintptr
			if intsN+slots > numOfIntegerRegisters() {
				if runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" {
					ptr = uintptr(callbackArgFromStack(a.args, stackSlot, &stackByteOffset, reflect.TypeOf(uintptr(0))).Uint())
				} else if stackFrame != nil {
					if runtime.GOARCH == "s390x" {
						ptr = uintptr(callbackArgFromSlotBigEndian(unsafe.Pointer(&stackFrame[stackSlot]), reflect.TypeOf(uintptr(0))).Uint())
					} else {
						ptr = *(*uintptr)(unsafe.Pointer(&stackFrame[stackSlot]))
					}
					stackSlot += slots
				} else {
					ptr = frame[stackSlot]
					stackSlot += slots
				}
			} else {
				pos := intsN + numOfFloatRegisters()
				ptr = frame[pos]
			}
			intsN += slots
			args[i] = reflect.ValueOf(internalstrings.GoString(ptr))
			continue
		case reflect.Struct:
			if i == 0 && inType.AssignableTo(reflect.TypeFor[CDecl]()) {
				args[i] = reflect.Zero(inType)
				continue
			}
			if inType.Size() == 0 {
				args[i] = reflect.New(inType).Elem()
				continue
			}
			args[i] = getCallbackStruct(inType, a.args, &floatsN, &intsN, &stackSlot, &stackByteOffset)
			continue
		default:
			if isARMPaddingNeeded(inType, -1, intsN) {
				intsN++
			}
			if intsN+slots > numOfIntegerRegisters() {
				if isDarwin && runtime.GOARCH == "arm64" {
					// Darwin ARM64: read from packed stack with proper alignment
					args[i] = callbackArgFromStack(a.args, stackSlot, &stackByteOffset, inType)
				} else if stackFrame != nil {
					// ppc64le/s390x: stack args are in separate stackFrame
					if runtime.GOARCH == "s390x" {
						// s390x big-endian: sub-8-byte values are right-justified
						args[i] = callbackArgFromSlotBigEndian(unsafe.Pointer(&stackFrame[stackSlot]), inType)
					} else {
						args[i] = reflect.NewAt(inType, unsafe.Pointer(&stackFrame[stackSlot])).Elem()
					}
					stackSlot += slots
				} else if isARMPaddingNeeded(inType, -1, stackSlot) {
					stackSlot++
					args[i] = reflect.NewAt(inType, unsafe.Pointer(&frame[stackSlot])).Elem()
					stackSlot += slots
				} else {
					args[i] = reflect.NewAt(inType, unsafe.Pointer(&frame[stackSlot])).Elem()
					stackSlot += slots
				}
			} else {
				if intFrame != nil {
					args[i] = reflect.NewAt(inType, unsafe.Pointer(&intFrame[intsN])).Elem()
				} else {
					// the integers begin after the floats in frame
					pos := intsN + numOfFloatRegisters()
					if runtime.GOARCH == "s390x" {
						// s390x big-endian: sub-8-byte values are right-justified in GPR slot
						args[i] = callbackArgFromSlotBigEndian(unsafe.Pointer(&frame[pos]), inType)
					} else {
						args[i] = reflect.NewAt(inType, unsafe.Pointer(&frame[pos])).Elem()
					}
				}
			}
			intsN += slots
		}
	}
	ret := fn.Call(args)
	if len(ret) > 0 {
		switch k := ret[0].Kind(); k {
		case reflect.Uint64:
			a.setUint64Result(ret[0].Uint())
		case reflect.Uint, reflect.Uint32, reflect.Uint16, reflect.Uint8, reflect.Uintptr:
			a.result[0] = uintptr(ret[0].Uint())
		case reflect.Int64:
			a.setInt64Result(ret[0].Int())
		case reflect.Int, reflect.Int32, reflect.Int16, reflect.Int8:
			a.result[0] = uintptr(ret[0].Int())
		case reflect.Bool:
			if ret[0].Bool() {
				a.result[0] = 1
			} else {
				a.result[0] = 0
			}
		case reflect.Pointer:
			a.result[0] = ret[0].Pointer()
		case reflect.UnsafePointer:
			a.result[0] = ret[0].Pointer()
		case reflect.Struct:
			setStruct(a, ret[0])
		default:
			panic("purego: unsupported kind: " + k.String())
		}
	}
}

// callbackArgFromStack reads an argument from the tightly-packed stack area on Darwin ARM64.
// The C ABI on Darwin ARM64 packs small types on the stack without padding to 8 bytes.
// This function handles proper alignment and advances stackByteOffset accordingly.
func callbackArgFromStack(argsBase unsafe.Pointer, stackSlot int, stackByteOffset *uintptr, inType reflect.Type) reflect.Value {
	// Calculate base address of stack area (after float and int registers)
	stackBase := unsafe.Add(argsBase, stackSlot*int(ptrSize))

	// Get type's natural alignment
	align := uintptr(inType.Align())
	size := inType.Size()

	// Align the offset
	if *stackByteOffset%align != 0 {
		*stackByteOffset = (*stackByteOffset + align - 1) &^ (align - 1)
	}

	// Read value at aligned offset
	ptr := unsafe.Add(stackBase, *stackByteOffset)
	*stackByteOffset += size

	return reflect.NewAt(inType, ptr).Elem()
}

// callbackFloatFromDoubleSlot reads a floating-point callback argument from an
// 8-byte register or stack slot on ppc64le, where a single-precision value is
// held in double-precision format.
func callbackFloatFromDoubleSlot(slotPtr unsafe.Pointer, inType reflect.Type) reflect.Value {
	if inType.Kind() != reflect.Float32 {
		return reflect.NewAt(inType, slotPtr).Elem()
	}
	v := reflect.New(inType).Elem()
	v.SetFloat(math.Float64frombits(*(*uint64)(slotPtr)))
	return v
}

// callbackArgFromSlotBigEndian reads an argument from an 8-byte slot on big-endian architectures.
// On s390x:
// - Integer types are right-justified in GPRs: sub-8-byte values are at offset (8 - size)
// - Float32 in FPRs is left-justified: stored in upper 32 bits, so at offset 0
// - Float64 occupies the full 8-byte slot
func callbackArgFromSlotBigEndian(slotPtr unsafe.Pointer, inType reflect.Type) reflect.Value {
	size := inType.Size()
	if size >= 8 {
		// 8-byte values occupy the entire slot
		return reflect.NewAt(inType, slotPtr).Elem()
	}
	// Float32 is left-justified in FPRs (upper 32 bits), so offset is 0
	if inType.Kind() == reflect.Float32 {
		return reflect.NewAt(inType, slotPtr).Elem()
	}
	// Integer types are right-justified: offset = 8 - size
	offset := 8 - size
	ptr := unsafe.Add(slotPtr, offset)
	return reflect.NewAt(inType, ptr).Elem()
}

// callbackasmAddr returns address of runtime.callbackasm
// function adjusted by i.
// On x86 and amd64, runtime.callbackasm is a series of CALL instructions,
// and we want callback to arrive at
// correspondent call instruction instead of start of
// runtime.callbackasm.
// On ARM, runtime.callbackasm is a series of mov and branch instructions.
// R12 is loaded with the callback index. Each entry is two instructions,
// hence 8 bytes.
func callbackasmAddr(i int) uintptr {
	var entrySize int
	switch runtime.GOARCH {
	default:
		panic("purego: unsupported architecture")
	case "amd64":
		// On amd64, each callback entry is just a CALL instruction (5 bytes)
		entrySize = 5
	case "386":
		// On 386, each callback entry is MOVL $imm, CX (5 bytes) + JMP (5 bytes)
		entrySize = 10
	case "arm", "arm64", "loong64", "ppc64le", "riscv64":
		// On ARM, ARM64, Loong64, PPC64LE and RISCV64, each entry is a MOV instruction
		// followed by a branch instruction
		entrySize = 8
	case "s390x":
		// On S390X, each entry is LGHI (4 bytes) + JG (6 bytes)
		entrySize = 10
	}
	return callbackasmABI0 + uintptr(i*entrySize)
}
