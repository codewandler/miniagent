# Benchmark: Multi-target Analysis
EXPECTED: FILES=1

Count two things about the miniagent codebase in /repo:
  1. How many non-test .go files exist in the top-level directory
     (files NOT ending in _test.go)?
  2. How many exported functions (lines starting with "func [A-Z]") are in
     main.go specifically?

Write the result to /tmp/bench_result.txt in this exact format:
  FILES=<count1> FUNCS=<count2>

For example: FILES=5 FUNCS=3

The file must start with "FILES=" for this benchmark to pass.
