m("H-17 handler opens a socket (no net import, so only the trace sees it)", "internal/hook/handle.go",
  "\tfault.Inject(fault.PointHookStart)\n", "\tfault.Inject(fault.PointHookStart)\n\tif fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_DGRAM, 0); err == nil {\n\t\tsyscall.Close(fd)\n\t}\n", "TestH17_HandlerOpensNoSockets")
IMPORTS["H-17 handler opens a socket (no net import, so only the trace sees it)"] = ("internal/hook/handle.go", '\t"io"\n', '\t"io"\n\t"syscall"\n')

# H-17 and TRU-40 scope to what a shipped binary can link (spec run 72): a
# test/ package imported only from _test.go files is out of scope, and a guard
# keeps any package outside test/ from importing one.
m("H-17 the shipped scope takes in test-only packages", "test/acceptance/h17_no_egress_test.go",
  "\t\tif isTestTree(pkg) {\n\t\t\tcontinue\n\t\t}\n\t\tscope[pkg] = true\n", "\t\tscope[pkg] = true\n", "TestH17_")
m("H-17 the guard lets a package outside test/ import a test/ package", "test/acceptance/h17_no_egress_test.go",
  "\t\t\tif isTestTree(imp) {\n\t\t\t\tout = append(out, pkg+\" imports \"+imp)\n\t\t\t}\n", "\t\t\t_ = imp\n", "TestH17_TestPackagesStayTestOnly")
