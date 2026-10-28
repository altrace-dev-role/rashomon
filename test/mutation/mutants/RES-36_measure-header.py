# RES-36, card BG1: benchgate reads the platform from the input's goos and
# goarch header. -measure labels its footer by it and says when it falls back
# to the host's; the gate refuses output whose header names another platform;
# output naming two values for one key is refused; a half header is no
# header (F2). The kill row is the named test's.
B = "scripts/benchgate/main.go"

m("RES-36 BG1 the header ignored, the footer names the host", B,
  '\t\tif err := p.header(ln); err != nil {\n', '\t\tif err := p.header(""); err != nil {\n', "TestRES36r_")
m("RES-36 BG1 goarch read but goos not", B,
  '}{{"goos", &p.GOOS}, {"goarch", &p.GOARCH}} {\n', '}{{"goarch", &p.GOARCH}} {\n', "TestRES36r_")
m("RES-36 BG1 mixed headers accepted", B,
  '\t\tcase *h.field != v:\n', '\t\tcase *h.field != v && false:\n', "TestRES36t_")
m("RES-36 BG1 the gate ignores the header", B,
  '\tif got := parsed.Platform(); got != "" && got != platform {\n',
  '\tif got := parsed.Platform(); got != "" && got != platform && false {\n', "TestRES36u_")
m("RES-36 BG1 the note dropped for a headerless input", B,
  '\t\twhere, note = platform, " (host platform: the input carried no goos/goarch header)"\n',
  '\t\twhere, note = platform, ""\n', "TestRES36s_")
m("RES-36 BG1 a half header read as a platform", B,
  '\tif p.GOOS == "" || p.GOARCH == "" {\n', '\tif p.GOOS == "" && p.GOARCH == "" {\n', "TestRES36s_")
