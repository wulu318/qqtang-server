package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

const (
	processVMRead    = 0x0010
	processQueryInfo = 0x0400
)

var (
	kernel32              = syscall.NewLazyDLL("kernel32.dll")
	procOpenProcess       = kernel32.NewProc("OpenProcess")
	procReadProcessMemory = kernel32.NewProc("ReadProcessMemory")
	procCloseHandle       = kernel32.NewProc("CloseHandle")
)

func main() {
	pid := flag.Int("pid", 0, "process id")
	base := flag.Uint64("base", 0x400000, "base address")
	size := flag.Uint64("size", 0xD31000, "size in bytes")
	out := flag.String("out", "", "output file")
	flag.Parse()
	if *pid == 0 || *out == "" {
		fmt.Fprintln(os.Stderr, "-pid and -out are required")
		os.Exit(2)
	}
	handle, _, err := procOpenProcess.Call(processVMRead|processQueryInfo, 0, uintptr(*pid))
	if handle == 0 {
		fmt.Fprintf(os.Stderr, "OpenProcess failed: %v\n", err)
		os.Exit(1)
	}
	defer procCloseHandle.Call(handle)

	file, ferr := os.Create(*out)
	if ferr != nil {
		fmt.Fprintln(os.Stderr, ferr)
		os.Exit(1)
	}
	defer file.Close()

	const chunk = 64 * 1024
	buffer := make([]byte, chunk)
	var read uint64
	for read < *size {
		length := uint64(chunk)
		if *size-read < length {
			length = *size - read
		}
		var got uintptr
		ok, _, callErr := procReadProcessMemory.Call(handle, uintptr(*base+read), uintptr(unsafe.Pointer(&buffer[0])), uintptr(length), uintptr(unsafe.Pointer(&got)))
		if ok == 0 {
			fmt.Fprintf(os.Stderr, "ReadProcessMemory at 0x%x failed: %v\n", *base+read, callErr)
			os.Exit(1)
		}
		if _, werr := file.Write(buffer[:got]); werr != nil {
			fmt.Fprintln(os.Stderr, werr)
			os.Exit(1)
		}
		read += uint64(got)
		if got == 0 {
			break
		}
	}
	fmt.Printf("wrote %d bytes\n", read)
	_ = binary.LittleEndian
}
