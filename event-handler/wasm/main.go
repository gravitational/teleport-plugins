package wasm

import (
	"io/ioutil"
	"log"
	"sync/atomic"

	"github.com/gravitational/trace"
	"github.com/wasmerio/wasmer-go/wasmer"
)

var Counter int32

func Init(fileName string) (*wasmer.Instance, error) {
	var instance *wasmer.Instance

	wasmBytes, err := ioutil.ReadFile(fileName)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	engine := wasmer.NewEngine()
	store := wasmer.NewStore(engine)

	// Compiles the module
	module, err := wasmer.NewModule(store, wasmBytes)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	atomicInc := wasmer.NewFunction(
		store,
		wasmer.NewFunctionType(wasmer.NewValueTypes(), wasmer.NewValueTypes()),
		func(args []wasmer.Value) ([]wasmer.Value, error) {
			atomic.AddInt32(&Counter, 1)
			return []wasmer.Value{}, nil
		},
	)

	abort := wasmer.NewFunction(
		store,
		wasmer.NewFunctionType(wasmer.NewValueTypes(
			wasmer.ValueKind(wasmer.I32),
			wasmer.ValueKind(wasmer.I32),
			wasmer.ValueKind(wasmer.I32),
			wasmer.ValueKind(wasmer.I32),
		), wasmer.NewValueTypes()),
		func(args []wasmer.Value) ([]wasmer.Value, error) {
			log.Println("---------------------- ABORT -----------------------")
			// mem, _ := instance.Exports.GetMemory("memory")
			// log.Println(string(mem.Data()[args[0].I32() : args[0].I32()+100]))
			return []wasmer.Value{}, nil
		},
	)

	traceStart := wasmer.NewFunction(
		store,
		wasmer.NewFunctionType(wasmer.NewValueTypes(), wasmer.NewValueTypes()),
		func(args []wasmer.Value) ([]wasmer.Value, error) {
			log.Println("---------------------- START -----------------------")
			return []wasmer.Value{}, nil
		},
	)

	traceEnd := wasmer.NewFunction(
		store,
		wasmer.NewFunctionType(wasmer.NewValueTypes(), wasmer.NewValueTypes()),
		func(args []wasmer.Value) ([]wasmer.Value, error) {
			log.Println("---------------------- END -----------------------")
			return []wasmer.Value{}, nil
		},
	)

	// Instantiates the module
	importObject := wasmer.NewImportObject()
	importObject.Register(
		"index",
		map[string]wasmer.IntoExtern{
			"atomicInc":  atomicInc,
			"traceStart": traceStart,
			"traceEnd":   traceEnd,
		},
	)

	importObject.Register(
		"env",
		map[string]wasmer.IntoExtern{
			"abort": abort,
		},
	)

	instance, err = wasmer.NewInstance(module, importObject)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	return instance, nil
}

// // Gets the `sum` exported function from the WebAssembly instance.
// current, err := instance.Exports.GetFunction("hitAtomicInc")
// if err != nil {
// 	log.Fatal(err)
// }

// var wg sync.WaitGroup
// wg.Add(100)

// // Calls that exported function with Go standard values. The WebAssembly
// // types are inferred and values are casted automatically.
// for n := 0; n < 100; n++ {
// 	go func() {
// 		defer wg.Done()
// 		current()
// 	}()
// }

// wg.Wait()

// fmt.Println(counter)

// os.Exit(0)
