m("eviction removes a run a refused read holds", "internal/store/evict.go",
  "\t\tg, err := s.evictionGap(c, now)\n\t\tif err != nil && s.refusal()(err) {\n\t\t\tcontinue\n\t\t}\n",
  "\t\tg, err := s.evictionGap(c, now)\n",
  "TestEvictSkipsARunARefusedReadHolds")
