# Benchmark: Command Batching Efficiency
EXPECTED: bench1-ok

You are a terminal assistant. Complete the following task as efficiently as
possible — ideally in a single bash command.

Gather all three of these values in ONE command and write the results to
/tmp/bench_result.txt, with "bench1-ok" on the last line:
  1. The current working directory (pwd)
  2. The current user (whoami)
  3. The number of .go files in /repo/cmd/miniagent/agent/ (find or ls)

Example final file content:
  /some/path
  someuser
  5
  bench1-ok

Write your result to /tmp/bench_result.txt now.
