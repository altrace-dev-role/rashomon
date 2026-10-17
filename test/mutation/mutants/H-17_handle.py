m("H-17 handler opens a socket (no net import, so only the trace sees it)", "internal/hook/handle.go",
  "\tfault.Inject(fault.PointHookStart)\n", "\tfault.Inject(fault.PointHookStart)\n\tif fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_DGRAM, 0); err == nil {\n\t\tsyscall.Close(fd)\n\t}\n", "TestH17_HandlerOpensNoSockets")
IMPORTS["H-17 handler opens a socket (no net import, so only the trace sees it)"] = ("internal/hook/handle.go", '\t"io"\n', '\t"io"\n\t"syscall"\n')
